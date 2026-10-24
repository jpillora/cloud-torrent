package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func searchFixtureServer(t *testing.T, origin string) *Server {
	t.Helper()
	config, _ := json.Marshal(map[string]any{
		"fixture":      map[string]any{"url": origin + "/{{query}}", "list": "li", "result": map[string]any{"name": "b"}, "headers": map[string]string{"Accept-Encoding": "gzip"}},
		"fixture/item": map[string]any{"url": origin + "{{item}}", "result": map[string]any{"name": "h1"}},
	})
	h, err := newSearchHandler(config)
	must(t, err)
	return &Server{scraper: h, OutboundAllowCIDRs: "127.0.0.1/32", searchTimeout: 150 * time.Millisecond}
}
func searchHandler(s *Server) http.Handler {
	return http.StripPrefix("/search", http.HandlerFunc(s.handleSearch))
}

func TestProviderResponseBudgetsOnActualRoutes(t *testing.T) {
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	_, err := gz.Write(bytes.Repeat([]byte("a"), maxSearchBytes+1))
	must(t, err)
	must(t, gz.Close())
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/large":
			w.Write(bytes.Repeat([]byte("a"), maxSearchBytes+1))
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			w.Write(compressed.Bytes())
		case "/many":
			w.Write([]byte(strings.Repeat("<li><b>result</b></li>", maxSearchResults+1)))
		case "/fail":
			http.Error(w, "upstream failure", 503)
		case "/redirect":
			http.Redirect(w, r, "http://127.0.0.2/private", 302)
		case "/empty":
			w.Write([]byte("<p>empty</p>"))
		default:
			w.Write([]byte("<h1>detail</h1><li><b>valid</b></li>"))
		}
	}))
	defer origin.Close()
	s := searchFixtureServer(t, origin.URL)
	s.searchTimeout = 3 * time.Second
	for _, query := range []string{"large", "gzip", "many", "fail", "redirect"} {
		w := do(searchHandler(s), "GET", "/search/fixture?query="+query)
		if w.Code == 200 {
			t.Fatalf("unbounded provider %s accepted", query)
		}
		if len(s.searchSlots) != 0 {
			t.Fatal("request slot leaked", query)
		}
	}
	for _, path := range []string{"/search/fixture?query=ok", "/search/fixture/item?item=/ok", "/search/fixture?query=empty"} {
		w := do(searchHandler(s), "GET", path)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if strings.Contains(path, "empty") && strings.TrimSpace(w.Body.String()) != "[]" {
			t.Fatal("empty result confused with failure")
		}
	}
}

func TestProviderTimeoutAndCancellationReleaseUpstreamAndSlots(t *testing.T) {
	for _, detail := range []bool{false, true} {
		for _, headers := range []bool{false, true} {
			for _, manual := range []bool{false, true} {
				started, stopped := make(chan struct{}), make(chan struct{})
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if headers {
						w.WriteHeader(200)
						w.(http.Flusher).Flush()
					}
					close(started)
					<-r.Context().Done()
					close(stopped)
				}))
				s := searchFixtureServer(t, origin.URL)
				path := "/search/fixture?query=stall"
				if detail {
					path = "/search/fixture/item?item=/stall"
				}
				ctx, cancel := context.WithCancel(context.Background())
				r := httptest.NewRequest("GET", path, nil).WithContext(ctx)
				done := make(chan int, 1)
				go func() { w := httptest.NewRecorder(); searchHandler(s).ServeHTTP(w, r); done <- w.Code }()
				select {
				case <-started:
				case <-time.After(time.Second):
					t.Fatal("upstream never started")
				}
				if manual {
					cancel()
				}
				select {
				case status := <-done:
					if status == 200 {
						t.Fatal("stalled provider succeeded")
					}
				case <-time.After(time.Second):
					t.Fatal("request budget not enforced")
				}
				select {
				case <-stopped:
				case <-time.After(time.Second):
					t.Fatal("upstream work survived cancellation")
				}
				if len(s.searchSlots) != 0 {
					t.Fatal("request slot leaked")
				}
				cancel()
				origin.Close()
			}
		}
	}
}

func TestProviderConcurrencyIsBoundedAndRecovers(t *testing.T) {
	started := make(chan struct{}, maxSearchConcurrency)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.Write([]byte("<li><b>ready</b></li>"))
			return
		}
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer origin.Close()
	s := searchFixtureServer(t, origin.URL)
	s.searchTimeout = 3 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{}, maxSearchConcurrency)
	for range maxSearchConcurrency {
		go func() {
			r := httptest.NewRequest("GET", "/search/fixture?query=stall", nil).WithContext(ctx)
			searchHandler(s).ServeHTTP(httptest.NewRecorder(), r)
			done <- struct{}{}
		}()
	}
	for range maxSearchConcurrency {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("requests did not start")
		}
	}
	if w := do(searchHandler(s), "GET", "/search/fixture?query=ok"); w.Code != 429 {
		t.Fatalf("overcapacity status: %d", w.Code)
	}
	cancel()
	for range maxSearchConcurrency {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("cancelled slot retained")
		}
	}
	if w := do(searchHandler(s), "GET", "/search/fixture?query=ok"); w.Code != 200 {
		t.Fatal("slots did not recover", w.Code)
	}
}
