package server

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/jpillora/backoff"
)

const searchConfigURL = "https://gist.githubusercontent.com/jpillora/4d945b46b3025843b066adf3d685be6b/raw/scraper-config.json"

func (s *Server) fetchSearchConfigLoop() {
	b := backoff.Backoff{Max: 30 * time.Minute}
	for {
		if err := s.fetchSearchConfig(); err != nil {
			//ignore error
			time.Sleep(b.Duration())
		} else {
			//no errror - check again in half hour
			time.Sleep(30 * time.Minute)
			b.Reset()
		}
	}
}

var currentConfig = defaultSearchConfig

func (s *Server) fetchSearchConfig() error {
	resp, err := http.Get(searchConfigURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	newConfig, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if bytes.Equal(currentConfig, newConfig) {
		return nil //skip
	}
	if err := s.scraper.LoadConfig(newConfig); err != nil {
		return err
	}
	s.state.SearchProviders = s.scraper.Config
	s.state.Update()
	log.Printf("Loaded new search providers")
	currentConfig = newConfig
	return nil
}

//see github.com/jpillora/scraper for config specification
//cloud-torrent uses "<id>-item" handlers
var defaultSearchConfig = []byte(`{
	"et": {
		"name": "ExtraTorrent",
		"url": "https://extratorrent.cc/search/?search={{query}}&s_cat=&pp=&srt=seeds&order=desc&page={{page:1}}",
		"list": "table.tl tr.tlr",
		"result": {
			"name":["td.tli > a"],
			"torrent": ["td:nth-child(1) a","@href","s/torrent_download/download/"],
			"url": ["td.tli > a","@href"],
			"size": "td:nth-child(5)",
			"seeds": "td.sy",
			"peers": "td.ly"
		}
	},
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
			"magent": ["td:nth-child(3) a:nth-child(1)", "@href"],
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
			"magnet": ["a.btn-magnet","@href"]
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
	}
}`)
