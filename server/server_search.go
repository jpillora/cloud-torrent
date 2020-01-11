package server

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
)

var fetches = 0
var currentConfig, _ = normalize(defaultSearchConfig)

func (s *Server) fetchSearchConfig(confurl string) error {
	log.Println("fetchSearchConfig: loading search config from", confurl)
	resp, err := http.Get(confurl)
	if err != nil {
		log.Println("[fetchSearchConfig]", err)
		return err
	}
	defer resp.Body.Close()
	newConfig, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	newConfig, err = normalize(newConfig)
	if err != nil {
		return err
	}
	fetches++
	if bytes.Equal(currentConfig, newConfig) {
		return nil //skip
	}
	if err := s.scraper.LoadConfig(newConfig); err != nil {
		return err
	}
	s.state.SearchProviders = s.scraper.Config
	s.state.Push()
	currentConfig = newConfig
	log.Printf("Loaded new search providers")
	return nil
}

func normalize(input []byte) ([]byte, error) {
	output := bytes.Buffer{}
	if err := json.Indent(&output, input, "", "  "); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

//see github.com/jpillora/scraper for config specification
//cloud-torrent uses "<id>-item" handlers
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
