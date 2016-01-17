## env example

<tmpl,code=go:cat env.go>
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
	//In this case UseEnv() is equivalent to
	//adding `env:"FOO"` and `env:"BAR"` tags
	opts.New(&c).UseEnv().Parse()
	fmt.Println(c.Foo)
	fmt.Println(c.Bar)
}
```
</tmpl>
```
$ export FOO=hello
$ export BAR=world
$ go run env.go
```
<tmpl,code:(export FOO=hello && export BAR=world && go run env.go)>
``` plain 
hello
world
```
</tmpl>
```
$ env --help
```
<tmpl,code:go run env.go --help>
``` plain 

  Usage: env [options]

  Options:
  --foo, -f   env FOO
  --bar, -b   env BAR
  --help, -h

```
</tmpl>
