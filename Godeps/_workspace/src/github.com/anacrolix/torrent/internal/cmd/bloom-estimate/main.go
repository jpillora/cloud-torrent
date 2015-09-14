package main

import (
	"flag"
	"fmt"

	"github.com/willf/bloom"
)

func main() {
	n := flag.Int("n", 0, "expected number of items")
	falsePositiveRate := flag.Float64("fpr", 0, "false positive rate")
	flag.Parse()
	filter := bloom.NewWithEstimates(uint(*n), *falsePositiveRate)
	fmt.Printf("m: %d, k: %d\n", filter.Cap(), filter.K())
}
