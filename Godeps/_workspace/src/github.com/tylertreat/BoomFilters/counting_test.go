package boom

import (
	"strconv"
	"testing"
)

// Ensures that Capacity returns the number of bits, m, in the Bloom filter.
func TestCountingCapacity(t *testing.T) {
	f := NewDefaultCountingBloomFilter(100, 0.1)

	if capacity := f.Capacity(); capacity != 480 {
		t.Errorf("Expected 480, got %d", capacity)
	}
}

// Ensures that K returns the number of hash functions in the Bloom Filter.
func TestCountingK(t *testing.T) {
	f := NewDefaultCountingBloomFilter(100, 0.1)

	if k := f.K(); k != 4 {
		t.Errorf("Expected 4, got %d", k)
	}
}

// Ensures that Count returns the number of items added to the filter.
func TestCountingCount(t *testing.T) {
	f := NewDefaultCountingBloomFilter(100, 0.1)
	for i := 0; i < 10; i++ {
		f.Add([]byte(strconv.Itoa(i)))
	}

	for i := 0; i < 5; i++ {
		f.TestAndRemove([]byte(strconv.Itoa(i)))
	}

	if count := f.Count(); count != 5 {
		t.Errorf("Expected 5, got %d", count)
	}
}

// Ensures that Test, Add, and TestAndAdd behave correctly.
func TestCountingTestAndAdd(t *testing.T) {
	f := NewDefaultCountingBloomFilter(100, 0.1)

	// `a` isn't in the filter.
	if f.Test([]byte(`a`)) {
		t.Error("`a` should not be a member")
	}

	if f.Add([]byte(`a`)) != f {
		t.Error("Returned CountingBloomFilter should be the same instance")
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

	// `x` should be a false positive.
	if !f.Test([]byte(`x`)) {
		t.Error("`x` should be a member")
	}
}

// Ensures that TestAndRemove behaves correctly.
func TestCountingTestAndRemove(t *testing.T) {
	f := NewDefaultCountingBloomFilter(100, 0.1)

	// `a` isn't in the filter.
	if f.TestAndRemove([]byte(`a`)) {
		t.Error("`a` should not be a member")
	}

	f.Add([]byte(`a`))

	// `a` is now in the filter.
	if !f.TestAndRemove([]byte(`a`)) {
		t.Error("`a` should be a member")
	}

	// `a` is no longer in the filter.
	if f.TestAndRemove([]byte(`a`)) {
		t.Error("`a` should not be a member")
	}
}

// Ensures that Reset sets every bit to zero and the count is zero.
func TestCountingReset(t *testing.T) {
	f := NewDefaultCountingBloomFilter(100, 0.1)
	for i := 0; i < 1000; i++ {
		f.Add([]byte(strconv.Itoa(i)))
	}

	if f.Reset() != f {
		t.Error("Returned CountingBloomFilter should be the same instance")
	}

	for i := uint(0); i < f.buckets.Count(); i++ {
		if f.buckets.Get(i) != 0 {
			t.Error("Expected all bits to be unset")
		}
	}

	if count := f.Count(); count != 0 {
		t.Errorf("Expected 0, got %d", count)
	}
}

func BenchmarkCountingAdd(b *testing.B) {
	b.StopTimer()
	f := NewDefaultCountingBloomFilter(100000, 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.Add(data[n])
	}
}

func BenchmarkCountingTest(b *testing.B) {
	b.StopTimer()
	f := NewDefaultCountingBloomFilter(100000, 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.Test(data[n])
	}
}

func BenchmarkCountingTestAndAdd(b *testing.B) {
	b.StopTimer()
	f := NewDefaultCountingBloomFilter(100000, 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.TestAndAdd(data[n])
	}
}

func BenchmarkCountingTestAndRemove(b *testing.B) {
	b.StopTimer()
	f := NewDefaultCountingBloomFilter(100000, 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.TestAndRemove(data[n])
	}
}
