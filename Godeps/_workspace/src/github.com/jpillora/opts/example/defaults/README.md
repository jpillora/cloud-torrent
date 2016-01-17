## defaults example

<tmpl,code=go:cat defaults.go>
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

	c := Config{
		Bar: "moon",
	}

	opts.Parse(&c)

	fmt.Println(c.Foo)
	fmt.Println(c.Bar)
}
```
</tmpl>
```
$ defaults --foo hello
```
<tmpl,code:go run defaults.go --foo hello>
``` plain 
hello
moon
```
</tmpl>
```
$ defaults --help
```
<tmpl,code:go run defaults.go --help>
``` plain 

  Usage: defaults [options]

  Options:
  --foo, -f
  --bar, -b   default moon
  --help, -h

```
</tmpl>