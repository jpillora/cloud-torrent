package main

import (
	"fmt"

	"github.com/jpillora/opts"
)

type FooConfig struct {
	Ping string
	Pong string
}

//config
type Config struct {
	Cmd string `type:"cmdname"`
	//command (external struct)
	Foo FooConfig
	//command (inline struct)
	Bar struct {
		Zip string
		Zap string
	}
}

func main() {

	c := Config{}

	opts.Parse(&c)

	fmt.Println(c.Cmd)
	fmt.Println(c.Bar.Zip)
	fmt.Println(c.Bar.Zap)
}
