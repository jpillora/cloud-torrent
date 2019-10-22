package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

type rssItem struct {
	Name      string `json:"name,omitempty"`
	Magnet    string `json:"magnet,omitempty"`
	Published string `json:"published,omitempty"`
}

func (s *Server) updateRSS() {
	fp := gofeed.NewParser()
	fp.Client = &http.Client{
		Timeout: 10 * time.Second,
	}
	for _, rss := range strings.Split(s.state.Config.RssURL, "\n") {
		if !strings.HasPrefix(rss, "http://") && !strings.HasPrefix(rss, "https://") {
			log.Printf("parse feed addr Invalid %s", rss)
			continue
		}
		rss = strings.TrimSpace(rss)

		feed, err := fp.ParseURL(rss)
		if err != nil {
			log.Printf("parse feed err %s", err.Error())
			continue
		}
		if s.Debug {
			log.Printf("retrived feed %s from %s", feed.Title, rss)
		}

		if olditems, ok := s.rssCache[rss]; !ok {
			if s.Debug {
				log.Printf("retrive %d new items, first record", len(feed.Items))
			}
			s.rssCache[rss] = feed.Items
		} else {
			if olditems[0].GUID != feed.Items[0].GUID {
				var newitems []*gofeed.Item
				for _, i := range feed.Items {
					if i.GUID == olditems[0].GUID {
						break
					}
					newitems = append(newitems, i)
				}
				log.Printf("feed updated %d new items", len(newitems))
				updatedItems := append(newitems, olditems...)
				s.rssCache[rss] = updatedItems
			}
		}
	}
}

func (s *Server) serveRSS(w http.ResponseWriter, r *http.Request) {

	if _, ok := r.URL.Query()["update"]; ok {
		s.updateRSS()
	}

	var results []rssItem
	for _, rss := range strings.Split(s.state.Config.RssURL, "\n") {
		if !strings.HasPrefix(rss, "http") {
			continue
		}
		rss = strings.TrimSpace(rss)
		if items, ok := s.rssCache[rss]; ok {
			for _, i := range items {
				results = append(results, rssItem{Name: i.Title, Magnet: i.Link, Published: i.Published})
			}
		}
	}

	b, err := json.Marshal(results)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}
