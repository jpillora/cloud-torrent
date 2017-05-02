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
	opts.Parse(&c)
	fmt.Println(c.Foo)
	fmt.Println(c.Bar)
}
