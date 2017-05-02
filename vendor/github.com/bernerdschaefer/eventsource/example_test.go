package eventsource_test

import (
	"fmt"
	"github.com/bernerdschaefer/eventsource"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func ExampleHandler() {
	http.Handle("/events", eventsource.Handler(func(lastID string, e *eventsource.Encoder, stop <-chan bool) {
		for {
			select {
			case <-time.After(200 * time.Millisecond):
				e.Encode(eventsource.Event{Data: []byte("tick")})
			case <-stop:
				return
			}
		}
	}))
}

func ExampleHandler_ServeHTTP() {
	es := eventsource.Handler(func(lastID string, e *eventsource.Encoder, stop <-chan bool) {
		for {
			select {
			case <-time.After(200 * time.Millisecond):
				e.Encode(eventsource.Event{Data: []byte("tick")})
			case <-stop:
				return
			}
		}
	})

	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		es.ServeHTTP(w, r)
	})
}

func ExampleEncoder() {
	enc := eventsource.NewEncoder(os.Stdout)

	events := []eventsource.Event{
		{ID: "1", Data: []byte("data")},
		{ResetID: true, Data: []byte("id reset")},
		{Type: "add", Data: []byte("1")},
	}

	for _, event := range events {
		if err := enc.Encode(event); err != nil {
			log.Fatal(err)
		}
	}

	if err := enc.WriteField("", []byte("heartbeat")); err != nil {
		log.Fatal(err)
	}

	if err := enc.Flush(); err != nil {
		log.Fatal(err)
	}

	// Output:
	// id: 1
	// data: data
	//
	// id
	// data: id reset
	//
	// event: add
	// data: 1
	//
	// : heartbeat
	//
}

func ExampleDecoder() {
	stream := strings.NewReader(`id: 1
event: add
data: 123

id: 2
event: remove
data: 321

id: 3
event: add
data: 123

`)
	dec := eventsource.NewDecoder(stream)

	for {
		var event eventsource.Event
		err := dec.Decode(&event)

		if err == io.EOF {
			break
		} else if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("%s. %s %s\n", event.ID, event.Type, event.Data)
	}

	// Output:
	// 1. add 123
	// 2. remove 321
	// 3. add 123
}

func ExampleNew() {
	req, _ := http.NewRequest("GET", "http://localhost:9090/events", nil)
	req.SetBasicAuth("user", "pass")

	es := eventsource.New(req, 3*time.Second)

	for {
		event, err := es.Read()

		if err != nil {
			log.Fatal(err)
		}

		log.Printf("%s. %s %s\n", event.ID, event.Type, event.Data)
	}
}
