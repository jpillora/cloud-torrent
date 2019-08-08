package main

import (
	"log"

	"github.com/jpillora/cloud-torrent/server"
	"github.com/jpillora/opts"
)

var VERSION = "0.0.0-src" //set with ldflags

func main() {
	s := server.Server{
		Title:      "SimpleTorrent",
		Port:       3000,
		ConfigPath: "cloud-torrent.json",
	}

	o := opts.New(&s)
	o.Version(VERSION)
	o.Repo("https://github.com/boypt/simple-torrent")
	o.PkgRepo()
	//o.LineWidth = 96
	o.Parse()

	if s.DisableLogTime {
		log.SetFlags(0)
	}

	log.Printf("############# SimpleTorrent ver[%s] #############\n", VERSION)
	if err := s.Run(VERSION); err != nil {
		log.Fatal(err)
	}
}
