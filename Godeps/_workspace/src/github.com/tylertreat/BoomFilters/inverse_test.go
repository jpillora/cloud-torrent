package boom

import (
	"strconv"
	"testing"
)

// Ensures that Capacity returns the correct filter size.
func TestInverseCapacity(t *testing.T) {
	f := NewInverseBloomFilter(100)

	if c := f.Capacity(); c != 100 {
		t.Errorf("expected 100, got %d", c)
	}
}

// Ensures that TestAndAdd behaves correctly.
func TestInverseTestAndAdd(t *testing.T) {
	f := NewInverseBloomFilter(3)

	if f.TestAndAdd([]byte(`a`)) {
		t.Error("`a` should not be a member")
	}

	if !f.TestAndAdd([]byte(`a`)) {
		t.Error("`a` should be a member")
	}

	// `d` hashes to the same index as `a`.
	if f.TestAndAdd([]byte(`d`)) {
		t.Error("`d` should not be a member")
	}

	// `a` was swapped out.
	if f.TestAndAdd([]byte(`a`)) {
		t.Error("`a` should not be a member")
	}

	if !f.TestAndAdd([]byte(`a`)) {
		t.Error("`a` should be a member")
	}

	// `b` hashes to another index.
	if f.TestAndAdd([]byte(`b`)) {
		t.Error("`b` should not be a member")
	}

	if !f.TestAndAdd([]byte(`b`)) {
		t.Error("`b` should be a member")
	}

	// `a` should still be a member.
	if !f.TestAndAdd([]byte(`a`)) {
		t.Error("`a` should be a member")
	}

	if f.Test([]byte(`c`)) {
		t.Error("`c` should not be a member")
	}

	if f.Add([]byte(`c`)) != f {
		t.Error("Returned InverseBloomFilter should be the same instance")
	}

	if !f.Test([]byte(`c`)) {
		t.Error("`c` should be a member")
	}
}

func BenchmarkInverseAdd(b *testing.B) {
	b.StopTimer()
	f := NewInverseBloomFilter(100000)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.Add(data[n])
	}
}

func BenchmarkInverseTest(b *testing.B) {
	b.StopTimer()
	f := NewInverseBloomFilter(100000)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
		f.Add(data[i])
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.Test(data[n])
	}
}

func BenchmarkInverseTestAndAdd(b *testing.B) {
	b.StopTimer()
	f := NewInverseBloomFilter(100000)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		f.TestAndAdd(data[n])
	}
}
