package roaring_test

import (
	"bytes"
	"fmt"
	"github.com/RoaringBitmap/roaring"
)

// Example_roaring demonstrates how to use the roaring library.
func Example_roaring() {
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
	size, err := rb1.WriteTo(buf)
	if err != nil {
		fmt.Println("Failed writing")
		return
	} else {
		fmt.Println("Wrote ", size, " bytes")
	}
	newrb := roaring.NewBitmap()
	size, err = newrb.ReadFrom(buf)
	if err != nil {
		fmt.Println("Failed reading")
		return
	}
	if !rb1.Equals(newrb) {
		fmt.Println("I did not get back to original bitmap?")
		return
	} else {
		fmt.Println("I wrote the content to a byte stream and read it back.")
	}
}
