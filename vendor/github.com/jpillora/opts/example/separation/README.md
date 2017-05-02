## library separation

*`main.go`*

<tmpl,code=go:cat main.go>
``` go
package main

import (
	"log"

	"github.com/jpillora/opts"
	"github.com/jpillora/opts/example/separation/foo"
)

//set this via ldflags
var VERSION = "0.0.0"

func main() {
	//configuration with defaults
	c := foo.Config{
		Ping: "hello",
		Pong: "world",
	}
	//parse config, note the library version, and extract the
	//repository link from the config package import path
	opts.New(&c).
		Name("foo").
		Version(VERSION).
		PkgRepo(). //includes the infered URL to the package in the help text
		Parse()
	//construct a foo
	f, err := foo.New(c)
	if err != nil {
		log.Fatal(err)
	}
	//ready! run foo!
	f.Run()
}
```
</tmpl>

*`foo/foo.go`*

<tmpl,code=go:cat foo/foo.go>
``` go
package foo

//this Config struct can used both by opts to parse CLI input
//and by library users who wish to use this code in their programs
type Config struct {
	Ping string
	Pong string
	Zip  int
	Zop  int
}

//use a Config value, not Config pointer.
//this prevents future modification from outside the library.
func New(c Config) (*Foo, error) {
	//ensure proper initialization of Foo
	foo := &Foo{
		c:    c,
		bar:  42 + c.Zip,
		bazz: 21 + c.Zop,
	}

	return foo, nil
}

type Foo struct {
	//internal config
	c Config
	//internal state
	bar  int
	bazz int
}

func (f *Foo) Run() {
	println("Foo is running...")
}
```
</tmpl>


```sh
# build program and set VERSION at compile time
$ go build -ldflags "-X main.VERSION=0.2.6" -o foo
$ ./foo --help
```
<tmpl,code: go build -ldflags "-X main.VERSION=0.2.6" -o tmp && ./tmp --help && rm tmp>
``` plain

  Usage: foo [options]

  Options:
  --ping, -p     default hello
  --pong         default world
  --zip, -z
  --zop
  --help, -h
  --version, -v

  Version:
    0.2.6

  Read more:
    https://github.com/jpillora/opts

```
</tmpl>
