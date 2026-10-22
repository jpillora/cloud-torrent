package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jpillora/backoff"
	"github.com/jpillora/scraper/scraper"
)

// searchConfigURL is the canonical, remotely-updated scraper provider config.
// It is fetched periodically so providers can be fixed without a redeploy.
const searchConfigURL = "https://gist.githubusercontent.com/jpillora/4d945b46b3025843b066adf3d685be6b/raw/scraper-config.json"

// userAgent is sent to providers so they treat us like a regular browser.
const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/57.0.2987.133 Safari/537.36"

// Expose only provider queries, never the dependency's config administration.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "search queries require GET", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if parts[0] == "" || len(parts) > 2 || (len(parts) == 2 && parts[1] != "item") {
		http.NotFound(w, r)
		return
	}
	s.searchMu.RLock()
	h := s.scraper
	s.searchMu.RUnlock()
	h.ServeHTTP(w, r)
}

func newSearchHandler(config []byte) (*scraper.Handler, error) {
	h := &scraper.Handler{Log: false, Debug: false}
	if err := h.LoadConfig(config); err != nil {
		return nil, err
	}
	// scraper v0.3.0 only inherits handler headers when logging is enabled.
	// Populate each immutable endpoint explicitly, preserving provider overrides.
	for _, endpoint := range h.Config {
		if endpoint == nil {
			continue
		}
		headers := http.Header{}
		for k, v := range endpoint.Headers {
			headers.Set(k, v)
		}
		if headers.Get("User-Agent") == "" {
			headers.Set("User-Agent", userAgent)
		}
		endpoint.Headers = make(map[string]string, len(headers))
		for k := range headers {
			endpoint.Headers[k] = headers.Get(k)
		}
	}
	return h, nil
}

// startSearch builds the scraper handler from the embedded default config,
// publishes the providers into the synced state, and kicks off the background
// refresh loop. Mounted in handler() at /search.
func (s *Server) startSearch() error {
	h, err := newSearchHandler(defaultSearchConfig)
	if err != nil {
		return err
	}
	s.scraper = h
	// baseline for the change-detection in fetchSearchConfig
	s.searchConfig, _ = normalize(defaultSearchConfig)
	s.state.Lock()
	s.state.SearchProviders = s.scraper.Config
	s.state.Unlock()
	go s.fetchSearchConfigLoop()
	return nil
}

// fetchSearchConfigLoop refreshes the remote provider config every 30 minutes,
// backing off (up to 30 min) on error.
func (s *Server) fetchSearchConfigLoop() {
	b := backoff.Backoff{Max: 30 * time.Minute}
	for {
		if err := s.fetchSearchConfig(); err != nil {
			time.Sleep(b.Duration())
		} else {
			time.Sleep(30 * time.Minute)
			b.Reset()
		}
	}
}

// fetchSearchConfig pulls the remote config; if it differs from the currently
// loaded one, it reloads the scraper and republishes the providers.
func (s *Server) fetchSearchConfig() error {
	newConfig, err := s.fetchBytes(context.Background(), searchConfigURL, 1<<20)
	if err != nil {
		return err
	}
	newConfig, err = normalize(newConfig)
	if err != nil {
		return err
	}
	s.searchMu.RLock()
	unchanged := bytes.Equal(s.searchConfig, newConfig)
	s.searchMu.RUnlock()
	if unchanged {
		return nil // unchanged
	}
	h, err := newSearchHandler(newConfig)
	if err != nil {
		return err
	}
	s.searchMu.Lock()
	s.scraper = h
	s.searchConfig = newConfig
	s.searchMu.Unlock()
	s.state.Lock()
	s.state.SearchProviders = h.Config
	s.state.Unlock()
	s.state.Push()
	log.Printf("Loaded new search providers")
	return nil
}

