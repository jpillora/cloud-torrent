package main

import (
	"fmt"

	"github.com/jpillora/opts"
)

type Config struct {
	Foo string
	Bar string
}

func main() {

	c := Config{}

	opts.New(&c).
		ConfigPath("config.json").
		Parse()

	fmt.Println(c.Foo)
	fmt.Println(c.Bar)
}
