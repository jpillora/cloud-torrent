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

func (s *Server) serveRSS(w http.ResponseWriter, r *http.Request) {

	var results []rssItem
	var errs []error
	fp := gofeed.NewParser()
	fp.Client = &http.Client{
		Timeout: 10 * time.Second,
	}
	for _, rss := range strings.Split(s.state.Config.RssURL, "\n") {
		if !strings.HasPrefix(rss, "http") {
			continue
		}

		rss = strings.TrimSpace(rss)

		log.Printf("retrive feed %s", rss)
		feed, err := fp.ParseURL(rss)
		if err != nil {
			log.Printf("parse feed err %s", err.Error())
			errs = append(errs, err)
			continue
		}
		for _, i := range feed.Items {
			results = append(results, rssItem{Name: i.Title, Magnet: i.Link, Published: i.Published})
		}
	}

	if len(results) == 0 {
		var estr []string
		for _, e := range errs {
			estr = append(estr, e.Error())
		}
		if len(estr) == 0 {
			estr = append(estr, "RssURL is not configured")
		}
		http.Error(w, strings.Join(estr, "|\n"), http.StatusInternalServerError)
		return
	}

	b, err := json.Marshal(results)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}
