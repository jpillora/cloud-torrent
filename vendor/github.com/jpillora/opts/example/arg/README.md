## arg example

<tmpl,code=go:cat arg.go>
``` go 
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
```
</tmpl>
```
$ arg --foo hello --bar world
```
<tmpl,code:go run arg.go --foo hello --bar world>
``` plain 

  Usage: arg [options] <foo>

  foo is a very important argument

  Options:
  --bar, -b
  --help, -h

  Error:
    flag provided but not defined: -foo

```
</tmpl>
```
$ arg --help
```
<tmpl,code:go run arg.go --help>
``` plain 

  Usage: arg [options] <foo>

  foo is a very important argument

  Options:
  --bar, -b
  --help, -h

```
</tmpl>