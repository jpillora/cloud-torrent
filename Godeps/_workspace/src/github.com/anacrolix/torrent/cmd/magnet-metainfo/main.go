// Converts magnet URIs and info hashes into torrent metainfo files.
package main

import (
	"flag"
	"log"
	"os"
	"sync"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
)

func main() {
	flag.Parse()
	cl, err := torrent.NewClient(nil)
	if err != nil {
		log.Fatalf("error creating client: %s", err)
	}
	wg := sync.WaitGroup{}
	for _, arg := range flag.Args() {
		t, err := cl.AddMagnet(arg)
		if err != nil {
			log.Fatalf("error adding magnet to client: %s", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-t.GotInfo()
			mi := t.Metainfo()
			t.Drop()
			f, err := os.Create(mi.Info.Name + ".torrent")
			if err != nil {
				log.Fatalf("error creating torrent metainfo file: %s", err)
			}
			defer f.Close()
			err = bencode.NewEncoder(f).Encode(mi)
			if err != nil {
				log.Fatalf("error writing torrent metainfo file: %s", err)
			}
		}()
	}
	wg.Wait()
}
