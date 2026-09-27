//go:build live

package server

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/jpillora/scraper/scraper"
)

// liveQueries is a search per provider that should always have results. A
// provider without one fails the run, so a new provider can't skip the check.
var liveQueries = map[string]string{
	"nyaa": "naruto",
	"lt":   "ubuntu",
	"abb":  "sherlock holmes",
	"ia":   "night of the living dead",
}

var infohashRe = regexp.MustCompile(`^([0-9a-fA-F]{40}|[A-Z2-7]{32})$`)

// TestLiveSearchProviders runs every built-in provider against the real site
// and follows its first result the way the UI does (resolveItem, then
// resolveLookup in static/files/js/lib/search.js) to something addable.
// Run with: go test -tags live -run TestLiveSearchProviders ./server
func TestLiveSearchProviders(t *testing.T) {
	h := &scraper.Handler{}
	if err := h.LoadConfig(builtinSearchConfig()); err != nil {
		t.Fatal(err)
	}
	for id := range h.Config {
		if strings.HasSuffix(id, "/item") {
			continue
		}
		t.Run(id, func(t *testing.T) {
			query, ok := liveQueries[id]
			if !ok {
				t.Fatalf("no live query for %q; add one to liveQueries", id)
			}
			results, err := h.Config[id].Execute(map[string]string{"query": query})
			if err != nil {
				t.Fatalf("search %q: %v", query, err)
			}
			// the scraper drops any row missing a configured field, so a
			// broken selector shows up here as no results at all
			if len(results) == 0 {
				t.Fatalf("search %q returned no results (%s)", query, whatCameBack(h.Config[id], query))
			}
			first := results[0]
			if first["name"] == "" {
				t.Fatalf("first result has no name: %v", first)
			}
			if strings.HasPrefix(first["magnet"], "magnet:?") {
				return
			}
			if strings.HasPrefix(first["torrent"], "http") {
				checkTorrentURL(t, first["torrent"])
				return
			}
			path := first["path"]
			if path == "" && strings.HasPrefix(first["url"], "/") {
				path = first["url"]
			}
			item := h.Config[id+"/item"]
			if path == "" || item == nil {
				t.Fatalf("first result has no magnet, torrent or item lookup: %v", first)
			}
			found, err := item.Execute(map[string]string{"item": path})
			if err != nil || len(found) == 0 {
				t.Fatalf("item lookup %s: %v", path, err)
			}
			got := found[0]
			switch {
			case strings.HasPrefix(got["magnet"], "magnet:?"):
			case strings.HasPrefix(got["torrent"], "http"):
				checkTorrentURL(t, got["torrent"])
			case infohashRe.MatchString(strings.TrimSpace(got["infohash"])):
				// a trackerless magnet only finds peers through DHT
				if !regexp.MustCompile(`^(http|udp)://`).MatchString(strings.TrimSpace(got["tracker"])) {
					t.Errorf("item %s has an infohash but no tracker: %v", path, got)
				}
			default:
				t.Errorf("item %s gave nothing addable: %v", path, got)
			}
		})
	}
}

var titleRe = regexp.MustCompile(`(?is)<title>\s*(.*?)\s*</title>`)

// whatCameBack refetches a search page to tell a block (a Cloudflare
// challenge, a 403) apart from a changed page layout.
func whatCameBack(e *scraper.Endpoint, query string) string {
	// the scraper query-escapes values after the "?" and leaves the rest as is
	q := query
	if strings.Contains(e.URL, "?") && strings.Index(e.URL, "{{query}}") > strings.Index(e.URL, "?") {
		q = url.QueryEscape(query)
	}
	u := strings.NewReplacer("{{query}}", q, "{{page:1}}", "1").Replace(e.URL)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err.Error()
	}
	for k, v := range e.Headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	title := ""
	if m := titleRe.FindSubmatch(b); m != nil {
		title = string(m[1])
	}
	return "GET " + resp.Request.URL.String() + ": " + resp.Status + ", title " + strconv.Quote(title)
}

// checkTorrentURL fetches a result's .torrent the way the engine would and
// checks it's a bencoded dictionary, not an error page.
func checkTorrentURL(t *testing.T, u string) {
	t.Helper()
	resp, err := http.Get(u)
	if err != nil {
		t.Fatalf("torrent %s: %v", u, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK || !bytes.HasPrefix(b, []byte("d")) {
		t.Fatalf("torrent %s: %s, not a .torrent file", u, resp.Status)
	}
}
