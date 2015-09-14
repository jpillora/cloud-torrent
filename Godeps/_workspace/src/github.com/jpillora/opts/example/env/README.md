## env example

<tmpl,code=go:cat env_all.go>
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

	//in this case UseEnv() is equivalent to
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
$ go run env_all.go
```
<tmpl,code:(export FOO=hello && export BAR=world && go run env_all.go)>
``` plain 
hello
world
```
</tmpl>
```
$ env --help
```
<tmpl,code:go run env_all.go --help>
``` plain 

  Usage: env_all [options]
  
  Options:
  --foo, -f   env FOO
  --bar, -b   env BAR
  --help, -h
  

```
</tmpl>