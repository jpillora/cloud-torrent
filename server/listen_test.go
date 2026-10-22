package server

import (
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestOpenBrowserAfterListenerReady(t *testing.T) {
	s := &Server{Open: true}
	srv := &http.Server{Addr: "127.0.0.1:0", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "ready")
	})}
	defer srv.Close()
	opened := make(chan error, 1)
	done := make(chan error, 1)
	go func() {
		done <- s.serveHTTP(srv, func(url string) error {
			client := &http.Client{Timeout: time.Second}
			resp, err := client.Get(url)
			if err == nil {
				defer resp.Body.Close()
				b, readErr := io.ReadAll(resp.Body)
				err = readErr
				if string(b) != "ready" {
					err = errors.New("launched URL was not served")
				}
			}
			opened <- err
			return err
		})
	}()
	select {
	case err := <-opened:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("browser launcher not called")
	}
	srv.Close()
	if err := <-done; !errors.Is(err, http.ErrServerClosed) {
		t.Fatal(err)
	}
}

func TestFailedBindDoesNotOpenBrowser(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	defer listener.Close()
	s := &Server{Open: true}
	called := false
	err = s.serveHTTP(&http.Server{Addr: listener.Addr().String()}, func(string) error { called = true; return nil })
	if err == nil || called {
		t.Fatalf("bind failure: error %v, opened %v", err, called)
	}
}

func TestBrowserURLs(t *testing.T) {
	for address, want := range map[string]string{
		"0.0.0.0:3000":        "http://127.0.0.1:3000",
		"[::]:3000":           "http://[::1]:3000",
		"[2001:db8::1]:1234":  "http://[2001:db8::1]:1234",
		"100.103.141.66:3001": "http://100.103.141.66:3001",
	} {
		if got := browserURL(address, false); got != want {
			t.Errorf("%s: %s", address, got)
		}
	}
	if got := browserURL("127.0.0.1:443", true); got != "https://127.0.0.1:443" {
		t.Fatal(got)
	}
}
