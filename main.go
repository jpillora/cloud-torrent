package main

import (
	"log"

	"github.com/jpillora/cloud-torrent/server"
	"github.com/jpillora/opts"
)

var VERSION = "0.0.0" //set with ldflags

func main() {
	s := server.Server{
		Port:       3000,
		ConfigPath: "cloud-torrent.json",
	}

	opts.New(&s).
		Version(VERSION).
		PkgRepo().
		Parse()

	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
