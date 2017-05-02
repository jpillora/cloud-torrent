package main

import (
	"fmt"
	"log"

	"github.com/anacrolix/tagflag"

	"github.com/anacrolix/torrent/metainfo"
)

func main() {
	var args struct {
		tagflag.StartPos
		Files []string `arity:"+" type:"pos"`
	}
	tagflag.Parse(&args)
	for _, arg := range args.Files {
		mi, err := metainfo.LoadFromFile(arg)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: %s\n", mi.HashInfoBytes().HexString(), arg)
	}
}
