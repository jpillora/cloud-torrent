package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

func main() {
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "%s\n", "torrent-magnet: unexpected positional arguments")
		os.Exit(2)
	}
	mi, err := metainfo.Load(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading metainfo from stdin: %s", err)
		os.Exit(1)
	}
	ts := torrent.TorrentSpecFromMetaInfo(mi)
	m := torrent.Magnet{
		InfoHash: ts.InfoHash,
		Trackers: func() (ret []string) {
			for _, tier := range ts.Trackers {
				for _, tr := range tier {
					ret = append(ret, tr)
				}
			}
			return
		}(),
		DisplayName: ts.DisplayName,
	}
	fmt.Fprintf(os.Stdout, "%s\n", m.String())
}
