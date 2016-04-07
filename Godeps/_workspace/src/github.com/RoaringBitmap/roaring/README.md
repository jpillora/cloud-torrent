roaring [![Build Status](https://travis-ci.org/RoaringBitmap/roaring.png)](https://travis-ci.org/RoaringBitmap/roaring)[![GoDoc](https://godoc.org/github.com/RoaringBitmap/roaring?status.svg)](https://godoc.org/github.com/RoaringBitmap/roaring)
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

Contributors: Todd Gruben (@tgruben), Daniel Lemire (@lemire), Elliot Murphy (@statik), Bob Potter (@bpot), Tyson Maly (@tvmaly), Will Glynn (@willglynn), Brent Pedersen (@brentp)

### References

-  Samy Chambi, Daniel Lemire, Owen Kaser, Robert Godin,
Better bitmap performance with Roaring bitmaps,
Software: Practice and Experience (accepted in 2015, to appear)
http://arxiv.org/abs/1402.6407 This paper used data from http://lemire.me/data/realroaring2014.html
- Daniel Lemire, Gregory Ssi-Yan-Kai, Owen Kaser, Consistently faster and smaller compressed bitmaps with Roaring, Software: Practice and Experience (accepted in 2016, to appear) http://arxiv.org/abs/1603.06549



### Dependencies

  - go get github.com/smartystreets/goconvey/convey
  - go get github.com/willf/bitset

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

### Benchmark

Type

         go test -bench Benchmark -run -


### Compatibility with Java RoaringBitmap library

You can read bitmaps in Go (resp. Java) that have been serialized in Java (resp. Go)
with the caveat that the Go library does not yet support run containers. So if you plan
to read bitmaps serialized from Java in Go, you might want to call ``removeRunCompression``
prior to serializing your Java instances. This is a temporary limitation: we plan to
add support for run containers to the Go library.

### Alternative

For an alternative implementation in Go, see https://github.com/fzandona/goroar
The two versions were written independently.
