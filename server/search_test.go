package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jpillora/scraper/scraper"
	"github.com/jpillora/velox"
)

type searchRoundTrip func(*http.Request) (*http.Response, error)

func (f searchRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSearchHeadersWithLoggingDisabled(t *testing.T) {
	agents := make(chan string, 4)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agents <- r.UserAgent()
		io.WriteString(w, `<h1>fixture</h1>`)
	}))
	defer upstream.Close()
	embedded, err := newSearchHandler(defaultSearchConfig)
	must(t, err)
	if embedded.Log {
		t.Fatal("header compatibility must not require logging")
	}
	for _, endpoint := range embedded.Config {
		if endpoint.Headers["User-Agent"] != userAgent {
			t.Fatal("embedded endpoint lacks browser User-Agent")
		}
	}
	embedded.Config["zq"].URL = upstream.URL
	_, err = embedded.Config["zq"].Execute(nil)
	must(t, err)
	if got := <-agents; got != userAgent {
		t.Fatalf("embedded request User-Agent %q", got)
	}
	refreshed := `{"fixture":{"url":"` + upstream.URL + `","result":{"name":"h1"}},"override":{"url":"` + upstream.URL + `","headers":{"user-agent":"provider-specific"},"result":{"name":"h1"}}}`
	client := &http.Client{Transport: searchRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() == searchConfigURL {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(refreshed))}, nil
		}
		return http.DefaultTransport.RoundTrip(r)
	})}
	s := &Server{scraper: embedded, fetchClient: client}
	_ = velox.SyncHandler(&s.state)
	must(t, s.fetchSearchConfig())
	if s.scraper.Log {
		t.Fatal("refresh enabled logging")
	}
	for id, want := range map[string]string{"fixture": userAgent, "override": "provider-specific"} {
		_, err := s.scraper.Config[id].Execute(nil)
		must(t, err)
		if got := <-agents; got != want {
			t.Fatalf("refreshed %s request User-Agent %q, want %q", id, got, want)
		}
	}
}

func TestSearchExposesOnlyReadOnlyProviderQueries(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<html><h1>Public fixture</h1><ul><li><b>Big Buck Bunny</b></li></ul></html>`)
	}))
	defer upstream.Close()
	config := []byte(`{"fixture":{"url":"` + upstream.URL + `","list":"li","result":{"name":"b"}},"fixture/item":{"url":"` + upstream.URL + `","result":{"name":"h1"}}}`)
	s := &Server{scraper: &providerCatalog{}, searchConfig: config, OutboundAllowCIDRs: "127.0.0.0/8"}
	if err := s.scraper.LoadConfig(config); err != nil {
		t.Fatal(err)
	}
	s.state.SearchProviders = searchDescriptors(s.scraper.Config)
	before, err := json.Marshal(s.scraper.Config)
	must(t, err)
	h := s.handler()
	for _, path := range []string{"/search/", "/search/fixture", "/search/fixture/item"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("%s %s returned %d", method, path, w.Code)
			}
		}
	}
	for _, path := range []string{"/search/", "/search/fixture/admin", "/search/fixture/item/extra"} {
		if w := do(h, http.MethodGet, path); w.Code != http.StatusNotFound {
			t.Fatalf("GET %s returned %d", path, w.Code)
		}
	}
	for path, want := range map[string]string{"/search/fixture": "Big Buck Bunny", "/search/fixture/item": "Public fixture"} {
		w := do(h, http.MethodGet, path)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), want) {
			t.Fatalf("GET %s returned %d: %s", path, w.Code, w.Body.String())
		}
	}
	after, err := json.Marshal(s.scraper.Config)
	must(t, err)
	state, err := json.Marshal(s.state.SearchProviders)
	must(t, err)
	expectedState, err := json.Marshal(searchDescriptors(s.scraper.Config))
	must(t, err)
	if string(before) != string(after) || string(state) != string(expectedState) || string(s.searchConfig) != string(config) {
		t.Fatal("query routes changed provider catalog or refresh baseline")
	}
}

func TestSearchDescriptorsDoNotExposeCredentials(t *testing.T) {
	config := scraper.Config{"private": &scraper.Endpoint{Name: "private", URL: "https://user:password@example.com/path?passkey=secret", Headers: map[string]string{"Authorization": "secret"}, Body: "secret"}}
	b, err := json.Marshal(searchDescriptors(config))
	must(t, err)
	for _, secret := range []string{"password", "secret", "Authorization", "passkey"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("provider secret exposed")
		}
	}
}
