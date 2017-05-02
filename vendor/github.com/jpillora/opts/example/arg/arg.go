package main

import (
	"fmt"

	"github.com/jpillora/opts"
)

type Config struct {
	Foo string `type:"arg" help:"foo is a very important argument"`
	Bar string
}

func main() {

	c := Config{}

	opts.New(&c).Parse()

	fmt.Println(c.Foo)
	fmt.Println(c.Bar)
}
