package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jpillora/velox"
)

func jsonSearchServer(t *testing.T, endpoint map[string]any) *Server {
	t.Helper()
	body, err := json.Marshal(map[string]any{"fixture": endpoint})
	must(t, err)
	h, err := newSearchHandler(body)
	must(t, err)
	return &Server{scraper: h, OutboundAllowCIDRs: "127.0.0.1/32", searchTimeout: time.Second}
}
func jsonEndpoint(raw string) map[string]any {
	return map[string]any{"type": "json", "url": raw, "json": map[string]any{"list": "/data/items", "fields": map[string]string{"name": "/title", "url": "/links/0", "torrent": "/download", "seeds": "/stats/seeds"}}}
}
func TestJSONProviderNestedArraysMirrorsAndEmpty(t *testing.T) {
	var primaryCalls, mirrorCalls atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { primaryCalls.Add(1); http.Error(w, "failure", 503) }))
	defer primary.Close()
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mirrorCalls.Add(1)
		if r.URL.Query().Get("query") == "empty" {
			io.WriteString(w, `{"data":{"items":[]}}`)
			return
		}
		io.WriteString(w, `{"data":{"items":[{"title":"CC0 fixture","links":["../detail/one"],"download":"/legal.torrent","stats":{"seeds":42}}]}}`)
	}))
	defer mirror.Close()
	endpoint := jsonEndpoint(primary.URL + "/search?query={{query}}")
	endpoint["mirrors"] = []string{mirror.URL + "/alternate/search?query={{query}}"}
	s := jsonSearchServer(t, endpoint)
	w := do(searchHandler(s), "GET", "/search/fixture?query=legal")
	if w.Code != 200 {
		t.Fatalf("fallback: %d %s", w.Code, w.Body.String())
	}
	var rows []map[string]string
	must(t, json.Unmarshal(w.Body.Bytes(), &rows))
	if len(rows) != 1 || rows[0]["url"] != mirror.URL+"/detail/one" || rows[0]["torrent"] != mirror.URL+"/legal.torrent" || rows[0]["path"] != "/detail/one" || rows[0]["seeds"] != "42" {
		t.Fatalf("normalized mirror rows: %+v", rows)
	}
	w = do(searchHandler(s), "GET", "/search/fixture?query=empty")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("empty: %d %s", w.Code, w.Body.String())
	}
	if primaryCalls.Load() != 2 || mirrorCalls.Load() != 2 {
		t.Fatal("unexpected attempts")
	}
	// A valid empty primary is final: do not query mirrors for different answers.
	endpoint["url"] = mirror.URL + "/search?query={{query}}"
	endpoint["mirrors"] = []string{primary.URL + "/fallback"}
	s = jsonSearchServer(t, endpoint)
	w = do(searchHandler(s), "GET", "/search/fixture?query=empty")
	if w.Code != 200 || primaryCalls.Load() != 2 {
		t.Fatal("empty result triggered fallback")
	}
}
func TestJSONProviderFailuresAndUnsafeResults(t *testing.T) {
	hash := strings.Repeat("a", 40)
	bodies := map[string]string{
		"malformed": `{"data":`, "missing": `{}`, "not-array": `{"data":{"items":{}}}`, "trailing": `{"data":{"items":[]}} {}`,
		"javascript": `{"data":{"items":[{"links":["javascript:alert(1)"]}]}}`,
		"data":       `{"data":{"items":[{"download":"data:text/plain,x"}]}}`,
		"file":       `{"data":{"items":[{"download":"file:///secret"}]}}`,
		"object":     `{"data":{"items":[{"title":{}}]}}`,
		"many":       `{"data":{"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, maxSearchResults+1), ",") + `]}}`,
		"large":      strings.Repeat(" ", maxSearchBytes+1),
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, bodies[strings.TrimPrefix(r.URL.Path, "/")])
	}))
	defer upstream.Close()
	s := jsonSearchServer(t, jsonEndpoint(upstream.URL+"/{{query}}"))
	for query := range bodies {
		t.Run(query, func(t *testing.T) {
			w := do(searchHandler(s), "GET", "/search/fixture?query="+query)
			if w.Code != 502 {
				t.Fatalf("invalid response accepted: %d %s", w.Code, w.Body.String())
			}
			if len(s.searchSlots) != 0 {
				t.Fatal("leaked slot")
			}
		})
	}
	// Validate unsafe identity fields for HTML and JSON through shared normalization.
	for _, fields := range []map[string]string{{"infohash": "bad"}, {"magnet": "javascript:bad"}, {"magnet": "magnet:?xt=urn:btih:bad"}, {"magnet": "magnet:?xt=urn:btih:" + hash + "&ws=file:///secret"}, {"magnet": "magnet:?xt=urn:btih:" + hash + "&tr=javascript:bad"}, {"path": "//elsewhere.example/private"}} {
		base, err := url.Parse(upstream.URL)
		must(t, err)
		if err := normalizeSearchResult(fields, base); err == nil {
			t.Fatalf("unsafe result accepted: %+v", fields)
		}
	}
}
func TestJSONRootArrayAndItemExtraction(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/array" {
			io.WriteString(w, `[{"a/b":{"~key":["legal"]}}]`)
		} else {
			io.WriteString(w, `{"hash":"`+strings.Repeat("a", 40)+`"}`)
		}
	}))
	defer upstream.Close()
	config := fmt.Sprintf(`{"fixture":{"type":"json","url":%q,"json":{"list":"","fields":{"name":"/a~1b/~0key/0"}}},"fixture/item":{"type":"json","url":%q,"json":{"fields":{"infohash":"/hash"}}}}`, upstream.URL+"/array", upstream.URL+"{{item}}")
	h, err := newSearchHandler([]byte(config))
	must(t, err)
	s := &Server{scraper: h, OutboundAllowCIDRs: "127.0.0.1/32"}
	for path, want := range map[string]string{"/search/fixture": "[{\"name\":\"legal\"}]", "/search/fixture/item?item=/detail": "{\"infohash\":\"" + strings.Repeat("a", 40) + "\"}"} {
		w := do(searchHandler(s), "GET", path)
		if w.Code != 200 || strings.TrimSpace(w.Body.String()) != want {
			t.Fatalf("shape: %d %s want %s", w.Code, w.Body.String(), want)
		}
	}
}
func TestSearchMirrorDeadlineCancellationAndAttemptCap(t *testing.T) {
	var attempts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { attempts.Add(1); http.Error(w, "failed", 503) }))
	defer upstream.Close()
	endpoint := jsonEndpoint(upstream.URL + "/primary")
	endpoint["mirrors"] = []string{upstream.URL + "/one", upstream.URL + "/two"}
	s := jsonSearchServer(t, endpoint)
	if w := do(searchHandler(s), "GET", "/search/fixture"); w.Code != 502 || attempts.Load() != 3 {
		t.Fatalf("attempts: %d %d", w.Code, attempts.Load())
	}
	endpoint["mirrors"] = []string{upstream.URL + "/one", upstream.URL + "/two", upstream.URL + "/three"}
	body, _ := json.Marshal(map[string]any{"fixture": endpoint})
	if _, err := newSearchHandler(body); err == nil {
		t.Fatal("accepted more than three attempts")
	}
	started := make(chan struct{})
	canceled := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(canceled) }))
	defer slow.Close()
	endpoint = jsonEndpoint(slow.URL)
	endpoint["mirrors"] = []string{upstream.URL + "/must-not-run"}
	s = jsonSearchServer(t, endpoint)
	s.searchTimeout = 50 * time.Millisecond
	done := make(chan int)
	go func() { w := do(searchHandler(s), "GET", "/search/fixture"); done <- w.Code }()
	<-started
	select {
	case code := <-done:
		if code != 504 {
			t.Fatalf("deadline: %d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("deadline did not bound request")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("upstream not canceled")
	}
	if attempts.Load() != 3 {
		t.Fatal("fallback started after deadline")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := httptest.NewRequest("GET", "/search/fixture", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	searchHandler(s).ServeHTTP(w, r)
	if w.Code != 504 || len(s.searchSlots) != 0 {
		t.Fatal("cancellation did not release slot")
	}
}
func TestProviderConfigurationAtomicValidationAndPrivateDescriptors(t *testing.T) {
	valid := []byte(`{"fixture":{"url":"https://example.com/search?token=secret","headers":{"Authorization":"secret"},"result":{"name":"h1"}}}`)
	h, err := newSearchHandler(valid)
	must(t, err)
	before, _ := json.Marshal(h.Config)
	for _, bad := range []string{
		`{"fixture":null}`, `{"fixture":{"type":"shell","url":"https://example.com","result":{"name":"h1"}}}`,
		`{"fixture":{"url":"javascript:bad","result":{"name":"h1"}}}`,
		`{"fixture":{"url":"https://{{query}}/","result":{"name":"h1"}}}`,
		`{"fixture":{"url":"https://example.com","list":"[","result":{"name":"h1"}}}`,
		`{"fixture":{"type":"json","url":"https://example.com","json":{"list":"/a","fields":{"name":"/bad~2"}}}}`,
	} {
		if err := h.LoadConfig([]byte(bad)); err == nil {
			t.Fatalf("accepted invalid catalog: %s", bad)
		}
		after, _ := json.Marshal(h.Config)
		if !bytes.Equal(before, after) {
			t.Fatal("invalid config replaced last-known-good")
		}
	}
	s := &Server{scraper: h, searchConfig: valid}
	_ = velox.SyncHandler(&s.state)
	s.fetchClient = &http.Client{Transport: searchRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"broken":null}`))}, nil
	})}
	if err := s.fetchSearchConfig(); err == nil {
		t.Fatal("invalid refresh succeeded")
	}
	if s.scraper != h || !bytes.Equal(s.searchConfig, valid) {
		t.Fatal("invalid refresh replaced catalog")
	}
	descriptor, _ := json.Marshal(searchDescriptors(h.Config))
	for _, secret := range []string{"secret", "token", "Authorization"} {
		if bytes.Contains(descriptor, []byte(secret)) {
			t.Fatalf("descriptor leaked %s", secret)
		}
	}
}

