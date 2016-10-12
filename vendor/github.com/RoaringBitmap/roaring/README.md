roaring [![Build Status](https://travis-ci.org/RoaringBitmap/roaring.png)](https://travis-ci.org/RoaringBitmap/roaring) [![Coverage Status](https://coveralls.io/repos/github/RoaringBitmap/roaring/badge.svg?branch=master)](https://coveralls.io/github/RoaringBitmap/roaring?branch=master) [![GoDoc](https://godoc.org/github.com/RoaringBitmap/roaring?status.svg)](https://godoc.org/github.com/RoaringBitmap/roaring) [![Go Report Card](https://goreportcard.com/badge/RoaringBitmap/roaring)](https://goreportcard.com/report/github.com/RoaringBitmap/roaring)
=============

This is a go port of the Roaring bitmap data structure.

Roaring is  used by Apache Spark (https://spark.apache.org/), Apache Kylin (http://kylin.io),
Druid.io (http://druid.io/), Whoosh (https://pypi.python.org/pypi/Whoosh/)
and  Apache Lucene (http://lucene.apache.org/) (as well as supporting systems
such as Solr and Elastic).

The original java version can be found at https://github.com/RoaringBitmap/RoaringBitmap

The Java and Go version are meant to be binary compatible: you can save bitmaps
from a Java program and load them back in Go, and vice versa.


This code is licensed under Apache License, Version 2.0 (ASL2.0).

Copyright 2016 by the authors.


### References

-  Samy Chambi, Daniel Lemire, Owen Kaser, Robert Godin,
Better bitmap performance with Roaring bitmaps,
Software: Practice and Experience Volume 46, Issue 5, pages 709–719, May 2016
http://arxiv.org/abs/1402.6407 This paper used data from http://lemire.me/data/realroaring2014.html
- Daniel Lemire, Gregory Ssi-Yan-Kai, Owen Kaser, Consistently faster and smaller compressed bitmaps with Roaring, Software: Practice and Experience (accepted in 2016, to appear) http://arxiv.org/abs/1603.06549



### Dependencies

  - go get github.com/smartystreets/goconvey/convey
  - go get github.com/willf/bitset
  - go get github.com/mschoch/smat

Note that the smat library requires Go 1.6 or better.

Naturally, you also need to grab the roaring code itself:
  - go get github.com/RoaringBitmap/roaring


### Example

Here is a simplified but complete example:

```go
package main

import (
    "fmt"
    "github.com/RoaringBitmap/roaring"
    "bytes"
)


func main() {
    // example inspired by https://github.com/fzandona/goroar
    fmt.Println("==roaring==")
    rb1 := roaring.BitmapOf(1, 2, 3, 4, 5, 100, 1000)
    fmt.Println(rb1.String())

    rb2 := roaring.BitmapOf(3, 4, 1000)
    fmt.Println(rb2.String())

    rb3 := roaring.NewBitmap()
    fmt.Println(rb3.String())

    fmt.Println("Cardinality: ", rb1.GetCardinality())

    fmt.Println("Contains 3? ", rb1.Contains(3))

    rb1.And(rb2)

    rb3.Add(1)
    rb3.Add(5)

    rb3.Or(rb1)

    // prints 1, 3, 4, 5, 1000
    i := rb3.Iterator()
    for i.HasNext() {
        fmt.Println(i.Next())
    }
    fmt.Println()

    // next we include an example of serialization
    buf := new(bytes.Buffer)
    rb1.WriteTo(buf) // we omit error handling
    newrb:= roaring.NewBitmap()
    newrb.ReadFrom(buf)
    if rb1.Equals(newrb) {
    	fmt.Println("I wrote the content to a byte stream and read it back.")
    }
}
```

If you wish to use serialization and handle errors, you might want to
consider the following sample of code:

```go
	rb := BitmapOf(1, 2, 3, 4, 5, 100, 1000)
	buf := new(bytes.Buffer)
	size,err:=rb.WriteTo(buf)
	if err != nil {
		t.Errorf("Failed writing")
	}
	newrb:= NewBitmap()
	size,err=newrb.ReadFrom(buf)
	if err != nil {
		t.Errorf("Failed reading")
	}
	if ! rb.Equals(newrb) {
		t.Errorf("Cannot retrieve serialized version")
	}
```

Given N integers in [0,x), then the serialized size in bytes of
a Roaring bitmap should never exceed this bound:

`` 8 + 9 * ((long)x+65535)/65536 + 2 * N ``

That is, given a fixed overhead for the universe size (x), Roaring
bitmaps never use more than 2 bytes per integer. You can call
``BoundSerializedSizeInBytes`` for a more precise estimate.


### Documentation

Current documentation is available at http://godoc.org/github.com/RoaringBitmap/roaring

### Thread-safety

In general, it should generally be considered safe to access
the same bitmaps using different threads as
long as they are not being modified. However, if some of your
bitmaps use copy-on-write, then more care is needed: pass
to your threads a (shallow) copy of your bitmaps.

### Coverage

We test our software. For a report on our test coverage, see

https://coveralls.io/github/RoaringBitmap/roaring?branch=master

### Benchmark

Type

         go test -bench Benchmark -run -

### Iterative use

You can use roaring with gore:

- go get -u github.com/motemen/gore
- Make sure that ``$GOPATH/bin`` is in your ``$PATH``.
- go get github/RoaringBitmap/roaring

```go
$ gore
gore version 0.2.6  :help for help
gore> :import github.com/RoaringBitmap/roaring
gore> x:=roaring.New()
gore> x.Add(1)
gore> x.String()
"{1}"
```


### Fuzzy testing

You can help us test further the library with fuzzy testing:

         go get github.com/dvyukov/go-fuzz/go-fuzz
         go get github.com/dvyukov/go-fuzz/go-fuzz-build
         go test -tags=gofuzz -run=TestGenerateSmatCorpus
         go-fuzz-build github.com/RoaringBitmap/roaring
         go-fuzz -bin=./roaring-fuzz.zip -workdir=workdir/ -timeout=200

Let it run, and if the # of crashers is > 0, check out the reports in
the workdir where you should be able to find the panic goroutine stack
traces.

### Compatibility with Java RoaringBitmap library

You can read bitmaps in Go (resp. Java) that have been serialized in Java (resp. Go)
with the caveat that the Go library does not yet support run containers. So if you plan
to read bitmaps serialized from Java in Go, you might want to call ``removeRunCompression``
prior to serializing your Java instances. This is a temporary limitation: we plan to
add support for run containers to the Go library.

### Alternative in Go

There is a Go version wrapping the C/C++ implementation https://github.com/RoaringBitmap/gocroaring

For an alternative implementation in Go, see https://github.com/fzandona/goroar
The two versions were written independently.
