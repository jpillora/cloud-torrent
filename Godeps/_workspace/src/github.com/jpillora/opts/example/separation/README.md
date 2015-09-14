## library separation

*`main.go`*

<tmpl,code=go:cat main.go>
``` go 
package main

import (
	"log"

	"github.com/jpillora/opts"
	"github.com/jpillora/opts/example/separation/lib"
)

//set this via ldflags
var VERSION = "0.0.0"

func main() {
	//configuration with defaults
	c := lib.Config{
		Ping: "!",
		Pong: "?",
	}
	//parse config, note the library version, and extract the
	//repository link from the config package import path
	opts.New(&c).
		Name("foo"). //explicitly name (otherwise it will use the project name from the pkg import path)
		Version(VERSION).
		PkgRepo().
		Parse()
	//construct a foo
	foo, err := lib.NewFoo(c)
	if err != nil {
		log.Fatal(err)
	}
	//ready! run foo!
	foo.Run()
}
```
</tmpl>

*`lib/foo.go`*

<tmpl,code=go:cat lib/foo.go>
``` go 
package lib

import "errors"

//this Config struct can used both by opts to parse CLI input
//and by library users who wish to use this code in their programs
type Config struct {
	Ping string
	Pong string
	Zip  int
	Zop  int
}

//use a Config value, not Config pointer.
//this prevents modification from outside the library.
func NewFoo(c Config) (*Foo, error) {
	//validate config
	if c.Zip < 7 {
		return nil, errors.New("Zip too small!")
	}
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


```
$ go build -ldflags "-X main.VERSION 0.2.6" -o foo
$ ./foo --help
```
<tmpl,code: go build -ldflags "-X main.VERSION 0.2.6" -o foo && ./foo --help && rm foo>
``` plain 

  Usage: foo [options]
  
  Options:
  --ping, -p     default !
  --pong         default ?
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