package main

import (
	"fmt"

	"github.com/jpillora/opts"
)

type Config struct {
	Bazzes []string `min:"2"`
}

func main() {

	c := Config{}

	opts.New(&c).Parse()

	for i, foo := range c.Bazzes {
		fmt.Println(i, foo)
	}
}
