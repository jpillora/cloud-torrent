package main

import (
	"fmt"

	"github.com/jpillora/opts"
)

func main() {
	config := struct {
		File  string `help:"file to load"`
		Lines int    `help:"number of lines to show"`
	}{}
	opts.Parse(&config)
	fmt.Println(config)
}
