## args example

<tmpl,code=go:cat args.go>
``` go 
package main

import (
	"fmt"

	"github.com/jpillora/opts"
)

type Config struct {
	Bazzes []string `min:"2"`
}

func main() {

	c := Config{}

	opts.New(&c).Parse()

	for i, foo := range c.Bazzes {
		fmt.Println(i, foo)
	}
}
```
</tmpl>
```
$ args --foo hello --bar world
```
<tmpl,code:go run args.go foo bar>
``` plain 
0 foo
1 bar
```
</tmpl>
```
$ args --help
```
<tmpl,code:go run args.go --help>
``` plain 

  Usage: args [options] bazzes...

  Options:
  --help, -h

```
</tmpl>