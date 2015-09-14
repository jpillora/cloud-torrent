package main

import (
	"log"

	"github.com/jpillora/opts"
	"github.com/jpillora/opts/example/separation/lib"
)

//set this via ldflags
var VERSION = "0.0.0"

func main() {
	//configuration with defaults
	c := lib.Config{
		Ping: "!",
		Pong: "?",
	}
	//parse config, note the library version, and extract the
	//repository link from the config package import path
	opts.New(&c).
		Name("foo"). //explicitly name (otherwise it will use the project name from the pkg import path)
		Version(VERSION).
		PkgRepo().
		Parse()
	//construct a foo
	foo, err := lib.NewFoo(c)
	if err != nil {
		log.Fatal(err)
	}
	//ready! run foo!
	foo.Run()
}
