package main

import (
	"log"

	"github.com/jpillora/opts"
	"github.com/jpillora/opts/example/separation/foo"
)

//set this via ldflags
var VERSION = "0.0.0"

func main() {
	//configuration with defaults
	c := foo.Config{
		Ping: "hello",
		Pong: "world",
	}
	//parse config, note the library version, and extract the
	//repository link from the config package import path
	opts.New(&c).
		Name("foo").
		Version(VERSION).
		PkgRepo(). //includes the infered URL to the package in the help text
		Parse()
	//construct a foo
	f, err := foo.New(c)
	if err != nil {
		log.Fatal(err)
	}
	//ready! run foo!
	f.Run()
}
