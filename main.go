package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/boypt/simple-torrent/server"
	"github.com/jpillora/opts"
)

var VERSION = "0.0.0-src" //set with ldflags

func main() {
	s := server.Server{
		Title:  "SimpleTorrent",
		Port:   3000, // depreciated
		Listen: ":3000",
	}

	o := opts.New(&s)
	o.Version(VERSION)
	o.Repo("https://github.com/boypt/simple-torrent")
	o.PkgRepo()
	o.SetLineWidth(96)
	o.Parse()

	t := &server.TPLInfo{
		Title:   s.Title,
		Version: VERSION,
		Runtime: fmt.Sprintf("%s %d bit", runtime.Version(), strconv.IntSize),
		Uptime:  time.Now().Unix(),
	}

	if s.DisableLogTime {
		log.SetFlags(0)
	}

	log.Print(t.GetInfo())
	if err := s.Run(t); err != nil {
		if errors.Is(err, server.ErrDiskSpace) {
			log.Println(err)
			os.Exit(42)
		}
		log.Fatal(err)
	}
}
