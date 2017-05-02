package boom

import (
	"strconv"
	"testing"
)

// Ensures that Capacity returns the number of bits, m, in the Bloom filter.
func TestDeletableCapacity(t *testing.T) {
	d := NewDeletableBloomFilter(100, 10, 0.1)

	if capacity := d.Capacity(); capacity != 470 {
		t.Errorf("Expected 470, got %d", capacity)
	}
}

// Ensures that K returns the number of hash functions in the Bloom Filter.
func TestDeletableK(t *testing.T) {
	d := NewDeletableBloomFilter(100, 10, 0.1)

	if k := d.K(); k != 4 {
		t.Errorf("Expected 4, got %d", k)
	}
}

// Ensures that Count returns the number of items added to the filter.
func TestDeletableCount(t *testing.T) {
	d := NewDeletableBloomFilter(100, 10, 0.1)
	for i := 0; i < 10; i++ {
		d.Add([]byte(strconv.Itoa(i)))
	}

	for i := 0; i < 5; i++ {
		d.TestAndRemove([]byte(strconv.Itoa(i)))
	}

	if count := d.Count(); count != 5 {
		t.Errorf("Expected 5, got %d", count)
	}
}

// Ensures that Test, Add, and TestAndAdd behave correctly.
func TestDeletableTestAndAdd(t *testing.T) {
	d := NewDeletableBloomFilter(100, 10, 0.1)

	// `a` isn't in the filter.
	if d.Test([]byte(`a`)) {
		t.Error("`a` should not be a member")
	}

	if d.Add([]byte(`a`)) != d {
		t.Error("Returned CountingBloomFilter should be the same instance")
	}

	// `a` is now in the filter.
	if !d.Test([]byte(`a`)) {
		t.Error("`a` should be a member")
	}

	// `a` is still in the filter.
	if !d.TestAndAdd([]byte(`a`)) {
		t.Error("`a` should be a member")
	}

	// `b` is not in the filter.
	if d.TestAndAdd([]byte(`b`)) {
		t.Error("`b` should not be a member")
	}

	// `a` is still in the filter.
	if !d.Test([]byte(`a`)) {
		t.Error("`a` should be a member")
	}

	// `b` is now in the filter.
	if !d.Test([]byte(`b`)) {
		t.Error("`b` should be a member")
	}

	// `c` is not in the filter.
	if d.Test([]byte(`c`)) {
		t.Error("`c` should not be a member")
	}

	for i := 0; i < 1000000; i++ {
		d.TestAndAdd([]byte(strconv.Itoa(i)))
	}

	// `x` should be a false positive.
	if !d.Test([]byte(`x`)) {
		t.Error("`x` should be a member")
	}
}

// Ensures that TestAndRemove behaves correctly.
func TestDeletableTestAndRemove(t *testing.T) {
	d := NewDeletableBloomFilter(100, 10, 0.1)

	// `a` isn't in the filter.
	if d.TestAndRemove([]byte(`a`)) {
		t.Error("`a` should not be a member")
	}

	d.Add([]byte(`a`))

	// `a` is now in the filter.
	if !d.TestAndRemove([]byte(`a`)) {
		t.Error("`a` should be a member")
	}

	// `a` is no longer in the filter.
	if d.TestAndRemove([]byte(`a`)) {
		t.Error("`a` should not be a member")
	}
}

// Ensures that Reset sets every bit to zero and the count is zero.
func TestDeletableReset(t *testing.T) {
	d := NewDeletableBloomFilter(100, 10, 0.1)
	for i := 0; i < 1000; i++ {
		d.Add([]byte(strconv.Itoa(i)))
	}

	if d.Reset() != d {
		t.Error("Returned CountingBloomFilter should be the same instance")
	}

	for i := uint(0); i < d.buckets.Count(); i++ {
		if d.buckets.Get(i) != 0 {
			t.Error("Expected all bits to be unset")
		}
	}

	for i := uint(0); i < d.collisions.Count(); i++ {
		if d.collisions.Get(i) != 0 {
			t.Error("Expected all bits to be unset")
		}
	}

	if count := d.Count(); count != 0 {
		t.Errorf("Expected 0, got %d", count)
	}
}

func BenchmarkDeletableAdd(b *testing.B) {
	b.StopTimer()
	d := NewDeletableBloomFilter(100, 10, 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		d.Add(data[n])
	}
}

func BenchmarkDeletableTest(b *testing.B) {
	b.StopTimer()
	d := NewDeletableBloomFilter(100, 10, 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		d.Test(data[n])
	}
}

func BenchmarkDeletableTestAndAdd(b *testing.B) {
	b.StopTimer()
	d := NewDeletableBloomFilter(100, 10, 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		d.TestAndAdd(data[n])
	}
}

func BenchmarkDeletableTestAndRemove(b *testing.B) {
	b.StopTimer()
	d := NewDeletableBloomFilter(100, 10, 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		d.TestAndRemove(data[n])
	}
}