func TestHTMLMirrorItemAndRedirectOrigin(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unavailable", 503) }))
	defer primary.Close()
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			http.Redirect(w, r, "/actual/search", 302)
			return
		}
		if r.URL.Path == "/detail" {
			io.WriteString(w, `<a href="/legal.torrent">download</a>`)
			return
		}
		io.WriteString(w, `<li><a href="../detail">legal fixture</a></li>`)
	}))
	defer mirror.Close()
	config, _ := json.Marshal(map[string]any{
		"fixture":      map[string]any{"url": primary.URL + "/search", "mirrors": []string{mirror.URL + "/search"}, "list": "li", "result": map[string]any{"name": "a", "url": []string{"a", "@href"}}},
		"fixture/item": map[string]any{"url": primary.URL + "{{item}}", "mirrors": []string{mirror.URL + "{{item}}"}, "result": map[string]any{"torrent": []string{"a", "@href"}}},
	})
	h, err := newSearchHandler(config)
	must(t, err)
	s := &Server{scraper: h, OutboundAllowCIDRs: "127.0.0.1/32"}
	w := do(searchHandler(s), "GET", "/search/fixture")
	if w.Code != 200 || !strings.Contains(w.Body.String(), mirror.URL+"/detail") {
		t.Fatalf("HTML mirror: %d %s", w.Code, w.Body.String())
	}
	w = do(searchHandler(s), "GET", "/search/fixture/item?item=/detail")
	if w.Code != 200 || !strings.Contains(w.Body.String(), mirror.URL+"/legal.torrent") {
		t.Fatalf("HTML item: %d %s", w.Code, w.Body.String())
	}
	w = do(searchHandler(s), "GET", "/search/fixture/item?item=evil.example")
	if w.Code != 400 {
		t.Fatal("item changed template authority")
	}
}

func TestOperatorProviderFileDisablesRemoteRefresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.json")
	config := []byte(`{"local":{"type":"json","url":"https://primary.example/search","mirrors":["https://mirror.example/search"],"json":{"list":"/items","fields":{"name":"/name"}}}}`)
	must(t, os.WriteFile(path, config, 0600))
	s := &Server{SearchConfigPath: path}
	must(t, s.startSearch())
	if s.scraper.Config["local"] == nil || len(s.scraper.options["local"].Mirrors) != 1 {
		t.Fatal("operator config not loaded")
	}
	if !bytes.Contains([]byte(strings.Join(s.contentPaths(), "\n")), []byte(path)) {
		t.Fatal("private config missing content protection")
	}
}
