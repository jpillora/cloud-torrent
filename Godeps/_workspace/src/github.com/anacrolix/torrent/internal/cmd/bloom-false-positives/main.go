package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/anacrolix/tagflag"
	"github.com/willf/bloom"
)

func main() {
	var args struct {
		M uint `help:"num bits"`
		K uint `help:"num hashing functions"`
	}
	tagflag.Parse(&args, tagflag.Description("adds lines from stdin to a bloom filter with the given configuration, and gives collision stats at EOF"))
	filter := bloom.New(args.M, args.K)
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
