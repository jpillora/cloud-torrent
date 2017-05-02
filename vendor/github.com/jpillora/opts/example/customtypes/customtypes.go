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
