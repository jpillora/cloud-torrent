## customtypes example

<tmpl,code=go:cat customtypes.go>
``` go 
package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/jpillora/opts"
)

//custom types are allowed if they implement the flag.Value interface
type MagicInt int

func (b MagicInt) String() string {
	return "{" + strconv.Itoa(int(b)) + "}"
}

func (b *MagicInt) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*b = MagicInt(n + 42)
	return nil
}

type Config struct {
	Foo  time.Duration
	Bar  MagicInt
	Bazz int
}

func main() {

	c := Config{}

	opts.Parse(&c)

	fmt.Printf("%3.0f %s %d\n", c.Foo.Seconds(), c.Bar, c.Bazz)
}
```
</tmpl>
```
$ customtypes --foo 2m --bar 5 --bazz 5
```
<tmpl,code:go run customtypes.go --foo 2m --bar 5 --bazz 5>
``` plain 
120 {47} 5
```
</tmpl>
```
$ customtypes --help
```
<tmpl,code:go run customtypes.go --help>
``` plain 

  Usage: customtypes [options]

  Options:
  --foo, -f
  --bar, -b
  --bazz
  --help, -h

```
</tmpl>