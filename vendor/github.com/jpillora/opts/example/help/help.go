package main

import "github.com/jpillora/opts"

type HelpConfig struct {
	Zip  string `arg:"!" help:"zip is very lorem ipsum dolor sit amet, consectetur adipiscing elit. Phasellus at commodo odio. Sed id tincidunt purus. Cras vel felis dictum, lobortis metus a, tempus tellus"`
	Foo  string `help:"this is help for foo"`
	Bar  string `help:"and help for bar"`
	Fizz string `help:"Lorem ipsum dolor sit amet, consectetur adipiscing elit. Phasellus at commodo odio. Sed id tincidunt purus. Cras vel felis dictum, lobortis metus a, tempus tellus"`
	Buzz string `help:"and help for buzz"`
}

func main() {

	c := HelpConfig{
		Foo: "42",
	}

	opts.New(&c).
		Name("help").
		Version("1.0.0").
		Repo("https://github.com/jpillora/foo").
		Parse()
}
