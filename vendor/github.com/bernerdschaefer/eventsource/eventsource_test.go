package eventsource

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"
)

type responseWriter interface {
	http.ResponseWriter
	http.Flusher
	http.CloseNotifier
}

func testServer(f func(responseWriter, *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f(w.(responseWriter), r)
	}))
}

func request(url string) *http.Request {
	req, _ := http.NewRequest("GET", url, nil)
	return req
}

func TestEventSourceHeaders(t *testing.T) {
	headers := make(chan http.Header)
	server := testServer(func(w responseWriter, r *http.Request) {
		headers <- r.Header
	})
	defer server.Close()

	es := New(request(server.URL), -1)
	go es.connect()

	h := <-headers

	if h.Get("Accept") != "text/event-stream" {
		t.Errorf("expected accept header = %q, got %q", "text/event-stream", h.Get("Accept"))
	}

	if h.Get("Cache-Control") != "no-cache" {
		t.Errorf("expected cache control header = %q, got %q", "no-cache", h.Get("Cache-Control"))
	}
}

func TestEventSource204(t *testing.T) {
	server := testServer(func(w responseWriter, r *http.Request) {
		w.WriteHeader(204)
	})
	defer server.Close()

	es := New(request(server.URL), -1)

	es.connect()

	if es.err == nil {
		t.Fatal("event source did not close on 204")
	}
}

func TestEventSource(t *testing.T) {
	server := testServer(func(w responseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	defer server.Close()

	es := New(request(server.URL), time.Millisecond)

	es.connect()

	if es.err == nil {
		t.Fatal("event source did not close on 200 with no content type")
	}
}

func TestEventSourceEmphemeral500(t *testing.T) {
	fail := true

	server := testServer(func(w responseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(500)
		} else {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
		}

		fail = !fail
	})
	defer server.Close()

	es := New(request(server.URL), time.Millisecond)

	es.connect()

	if es.err != nil {
		t.Fatalf("event source did not reconnect on 500; got %q", es.err)
	}
}

func TestEventSourceRead(t *testing.T) {
	fail := make(chan struct{})
	more := make(chan bool, 1)
	server := testServer(func(w responseWriter, r *http.Request) {
		select {
		case <-fail:
			w.WriteHeader(204)
			return
		default:
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		var id int

		if lastID := r.Header.Get("Last-Event-Id"); lastID != "" {
			if i, err := strconv.ParseInt(lastID, 10, 64); err == nil {
				id = int(i) + 1
			}
		}

		for {
			if !<-more {
				break
			}
			fmt.Fprintf(w, "id: %d\ndata: message %d\n\n", id, id)
			w.Flush()
			id++
		}
	})
	defer server.Close()
	defer close(more)

	es := New(request(server.URL), -1)
	more <- true

	event, err := es.Read()
	if err != nil {
		t.Fatal(err)
	}

	if event.ID != "0" {
		t.Fatalf("expected id = 0, got %s", event.ID)
	}

	if event.Type != "message" {
		t.Fatalf("expected event = message, got %s", event.Type)
	}

	if !bytes.Equal([]byte("message 0"), event.Data) {
		t.Fatalf("expected data = message 0, got %s", event.Data)
	}

	more <- true
	event, err = es.Read()
	if err != nil {
		t.Fatal(err)
	}

	if event.ID != "1" {
		t.Fatalf("expected id = 1, got %s", event.ID)
	}

	if event.Type != "message" {
		t.Fatalf("expected event = message, got %s", event.Type)
	}

	if !bytes.Equal([]byte("message 1"), event.Data) {
		t.Fatalf("expected data = message 1, got %s", event.Data)
	}

	// stop handler
	more <- false
	// start handler
	more <- true
	event, err = es.Read()
	if err != nil {
		t.Fatal(err)
	}

	if event.ID != "2" {
		t.Fatalf("expected id = 2, got %s", event.ID)
	}

	if event.Type != "message" {
		t.Fatalf("expected event = message, got %s", event.Type)
	}

	if !bytes.Equal([]byte("message 2"), event.Data) {
		t.Fatalf("expected data = message 2, got %s", event.Data)
	}

	more <- false
	close(fail)

	if _, err := es.Read(); err == nil {
		t.Fatal("expected fatal err")
	}
}

func TestEventSourceChangeRetry(t *testing.T) {
	server := testServer(func(w responseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		NewEncoder(w).Encode(Event{
			Retry: "10000",
			Data:  []byte("foo"),
		})
	})

	defer server.Close()

	es := New(request(server.URL), -1)

	event, err := es.Read()

	if err != nil {
		t.Fatal(err)
	}

	if event.Retry != "10000" {
		t.Error("event retry not set")
	}

	if es.retry != (10 * time.Second) {
		t.Fatal("expected retry to be updated, but wasn't")
	}
}

func TestEventSourceBOM(t *testing.T) {
	server := testServer(func(w responseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)

		w.Write([]byte("\xEF\xBB\xBF"))
		NewEncoder(w).Encode(Event{Type: "custom", Data: []byte("foo")})
	})
	defer server.Close()

	es := New(request(server.URL), -1)

	event, err := es.Read()

	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(event, Event{Type: "custom", Data: []byte("foo")}) {
		t.Fatal("message was unsuccessfully decoded with BOM")
	}
}
