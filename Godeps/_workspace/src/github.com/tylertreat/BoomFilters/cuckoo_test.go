package boom

import (
	"strconv"
	"testing"
)

// Ensures that Buckets returns the number of buckets, m, in the Cuckoo Filter.
func TestCuckooBuckets(t *testing.T) {
	f := NewCuckooFilter(100, 0.1)

	if buckets := f.Buckets(); buckets != 1024 {
		t.Errorf("Expected 1024, got %d", buckets)
	}
}

// Ensures that Capacity returns the expected filter capacity.
func TestCuckooCapacity(t *testing.T) {
	f := NewCuckooFilter(100, 0.1)

	if capacity := f.Capacity(); capacity != 100 {
		t.Errorf("Expected 100, got %d", capacity)
	}
}

// Ensures that Count returns the number of items added to the filter.
func TestCuckooCount(t *testing.T) {
	f := NewCuckooFilter(100, 0.1)
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
func TestCuckooTestAndAdd(t *testing.T) {
	f := NewCuckooFilter(100, 0.1)

	// `a` isn't in the filter.
	if f.Test([]byte(`a`)) {
		t.Error("`a` should not be a member")
	}

	if f.Add([]byte(`a`)) != nil {
		t.Error("error should be nil")
	}

	// `a` is now in the filter.
	if !f.Test([]byte(`a`)) {
		t.Error("`a` should be a member")
	}

	// `a` is still in the filter.
	if member, err := f.TestAndAdd([]byte(`a`)); !member {
		t.Error("`a` should be a member")
	} else if err != nil {
		t.Error("error should be nil")
	}

	// `b` is not in the filter.
	if member, err := f.TestAndAdd([]byte(`b`)); member {
		t.Error("`b` should not be a member")
	} else if err != nil {
		t.Error("error should be nil")
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

	for i := 0; i < 10000; i++ {
		f.TestAndAdd([]byte(strconv.Itoa(i)))
	}

	// Filter should be full.
	if f.Add([]byte(`abc`)) == nil {
		t.Error("error should not be nil")
	}

	// Filter should be full.
	if _, err := f.TestAndAdd([]byte(`xyz`)); err == nil {
		t.Error("error should not be nil")
	}
}

// Ensures that TestAndRemove behaves correctly.
func TestCuckooTestAndRemove(t *testing.T) {
	f := NewCuckooFilter(100, 0.1)

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

// Ensures that Reset clears all buckets and the count is zero.
func TestCuckooReset(t *testing.T) {
	f := NewCuckooFilter(100, 0.1)
	for i := 0; i < 1000; i++ {
		f.Add([]byte(strconv.Itoa(i)))
	}

	if f.Reset() != f {
		t.Error("Returned CuckooFilter should be the same instance")
	}

	for i := uint(0); i < f.m; i++ {
		for j := uint(0); j < f.b; j++ {
			if f.buckets[i][j] != nil {
				t.Error("Expected all buckets cleared")
			}
		}
	}

	if count := f.Count(); count != 0 {
		t.Errorf("Expected 0, got %d", count)
	}
}

func BenchmarkCuckooAdd(b *testing.B) {
	b.StopTimer()
	f := NewCuckooFilter(uint(b.N), 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.Add(data[n])
	}
}

func BenchmarkCuckooTest(b *testing.B) {
	b.StopTimer()
	f := NewCuckooFilter(uint(b.N), 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.Test(data[n])
	}
}

func BenchmarkCuckooTestAndAdd(b *testing.B) {
	b.StopTimer()
	f := NewCuckooFilter(uint(b.N), 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.TestAndAdd(data[n])
	}
}

func BenchmarkCuckooTestAndRemove(b *testing.B) {
	b.StopTimer()
	f := NewCuckooFilter(uint(b.N), 0.1)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.TestAndRemove(data[n])
	}
}
