package main

import (
	"log"
	"flag"

	"github.com/jpillora/cloud-torrent/server"
	"github.com/jpillora/opts"
)

var VERSION = "0.0.0-src" //set with ldflags

func main() {
	hostName := flag.String("h", "",  "Host name for SSL")		
	flag.Parse()

	s := server.Server{
		Title:      "Cloud Torrent",
		Port:       3000,
		ConfigPath: "cloud-torrent.json",		
	}

	if hostName != "" {
		s.HostName = *hostName
	}

	o := opts.New(&s)
	o.Version(VERSION)
	o.PkgRepo()
	o.LineWidth = 96
	o.Parse()

	if err := s.Run(VERSION); err != nil {
		log.Fatal(err)
	}
}
