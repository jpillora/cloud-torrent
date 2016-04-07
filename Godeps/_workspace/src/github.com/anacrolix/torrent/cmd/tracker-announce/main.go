package main

import (
	"flag"
	"log"
	"math"
	"strings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/tracker"
)

func argSpec(arg string) (ts *torrent.TorrentSpec, err error) {
	if strings.HasPrefix(arg, "magnet:") {
		return torrent.TorrentSpecFromMagnetURI(arg)
	}
	mi, err := metainfo.LoadFromFile(arg)
	if err != nil {
		return
	}
	ts = torrent.TorrentSpecFromMetaInfo(mi)
	return
}

func main() {
	flag.Parse()
	ar := tracker.AnnounceRequest{
		NumWant: -1,
		Left:    math.MaxUint64,
	}
	for _, arg := range flag.Args() {
		ts, err := argSpec(arg)
		if err != nil {
			log.Fatal(err)
		}
		ar.InfoHash = ts.InfoHash
		for _, tier := range ts.Trackers {
			for _, tURI := range tier {
				resp, err := tracker.Announce(tURI, &ar)
				if err != nil {
					log.Print(err)
					continue
				}
				log.Printf("%q: %#v", tURI, resp)
			}
		}
	}
}
