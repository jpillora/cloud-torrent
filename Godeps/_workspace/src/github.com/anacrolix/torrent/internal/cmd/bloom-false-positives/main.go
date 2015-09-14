package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"

	"github.com/willf/bloom"
)

func main() {
	m := flag.Uint("m", 0, "")
	k := flag.Uint("k", 0, "")
	flag.Parse()
	filter := bloom.New(*m, *k)
	scanner := bufio.NewScanner(os.Stdin)
	n := 0
	collisions := 0
	for scanner.Scan() {
		if filter.TestAndAdd(scanner.Bytes()) {
			collisions++
		}
		n++
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "error reading stdin: %s", err)
		os.Exit(1)
	}
	fmt.Printf("collisions %d/%d (%f)\n", collisions, n, float64(collisions)/float64(n)*100)
}
