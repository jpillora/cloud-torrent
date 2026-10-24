package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
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
	if h == nil {
		http.NotFound(w, r)
		return
	}
	e := h.Endpoint(strings.TrimPrefix(r.URL.Path, "/"))
	if e == nil {
		http.NotFound(w, r)
		return
	}
	s.executeSearch(w, r, e, h.options[strings.TrimPrefix(r.URL.Path, "/")])
}

// startSearch builds the scraper handler from the embedded default config,
// publishes the providers into the synced state, and kicks off the background
// refresh loop. Mounted in handler() at /search.
func (s *Server) startSearch() error {
	config := defaultSearchConfig
	if s.SearchConfigPath != "" {
		f, err := os.Open(s.SearchConfigPath)
		if err != nil {
			return err
		}
		config, err = io.ReadAll(io.LimitReader(f, (1<<20)+1))
		f.Close()
		if err != nil {
			return err
		}
	}
	h, err := newSearchHandler(config)
	if err != nil {
		return err
	}
	s.scraper = h
	// baseline for the change-detection in fetchSearchConfig
	s.searchConfig, _ = normalize(config)
	s.state.Lock()
	s.state.SearchProviders = searchDescriptors(s.scraper.Config)
	s.state.Unlock()
	if s.SearchConfigPath == "" {
		go s.fetchSearchConfigLoop()
	}
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
	for _, opt := range h.options {
		if len(opt.Mirrors) > 0 {
			return errors.New("mirrors require a private operator search configuration")
		}
	}
	s.searchMu.Lock()
	s.scraper = h
	s.searchConfig = newConfig
	s.searchMu.Unlock()
	s.state.Lock()
	s.state.SearchProviders = searchDescriptors(h.Config)
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

// Only presentation data crosses the realtime boundary. Endpoint headers,
// request bodies and query/passkey-bearing templates remain server-side.
func searchDescriptors(config scraper.Config) scraper.Config {
	out := scraper.Config{}
	for id, e := range config {
		if e == nil {
			continue
		}
		origin := ""
		if u, err := url.Parse(e.URL); err == nil {
			origin = (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
		}
		out[id] = &scraper.Endpoint{Name: e.Name, URL: origin}
	}
	return out
}
