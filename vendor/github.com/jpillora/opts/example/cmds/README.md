## cmds example

<tmpl,code=go:cat cmds.go>
``` go 
package main

import (
	"fmt"

	"github.com/jpillora/opts"
)

type FooConfig struct {
	Ping string
	Pong string
}

//config
type Config struct {
	Cmd string `type:"cmdname"`
	//command (external struct)
	Foo FooConfig
	//command (inline struct)
	Bar struct {
		Zip string
		Zap string
	}
}

func main() {

	c := Config{}

	opts.Parse(&c)

	fmt.Println(c.Cmd)
	fmt.Println(c.Bar.Zip)
	fmt.Println(c.Bar.Zap)
}
```
</tmpl>
```
$ cmds bar --zip hello --zap world
```
<tmpl,code:go run cmds.go bar --zip hello --zap world>
``` plain 
bar
hello
world
```
</tmpl>
```
$ cmds --help
```
<tmpl,code:go run cmds.go --help>
``` plain 

  Usage: cmds [options] <command>

  Options:
  --help, -h

  Commands:
  • foo
  • bar

```
</tmpl>