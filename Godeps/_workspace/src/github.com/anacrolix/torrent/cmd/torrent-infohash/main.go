package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/anacrolix/torrent/metainfo"
)

func main() {
	flag.Parse()
	for _, arg := range flag.Args() {
		mi, err := metainfo.LoadFromFile(arg)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%x: %s\n", mi.Info.Hash, arg)
	}
}