// normalize pretty-prints JSON so two configs can be compared byte-for-byte.
func normalize(input []byte) ([]byte, error) {
	out := bytes.Buffer{}
	if err := json.Indent(&out, input, "", "  "); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// defaultSearchConfig is the built-in scraper provider config (see
// github.com/jpillora/scraper for the spec). "<id>/item" handlers resolve a
// single result's magnet/infohash from its detail page.
var defaultSearchConfig = []byte(`{
	"zq": {
		"name": "Zooqle",
		"url": "https://zooqle.com/search?q={{query}}&pg={{page:1}}&s=ns&v=t&sd=d",
		"list": "#body_container .panel-body > table tbody tr",
		"result": {
			"name": "td:nth-child(2) a",
			"url": ["td:nth-child(2) a", "@href"],
			"magnet": ["a[title=Magnet\\ link]", "@href"],
			"seeds": "td:nth-child(6) .progress-bar:nth-child(1)",
			"peers": "td:nth-child(6) .progress-bar:nth-child(2)"
		}
	},
	"rbg": {
		"name": "RARBG",
		"url": "https://rarbg.to/torrents.php?search={{query}}&order=seeders&by=DESC&page={{page:1}}",
		"list": "table.lista2t tr.lista2",
		"result": {
			"name":["td:nth-child(2) > a[title]"],
			"torrent":["td:nth-child(2) > a[title]","@href","s~/torrent/~~","s~^~https://rarbg.to/download.php?f=file.torrent&id=~"],
			"size": "td:nth-child(4)",
			"seeds": "td:nth-child(5)",
			"peers": "td:nth-child(6)"
		}
	},
	"eztv": {
		"name": "EZTV",
		"url": "https://eztv.ag/search/{{query}}",
		"list": "table tr.forum_header_border",
		"result": {
			"name": "td:nth-child(2) a",
			"url": ["td:nth-child(2) a", "@href"],
			"magnet": ["td:nth-child(3) a:nth-child(1)", "@href"],
			"size": "td:nth-child(4)",
			"seeds": "td:nth-child(6)"
		}
	},
	"1337x": {
		"name": "1337X",
		"url": "http://1337x.to/sort-search/{{query}}/seeders/desc/{{page:1}}/",
		"list": ".box-info-detail table.table tr",
		"result": {
			"name":[".coll-1 a:nth-child(2)"],
			"url":[".coll-1 a:nth-child(2)", "@href"],
			"seeds": ".coll-2",
			"peers": ".coll-3",
			"size": [".coll-4", "/([\\d\\.]+ [KMGT]?B)/"]
		}
	},
	"1337x/item": {
		"name": "1337X (Item)",
		"url": "http://1337x.to{{item}}",
		"result": {
			"magnet": [".download-links-dontblock a.btn","@href"]
		}
	},
	"abb": {
		"name": "The Audiobook Bay",
		"url": "http://audiobookbay.me/page/{{page:1}}?s={{query}}",
		"list": "#content > div",
		"result": {
			"name":["div.postTitle > h2 > a","@title"],
			"url":["div.postTitle > h2 > a","@href"],
			"seeds": "div.postContent > p:nth-child(3) > span:nth-child(1)",
			"peers": "div.postContent > p:nth-child(3) > span:nth-child(3)"
		}
	},
	"abb/item": {
		"name": "The Audiobook Bay (Item)",
		"url": "http://audiobookbay.me{{item}}",
		"result": {
			"infohash": "/td>([a-f0-9]+)</",
			"tracker": "table tr td:nth-child(2)"
		}
	},
	"tpb": {
		"name": "The Pirate Bay",
		"url": "https://thepiratebay.org/search/{{query}}/{{page:0}}/7//",
		"list": "#searchResult > tbody > tr",
		"result": {
			"name":"a.detLink",
			"path":["a.detLink","@href"],
			"magnet": ["a[title=Download\\ this\\ torrent\\ using\\ magnet]","@href"],
			"size": "/Size (\\d+(\\.\\d+).[KMG]iB)/",
			"seeds": "td:nth-child(3)",
			"peers": "td:nth-child(4)"
		}
	}
}`)
