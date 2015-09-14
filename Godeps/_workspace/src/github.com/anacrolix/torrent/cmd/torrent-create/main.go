package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"runtime"

	torrent "github.com/anacrolix/torrent/metainfo"
)

var (
	builtinAnnounceList = [][]string{
		{"udp://tracker.openbittorrent.com:80"},
		{"udp://tracker.publicbt.com:80"},
		{"udp://tracker.istole.it:6969"},
	}
)

func init() {
	flag.Parse()
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	b := torrent.Builder{}
	for _, filename := range flag.Args() {
		if err := filepath.Walk(filename, func(path string, info os.FileInfo, err error) error {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return err
			}
			log.Print(path)
			if info.IsDir() {
				return nil
			}
			b.AddFile(path)
			return nil
		}); err != nil {
			log.Print(err)
		}
	}
	for _, group := range builtinAnnounceList {
		b.AddAnnounceGroup(group)
	}
	batch, err := b.Submit()
	if err != nil {
		log.Fatal(err)
	}
	errs, status := batch.Start(os.Stdout, runtime.NumCPU())
	lastProgress := int64(-1)
	for {
		select {
		case err, ok := <-errs:
			if !ok || err == nil {
				return
			}
			log.Print(err)
		case bytesDone := <-status:
			progress := 100 * bytesDone / batch.TotalSize()
			if progress != lastProgress {
				log.Printf("%d%%", progress)
				lastProgress = progress
			}
		}
	}
}
