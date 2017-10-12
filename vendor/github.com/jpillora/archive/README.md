# archive

**Archive** combines Go's `archive`/[`zip`](http://golang.org/pkg/archive/zip) & [`tar`](https://golang.org/pkg/archive/tar) into a simpler, higher-level API

[![GoDoc](https://godoc.org/github.com/jpillora/archive?status.svg)](https://godoc.org/github.com/jpillora/archive)

### Features

* Simple
* Supports `tar`, `tar.gz` and `zip`
* Great for streaming directories over the network (e.g. see [serve(1)](https://github.com/jpillora/serve))

### Quick Usage

``` go
package main

import (
	"os"

	"github.com/jpillora/archive"
)

func main() {
	a := archive.NewTarGzWriter(os.Stdout)
	a.AddBytes("foo.txt", []byte("hello foo!"))
	a.AddBytes("dir/bar.txt", []byte("hello bar!"))
	a.Close()
}
```

``` sh
$ go run example.go | tar zxvf -
x foo.txt
x dir/bar.txt
```

See the [example/](example/) for more

#### Todo

* Add extraction support
* Add `.7z` archive type
* Add `.lz` compression type

#### MIT License

Copyright © 2015 Jaime Pillora &lt;dev@jpillora.com&gt;

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