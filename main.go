package main

import (
	"log"

	"github.com/jpillora/cloud-torrent/cloudtorrent"
	"github.com/jpillora/opts"
)

var VERSION = "0.0.0-src" //set with ldflags

func main() {
	app := cloudtorrent.App{
		Title:      "Cloud Torrent",
		Port:       3000,
		ConfigPath: "cloud-torrent.json",
	}

	o := opts.New(&app)
	o.Version(VERSION)
	o.PkgRepo()
	o.LineWidth = 96
	o.Parse()

	if err := app.Run(VERSION); err != nil {
		log.Fatal(err)
	}
}
