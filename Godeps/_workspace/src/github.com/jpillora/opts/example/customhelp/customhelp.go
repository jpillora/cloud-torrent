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

	//see default templates and the default template order
	//in the opts/help.go file
	o := opts.New(&c).
		DocAfter("usage", "mytext", "\nthis is a some text!\n"). //add new entry
		Repo("myfoo.com/bar").
		DocSet("repo", "\nMy awesome repo:\n  {{.Repo}}"). //change existing entry
		Parse()

	fmt.Println(o.Help())
}
