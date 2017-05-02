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
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
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

func TestHyperLogLogSerialization(t *testing.T) {
	hll, err := NewDefaultHyperLogLog(0.1)
	if err != nil {
		t.Fatalf("can't make NewDefaultHyperLogLog(0.1): %v", err)
	}
	ppl := []string{"frank", "alice", "bob"}
	for _, v := range ppl {
		hll.Add([]byte(v))
		if v == "bob" || v == "frank" {
			hll.Add([]byte(v))
		}
	}

	buf := new(bytes.Buffer)
	// serialize
	wn, err := hll.WriteDataTo(buf)
	if err != nil {
		t.Error("unexpected error bytes written %d", err, wn)
	}
	hll.Reset()

	newHll, err := NewDefaultHyperLogLog(0.1)
	if err != nil {
		t.Error("unexpected error", err)
	}

	rn, err := newHll.ReadDataFrom(buf)
	if err != nil {
		t.Errorf("readfrom err %s bytes read %d", err, rn)
	}

	if count := newHll.Count(); count != uint64(len(ppl)) {
		t.Errorf("expected %d, got %d\n", len(ppl), count)
	}

	wrongHll, err := NewDefaultHyperLogLog(0.01)
	if err != nil {
		t.Error("unexpected error", err)
	}
	_, err = newHll.WriteDataTo(buf)
	if err != nil {
		t.Error("unexpected error", err)
	}
	newHll.Reset()

	// hll register number should be same with serialized hll
	_, err = wrongHll.ReadDataFrom(buf)

	if !strings.Contains(err.Error(), "hll register") {
		t.Error("unexpected error %s", err)
	}

}

func BenchmarkHllWriteDataTo(b *testing.B) {
	b.StopTimer()
	hll, err := NewDefaultHyperLogLog(0.1)
	if err != nil {
		b.Errorf("unexpected error %s\n", err)
	}
	ppl := [][]byte{[]byte("frank"), []byte("alice"), []byte("bob")}
	for _, v := range ppl {
		hll.Add(v)
		if string(v) == string(ppl[2]) {
			hll.Add(v)
		}

	}
	var buf bytes.Buffer
	b.StartTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := hll.WriteDataTo(&buf)
		if err != nil {
			b.Errorf("unexpected error %s\n", err)
		}
	}

}

func BenchmarkHllReadDataFrom(b *testing.B) {
	b.StopTimer()
	buf := new(bytes.Buffer)
	b.N = 10000

	hll, err := NewDefaultHyperLogLog(0.1)
	if err != nil {
		b.Errorf("unexpected error %s\n", err)
	}
	// add data to hll
	ppl := [][]byte{[]byte("frank"), []byte("alice"), []byte("bob")}
	for _, v := range ppl {
		hll.Add(v)
		if string(v) == string(ppl[2]) {
			hll.Add(v)
		}

	}
	// serialize
	_, err = hll.WriteDataTo(buf)
	if err != nil {
		b.Errorf("unexpected error %s\n", err)
	}
	// prepare buffer with data to read
	data := make([]byte, 0)
	for i := 0; i < b.N; i++ {
		data = append(data, buf.Bytes()...)
	}

	rd := bytes.NewReader(data)
	newHll, err := NewDefaultHyperLogLog(0.1)
	if err != nil {
		b.Errorf("unexpected error %s\n", err)
	}

	b.StartTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// deserialize hll
		_, err := newHll.ReadDataFrom(rd)
		if err != nil {
			b.Errorf("unexpected error %s\n", err)
		}
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
