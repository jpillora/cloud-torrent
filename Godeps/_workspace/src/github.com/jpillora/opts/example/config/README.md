## config example

<tmpl,code=go:cat config.go>
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

	opts.New(&c).
		ConfigPath("config.json").
		Parse()

	fmt.Println(c.Foo)
	fmt.Println(c.Bar)
}
```
</tmpl>

<tmpl,code=json:cat config.json>
``` json 
{
	"foo": "hello",
	"bar": "world"
}
```
</tmpl>

```
$ config --bar moon
```
<tmpl,code:go run config.go --bar moon>
``` plain 
hello
moon
```
</tmpl>
```
$ config --help
```
<tmpl,code:go run config.go --help>
``` plain 

  Usage: config [options]

  Options:
  --foo, -f
  --bar, -b
  --help, -h

```
</tmpl>