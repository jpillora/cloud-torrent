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
	//In this case UseEnv() is equivalent to
	//adding `env:"FOO"` and `env:"BAR"` tags
	opts.New(&c).UseEnv().Parse()
	fmt.Println(c.Foo)
	fmt.Println(c.Bar)
}
