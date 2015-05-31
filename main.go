package main

import (
	"log"

	"github.com/jpillora/cloud-torrent/ct"
	"github.com/jpillora/opts"
)

var VERSION = "0.0.0"

func main() {
	s := ct.Server{
		Port: 3000,
	}

	opts.New(&s).
		Version(VERSION).
		PkgRepo().
		Parse()

	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
