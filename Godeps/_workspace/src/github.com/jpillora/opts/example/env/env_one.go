package main

import (
	"fmt"

	"github.com/jpillora/opts"
)

type Config2 struct {
	Foo string `env:"MY_FOO_VAR"`
	Bar string
}

func main() {

	c := Config2{}

	//UseEnv() essentially adds an `env` tag on all fields,
	//infering the env var name from the field name.
	//Specifically adding the `env` tag will only enable it
	//for a single field.
	opts.New(&c).Parse()

	fmt.Println(c.Foo)
	fmt.Println(c.Bar)
}
