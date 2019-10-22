package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

type rssJSONItem struct {
	Name      string `json:"name,omitempty"`
	Magnet    string `json:"magnet,omitempty"`
	InfoHash  string `json:"hashinfo,omitempty"`
	Published string `json:"published,omitempty"`
	URL       string `json:"url,omitempty"`
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

		// sort the retrived feed by Published attr
		// make sure the first is the latest
		sort.Slice(feed.Items, func(i, j int) bool {
			return feed.Items[i].PublishedParsed.After(*(feed.Items[j].PublishedParsed))
		})

		s.rssLock.Lock()
		if olditems, ok := s.rssCache[rss]; ok && len(olditems) > 0 {
			var lastIdx int
			for i, item := range feed.Items {
				if item.GUID == olditems[0].GUID {
					lastIdx = i
					break
				}
			}
			if lastIdx > 0 {
				log.Printf("feed updated with %d new items", lastIdx)
				s.rssCache[rss] = append(feed.Items[:lastIdx], olditems...)
			}
		} else {
			if s.Debug {
				log.Printf("retrive %d new items, first record", len(feed.Items))
			}
			s.rssCache[rss] = feed.Items
		}
		s.rssLock.Unlock()
	}
}

func (s *Server) serveRSS(w http.ResponseWriter, r *http.Request) {

	if _, ok := r.URL.Query()["update"]; ok {
		s.updateRSS()
	}

	var results []rssJSONItem
	for _, rss := range strings.Split(s.state.Config.RssURL, "\n") {
		if !strings.HasPrefix(rss, "http") {
			continue
		}
		rss = strings.TrimSpace(rss)
		if items, ok := s.rssCache[rss]; ok {
			for _, i := range items {
				ritem := rssJSONItem{Name: i.Title, Published: i.Published, URL: i.Link}

				// not sure how the torrent feed standard is
				// here is get magnet from struct of https://eztv.io/ezrss.xml
				if len(i.Extensions) > 0 {
					if etor, ok := i.Extensions["torrent"]; ok {
						if lnk, ok := etor["magnetURI"]; ok {
							ritem.Magnet = lnk[0].Value
						}
						if ih, ok := etor["infoHash"]; ok {
							ritem.InfoHash = ih[0].Value
						}
					}
				} else {
					ritem.Magnet = i.Link
				}
				results = append(results, ritem)
			}
		}
	}

	b, err := json.Marshal(results)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}
