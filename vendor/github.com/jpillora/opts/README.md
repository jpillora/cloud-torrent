# opts

**A low friction command-line interface library for Go (Golang)**

[![GoDoc](https://godoc.org/github.com/jpillora/opts?status.svg)](https://godoc.org/github.com/jpillora/opts)  [![CircleCI](https://circleci.com/gh/jpillora/opts.svg?style=shield&circle-token=69ef9c6ac0d8cebcb354bb85c377eceff77bfb1b)](https://circleci.com/gh/jpillora/opts)

Command-line parsing should be easy. Use configuration structs:

``` go
package main

import (
	"fmt"

	"github.com/jpillora/opts"
)

func main() {
	config := struct {
		File  string `help:"file to load"`
		Lines int    `help:"number of lines to show"`
	}{}
	opts.Parse(&config)
	fmt.Println(config)
}
```

```
$ go run main.go -f foo -l 12
{foo 12}
```

### Features (with examples)

* Easy to use ([simple](example/simple/))
* Promotes separation of CLI code and library code ([separation](example/separation/))
* Automatically generated `--help` text via struct tags `help:"Foo bar"` ([help](example/help/))
* Commands by nesting structs ([cmds](example/cmds/))
* Default values by modifying the struct prior to `Parse()` ([defaults](example/defaults/))
* Default values from a JSON config file, unmarshalled via your config struct ([config](example/config/))
* Default values from environment, defined by your field names ([env](example/env/))
* Infers program name from package name (and optional repository link)
* Extensible via `flag.Value` ([customtypes](example/customtypes/))
* Customizable help text by modifying the default templates ([customhelp](example/customhelp/))

### Overview

Internally, `opts` creates `flag.FlagSet`s from your configuration structs using `pkg/reflect`. So, given the following program:

```go
type Config struct {
	Alpha   string        `help:"a string"`
	Bravo   int           `help:"an int"`
	Charlie bool          `help:"a bool"`
	Delta   time.Duration `help:"a duration"`
}

c := Config{
	Bravo: 42,
	Delta: 2 * time.Minute,
}
```

```go
opts.Parse(&c)
```

At this point, `opts.Parse` will *approximately* perform:

```go
foo := Config{}
set := flag.NewFlagSet("Config")
set.StringVar(&foo.Alpha, "", "a string")
set.IntVar(&foo.Bravo, 42, "an int")
set.BoolVar(&foo.Charlie, false, "a bool")
set.DurationVar(&foo.Delta, 2 * time.Minute, "a duration")
set.Parse(os.Args)
```

And, you get pretty `--help` output:

```
$ ./foo --help

  Usage: foo [options]

  Options:
  --alpha, -a    a string
  --bravo, -b    an int (default 42)
  --charlie, -c  an bool
  --delta, -d    a duration (default 2m0s)
  --help, -h

```

### All Examples

---

#### See all [example](example/)s here

---

### Package API

See [![GoDoc](https://godoc.org/github.com/jpillora/opts?status.svg)](https://godoc.org/github.com/jpillora/opts)

### Struct Tag API

**opts** tries to set sane defaults so, for the most part, you'll get the desired behaviour
by simply providing a configuration struct. These defaults can be overridden using the struct
tag `key:"value"`s outlined below.

#### **Common tags**

These tags are usable across all `type`s:

* `name` - Name is used to display the field in the help text (defaults to the field name converted to lowercase and dashes)
* `help` - Help is used to describe the field (defaults to "")
* `type` - The `opts` type assigned the field (defaults using the table below)

#### `type` defaults

All fields will have a **opts** `type`. By default a struct field will be assigned a `type` depending on its field type:

| Field Type    | Default `type` | Valid `type`s      |
| ------------- |:-------------:|:-------------------:|
| int           | opt           | opt, arg            |
| string        | opt           | opt, arg, cmdname   |
| bool          | opt           | opt, arg            |
| flag.Value    | opt           | opt, arg            |
| time.Duration | opt           | opt, arg            |
| []string      | arglist       | arglist             |
| struct        | cmd           | cmd, embedded       |

This default assignment can be overridden with a `type` struct tag. For example you could set a string struct field to be an `arg` field with `type:"arg"`.

#### `type` specific properties

* **`opt`**

	An option (`opt`) field will appear in the options list and by definition, be optional.

	* `short` - An alias or shortcut to this option (defaults to the first letter of the `name` property)
	* `env` - An environment variable to use to retrieve the default (**when `UseEnv()` is set** this defaults to `name` property converted to uppercase and underscores)

	Restricted to fields with type `int`,`string`,`bool`,`time.Duration` and `flag.Value`

* **`arg`**

	An argument (`arg`) field will appear in the usage and will be required if it does not have a default value set.

	Restricted to fields with type `string`

* **`arglist`**

	An argument list (`arglist`) field will appear in the usage. Useful for a you allow any number number of arguments. For example file and directory targets.

	* `min` - An integer representing the minimum number of args specified

	Restricted to fields with type `[]string`

* **`cmd`**

	A command is nested `opts.Opt` instance, so its fields behave in exactly the same way as the parent struct.

	You can access the options of a command with `prog --prog-opt X cmd --cmd-opt Y`

	Restricted to fields with type `struct`

* **`cmdname`**

	A special type which will assume the name of the selected command

	Restricted to fields with type `string`

* **`embedded`**

	A special type which causes the fields of struct to be used in the current struct. Useful if you want to extend existing structs with extra command-line options.

### Other projects

Other CLI libraries which infer flags from struct tags:

* https://github.com/jessevdk/go-flags is similar though it still could be simpler and more customizable.

### Why

Why yet another struct-based command-line library? I started this project [back in April](https://github.com/jpillora/opts/commit/b87563662e56b05fbcc326449db57a7761ef4d51)
when the only thing around was `jessevdk/go-flags` and I wanted more customization. Now there is [tj/go-config](https://github.com/tj/go-config) and [alexflint/go-arg](https://github.com/alexflint/go-arg) and still, these don't yet include [nested structs](example/cmds/) and [customizable help text](example/customhelp/).

### Todo

* More tests
* Option groups (Separate sets of options in `--help`)
* Bash completion
* Multiple short options `-aux` (Requires a non-`pkg/flag` parser)

#### MIT License

Copyright © 2015 &lt;dev@jpillora.com&gt;

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
'Software'), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED 'AS IS', WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
