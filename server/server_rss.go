package server

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/boypt/simple-torrent/common"
	"github.com/dustin/go-humanize"
	"github.com/mmcdole/gofeed"
)

var (
	magnetExp   = regexp.MustCompile(`magnet:[^< ]+`)
	hashinfoExp = regexp.MustCompile(`[0-9a-zA-Z]{40}`)
	torrentExp  = regexp.MustCompile(`\.torrent`)
)

type rssJSONItem struct {
	Name            string `json:"name"`
	Magnet          string `json:"magnet"`
	InfoHash        string `json:"infohash"`
	Published       string `json:"published"`
	URL             string `json:"url"`
	Torrent         string `json:"torrent"`
	Size            string `json:"size"`
	publishedParsed *time.Time
}

func (ritem *rssJSONItem) findFromFeedItem(i *gofeed.Item) (found bool) {

	for _, ex := range []string{"torrent", "nyaa"} {
		if ok := ritem.readExtention(i, ex); ok {
			break
		}
	}

	// some sites put it under enclosures
	for _, e := range i.Enclosures {
		if strings.HasPrefix(e.URL, "magnet:") {
			ritem.Magnet = e.URL
		} else if torrentExp.Match([]byte(e.URL)) {
			ritem.Torrent = e.URL
		}
	}

	// maybe the Link is a torrent file
	if torrentExp.MatchString(i.Link) {
		ritem.Torrent = i.Link
	}

	// not found magnet/torrent, try to find them in the description
	if ritem.Magnet == "" && ritem.InfoHash == "" && ritem.Torrent == "" {

		// try to find magnet in description
		if s := magnetExp.FindString(i.Description); s != "" {
			ritem.Magnet = s
		}

		// try to find hashinfo in description
		if s := hashinfoExp.FindString(i.Description); s != "" {
			ritem.InfoHash = s
		}

		//still not found?, well... whatever
	}

	return (ritem.Magnet != "" || ritem.InfoHash != "" || ritem.Torrent != "")
}

func (r *rssJSONItem) readExtention(i *gofeed.Item, ext string) (found bool) {

	// There are no starndards for rss feeds contains torrents or magnets
	// Heres some sites putting info in the extentions
	if etor, ok := i.Extensions[ext]; ok {

		if e, ok := etor["size"]; ok && len(e) > 0 {
			r.Size = e[0].Value
		}

		if e, ok := etor["contentLength"]; ok && len(e) > 0 {
			if size, err := strconv.ParseUint(e[0].Value, 10, 64); err == nil {
				r.Size = humanize.Bytes(size)
			}
		}

		if e, ok := etor["magnetURI"]; ok && len(e) > 0 {
			r.Magnet = e[0].Value
			found = true
		}

		if e, ok := etor["infoHash"]; ok && len(e) > 0 {
			r.InfoHash = e[0].Value
			found = true
		}
	}

	return
}

func (s *Server) updateRSS() {
	fp := gofeed.NewParser()
	fp.Client = &http.Client{
		Timeout: 60 * time.Second,
	}
	for _, rss := range strings.Split(s.engineConfig.RssURL, "\n") {
		if !strings.HasPrefix(rss, "http://") && !strings.HasPrefix(rss, "https://") {
			continue
		}
		rss = strings.TrimSpace(rss)
		feed, err := fp.ParseURL(rss)
		if err != nil {
			log.Printf("RSS: parse feed err %s", err.Error())
			continue
		}

		if s.Debug {
			log.Printf("RSS: retrived feed %s from %s", feed.Title, rss)
		}

		if oldmark, ok := s.rssMark[rss]; ok {
			var lastIdx int
			for i, item := range feed.Items {
				if item.GUID == oldmark {
					lastIdx = i
					break
				}
			}
			if lastIdx > 0 {
				log.Printf("RSS: feed updated with %d new items", lastIdx)
				s.rssMark[rss] = feed.Items[0].GUID
				s.rssCache = append(feed.Items[:lastIdx], s.rssCache...)
			}
		} else if len(feed.Items) > 0 {
			if s.Debug {
				log.Printf("RSS: retrive %d new items, first record", len(feed.Items))
			}
			s.rssMark[rss] = feed.Items[0].GUID
			s.rssCache = append(feed.Items, s.rssCache...)
		}

		if len(s.rssCache) > 200 {
			s.rssCache = s.rssCache[:200]
		}
	}

	// sort the retrived feed by Published attr
	// make sure the first is the latest
	sort.Slice(s.rssCache, func(i, j int) bool {
		return s.rssCache[i].PublishedParsed.After(*(s.rssCache[j].PublishedParsed))
	})
	if len(s.rssCache) > 0 {
		s.state.LatestRSSGuid = s.rssCache[0].GUID
		s.state.Push()
	}
}

func (s *Server) serveRSS(w http.ResponseWriter, r *http.Request) {

	if _, ok := r.URL.Query()["update"]; ok {
		s.updateRSS()
	}

	var results []rssJSONItem
	for _, i := range s.rssCache {
		ritem := rssJSONItem{
			Name:            i.Title,
			Published:       i.Published,
			URL:             i.Link,
			publishedParsed: i.PublishedParsed,
		}

		ritem.findFromFeedItem(i)
		results = append(results, ritem)
	}

	w.Header().Set("Content-Type", "application/json")
	common.HandleError(json.NewEncoder(w).Encode(results))
}
