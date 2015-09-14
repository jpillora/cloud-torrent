/*
Original work Copyright 2013 Eric Lesh
Modified work Copyright 2015 Tyler Treat

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
"Software"), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.
*/

package boom

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"testing"
)

// Return a dictionary up to n words. If n is zero, return the entire
// dictionary.
func dictionary(n int) []string {
	var words []string
	dict := "/usr/share/dict/words"
	f, err := os.Open(dict)
	if err != nil {
		fmt.Printf("can't open dictionary file '%s': %v\n", dict, err)
		os.Exit(1)
	}
	count := 0
	buf := bufio.NewReader(f)
	for {
		if n != 0 && count >= n {
			break
		}
		word, err := buf.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			continue
		}
		words = append(words, word)
		count++
	}
	f.Close()
	return words
}

func geterror(actual uint64, estimate uint64) (result float64) {
	return (float64(estimate) - float64(actual)) / float64(actual)
}

func testHyperLogLog(t *testing.T, n, lowB, highB int) {
	words := dictionary(n)
	bad := 0
	nWords := uint64(len(words))
	for i := lowB; i < highB; i++ {
		m := uint(math.Pow(2, float64(i)))

		h, err := NewHyperLogLog(m)
		if err != nil {
			t.Fatalf("can't make NewHyperLogLog(%d): %v", m, err)
		}

		for _, word := range words {
			h.Add([]byte(word))
		}

		expectedError := 1.04 / math.Sqrt(float64(m))
		actualError := math.Abs(geterror(nWords, h.Count()))

		if actualError > expectedError {
			bad++
			t.Logf("m=%d: error=%.5f, expected <%.5f; actual=%d, estimated=%d\n",
				m, actualError, expectedError, nWords, h.Count())
		}

	}
	t.Logf("%d of %d tests exceeded estimated error", bad, highB-lowB)
}

func TestHyperLogLogSmall(t *testing.T) {
	testHyperLogLog(t, 5, 4, 17)
}

func TestHyperLogLogBig(t *testing.T) {
	testHyperLogLog(t, 0, 4, 17)
}

func TestNewDefaultHyperLogLog(t *testing.T) {
	hll, err := NewDefaultHyperLogLog(0.1)
	if err != nil {
		t.Fatalf("can't make NewDefaultHyperLogLog(0.1): %v", err)
	}

	if hll.m != 128 {
		t.Errorf("expected 128, got %d", hll.m)
	}
}

func benchmarkCount(b *testing.B, registers int) {
	words := dictionary(0)
	m := uint(math.Pow(2, float64(registers)))

	h, err := NewHyperLogLog(m)
	if err != nil {
		return
	}

	for _, word := range words {
		h.Add([]byte(word))
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		h.Count()
	}
}

func BenchmarkHLLCount4(b *testing.B) {
	benchmarkCount(b, 4)
}

func BenchmarkHLLCount5(b *testing.B) {
	benchmarkCount(b, 5)
}

func BenchmarkHLLCount6(b *testing.B) {
	benchmarkCount(b, 6)
}

func BenchmarkHLLCount7(b *testing.B) {
	benchmarkCount(b, 7)
}

func BenchmarkHLLCount8(b *testing.B) {
	benchmarkCount(b, 8)
}

func BenchmarkHLLCount9(b *testing.B) {
	benchmarkCount(b, 9)
}

func BenchmarkHLLCount10(b *testing.B) {
	benchmarkCount(b, 10)
}
