## simple example

<tmpl,code=go:cat simple.go>
``` go 
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
```
</tmpl>
```
$ simple --foo hello --bar world
```
<tmpl,code:go run simple.go --foo hello --bar world>
``` plain 
hello
world
```
</tmpl>
```
$ simple --help
```
<tmpl,code:go run simple.go --help>
``` plain 

  Usage: simple [options]

  Options:
  --foo, -f
  --bar, -b
  --help, -h

```
</tmpl>