package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jpillora/scraper/scraper"
)

func TestSearchExposesOnlyReadOnlyProviderQueries(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<html><h1>Public fixture</h1><ul><li><b>Big Buck Bunny</b></li></ul></html>`)
	}))
	defer upstream.Close()
	config := []byte(`{"fixture":{"url":"` + upstream.URL + `","list":"li","result":{"name":"b"}},"fixture/item":{"url":"` + upstream.URL + `","result":{"name":"h1"}}}`)
	s := &Server{scraper: &scraper.Handler{}, searchConfig: config}
	if err := s.scraper.LoadConfig(config); err != nil {
		t.Fatal(err)
	}
	s.state.SearchProviders = s.scraper.Config
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
	if string(before) != string(after) || string(before) != string(state) || string(s.searchConfig) != string(config) {
		t.Fatal("query routes changed provider catalog or refresh baseline")
	}
}
