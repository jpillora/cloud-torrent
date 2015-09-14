package boom

import (
	"math"
	"strconv"
	"testing"
)

// Ensures that NewUnstableBloomFilter creates a Stable Bloom Filter with p=0,
// max=1 and k hash functions.
func TestNewUnstableBloomFilter(t *testing.T) {
	f := NewUnstableBloomFilter(100, 0.1)
	k := OptimalK(0.1)

	if f.k != k {
		t.Errorf("Expected %f, got %d", k, f.k)
	}

	if f.m != 100 {
		t.Errorf("Expected 100, got %d", f.m)
	}

	if f.P() != 0 {
		t.Errorf("Expected 0, got %d", f.p)
	}

	if f.max != 1 {
		t.Errorf("Expected 1, got %d", f.max)
	}
}

// Ensures that Cells returns the number of cells, m, in the Stable Bloom
// Filter.
func TestCells(t *testing.T) {
	f := NewStableBloomFilter(100, 1, 0.1)

	if cells := f.Cells(); cells != 100 {
		t.Errorf("Expected 100, got %d", cells)
	}
}

// Ensures that K returns the number of hash functions in the Stable Bloom
// Filter.
func TestK(t *testing.T) {
	f := NewStableBloomFilter(100, 1, 0.01)

	if k := f.K(); k != 3 {
		t.Errorf("Expected 3, got %d", k)
	}
}

// Ensures that Test, Add, and TestAndAdd behave correctly.
func TestTestAndAdd(t *testing.T) {
	f := NewDefaultStableBloomFilter(10000, 0.01)

	// `a` isn't in the filter.
	if f.Test([]byte(`a`)) {
		t.Error("`a` should not be a member")
	}

	if f.Add([]byte(`a`)) != f {
		t.Error("Returned StableBloomFilter should be the same instance")
	}

	// `a` is now in the filter.
	if !f.Test([]byte(`a`)) {
		t.Error("`a` should be a member")
	}

	// `a` is still in the filter.
	if !f.TestAndAdd([]byte(`a`)) {
		t.Error("`a` should be a member")
	}

	// `b` is not in the filter.
	if f.TestAndAdd([]byte(`b`)) {
		t.Error("`b` should not be a member")
	}

	// `a` is still in the filter.
	if !f.Test([]byte(`a`)) {
		t.Error("`a` should be a member")
	}

	// `b` is now in the filter.
	if !f.Test([]byte(`b`)) {
		t.Error("`b` should be a member")
	}

	// `c` is not in the filter.
	if f.Test([]byte(`c`)) {
		t.Error("`c` should not be a member")
	}

	for i := 0; i < 1000000; i++ {
		f.TestAndAdd([]byte(strconv.Itoa(i)))
	}

	// `a` should have been evicted.
	if f.Test([]byte(`a`)) {
		t.Error("`a` should not be a member")
	}
}

// Ensures that StablePoint returns the expected fraction of zeros for large
// iterations.
func TestStablePoint(t *testing.T) {
	f := NewStableBloomFilter(1000, 1, 0.1)
	for i := 0; i < 1000000; i++ {
		f.Add([]byte(strconv.Itoa(i)))
	}

	zeros := 0
	for i := uint(0); i < f.m; i++ {
		if f.cells.Get(i) == 0 {
			zeros++
		}
	}

	actual := round(float64(zeros)/float64(f.m), 0.5, 1)
	expected := round(f.StablePoint(), 0.5, 1)

	if actual != expected {
		t.Errorf("Expected stable point %f, got %f", expected, actual)
	}

	// A classic Bloom filter is a special case of SBF where P is 0 and max is
	// 1. It doesn't have a stable point.
	bf := NewUnstableBloomFilter(1000, 0.1)
	if stablePoint := bf.StablePoint(); stablePoint != 0 {
		t.Errorf("Expected stable point 0, got %f", stablePoint)
	}
}

// Ensures that FalsePositiveRate returns the upper bound on false positives
// for stable filters.
func TestFalsePositiveRate(t *testing.T) {
	f := NewDefaultStableBloomFilter(1000, 0.01)
	fps := round(f.FalsePositiveRate(), 0.5, 2)
	if fps > 0.01 {
		t.Errorf("Expected fps less than or equal to 0.01, got %f", fps)
	}

	// Classic Bloom filters have an unbounded rate of false positives. Once
	// they become full, every query returns a false positive.
	bf := NewUnstableBloomFilter(1000, 0.1)
	if fps := bf.FalsePositiveRate(); fps != 1 {
		t.Errorf("Expected fps 1, got %f", fps)
	}
}

// Ensures that Reset sets every cell to zero.
func TestReset(t *testing.T) {
	f := NewDefaultStableBloomFilter(1000, 0.01)
	for i := 0; i < 1000; i++ {
		f.Add([]byte(strconv.Itoa(i)))
	}

	if f.Reset() != f {
		t.Error("Returned StableBloomFilter should be the same instance")
	}

	for i := uint(0); i < f.m; i++ {
		if cell := f.cells.Get(i); cell != 0 {
			t.Errorf("Expected zero cell, got %d", cell)
		}
	}
}

func BenchmarkStableAdd(b *testing.B) {
	b.StopTimer()
	f := NewDefaultStableBloomFilter(100000, 0.01)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.Add(data[n])
	}
}

func BenchmarkStableTest(b *testing.B) {
	b.StopTimer()
	f := NewDefaultStableBloomFilter(100000, 0.01)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.Test(data[n])
	}
}

func BenchmarkStableTestAndAdd(b *testing.B) {
	b.StopTimer()
	f := NewDefaultStableBloomFilter(100000, 0.01)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.TestAndAdd(data[n])
	}
}

func BenchmarkUnstableAdd(b *testing.B) {
	b.StopTimer()
	f := NewUnstableBloomFilter(100000, 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.Add(data[n])
	}
}

func BenchmarkUnstableTest(b *testing.B) {
	b.StopTimer()
	f := NewUnstableBloomFilter(100000, 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.Test(data[n])
	}
}

func BenchmarkUnstableTestAndAdd(b *testing.B) {
	b.StopTimer()
	f := NewUnstableBloomFilter(100000, 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.TestAndAdd(data[n])
	}
}
func round(val float64, roundOn float64, places int) (newVal float64) {
	var round float64
	pow := math.Pow(10, float64(places))
	digit := pow * val
	_, div := math.Modf(digit)
	if div >= roundOn {
		round = math.Ceil(digit)
	} else {
		round = math.Floor(digit)
	}
	newVal = round / pow
	return
}
