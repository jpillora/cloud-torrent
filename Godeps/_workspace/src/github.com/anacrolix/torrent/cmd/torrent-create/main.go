package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/docopt/docopt-go"

	"github.com/anacrolix/torrent/metainfo"
)

var (
	builtinAnnounceList = [][]string{
		{"udp://tracker.openbittorrent.com:80"},
		{"udp://tracker.publicbt.com:80"},
		{"udp://tracker.istole.it:6969"},
	}
)

func main() {
	opts, err := docopt.Parse("Usage: torrent-create <root>", nil, true, "", true)
	if err != nil {
		panic(err)
	}
	root := opts["<root>"].(string)
	mi := metainfo.MetaInfo{
		AnnounceList: builtinAnnounceList,
	}
	mi.SetDefaults()
	err = mi.Info.BuildFromFilePath(root)
	if err != nil {
		log.Fatal(err)
	}
	err = mi.Info.GeneratePieces(func(fi metainfo.FileInfo) (io.ReadCloser, error) {
		return os.Open(filepath.Join(root, strings.Join(fi.Path, string(filepath.Separator))))
	})
	if err != nil {
		log.Fatalf("error generating pieces: %s", err)
	}
	err = mi.Write(os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
}
