package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/anacrolix/torrent/metainfo"
)

func main() {
	name := flag.Bool("name", false, "print name")
	flag.Parse()
	for _, filename := range flag.Args() {
		metainfo, err := metainfo.LoadFromFile(filename)
		if err != nil {
			log.Print(err)
			continue
		}
		if *name {
			fmt.Printf("%s\n", metainfo.Info.Name)
			continue
		}
		d := map[string]interface{}{
			"Name":      metainfo.Info.Name,
			"NumPieces": metainfo.Info.NumPieces(),
		}
		b, _ := json.MarshalIndent(d, "", "  ")
		os.Stdout.Write(b)
	}
	if !*name {
		os.Stdout.WriteString("\n")
	}
}
