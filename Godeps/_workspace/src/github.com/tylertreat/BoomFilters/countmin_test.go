package boom

import (
	"strconv"
	"testing"
)

// Ensures that TotalCount returns the number of items added to the sketch.
func TestCMSTotalCount(t *testing.T) {
	cms := NewCountMinSketch(0.001, 0.99)

	for i := 0; i < 100; i++ {
		cms.Add([]byte(strconv.Itoa(i)))
	}

	if count := cms.TotalCount(); count != 100 {
		t.Errorf("expected 100, got %d", count)
	}
}

// Ensures that Add adds to the set and Count returns the correct
// approximation.
func TestCMSAddAndCount(t *testing.T) {
	cms := NewCountMinSketch(0.001, 0.99)

	if cms.Add([]byte(`a`)) != cms {
		t.Error("Returned CountMinSketch should be the same instance")
	}

	cms.Add([]byte(`b`))
	cms.Add([]byte(`c`))
	cms.Add([]byte(`b`))
	cms.Add([]byte(`d`))
	cms.Add([]byte(`a`)).Add([]byte(`a`))

	if count := cms.Count([]byte(`a`)); count != 3 {
		t.Errorf("expected 3, got %d", count)
	}

	if count := cms.Count([]byte(`b`)); count != 2 {
		t.Errorf("expected 2, got %d", count)
	}

	if count := cms.Count([]byte(`c`)); count != 1 {
		t.Errorf("expected 1, got %d", count)
	}

	if count := cms.Count([]byte(`d`)); count != 1 {
		t.Errorf("expected 1, got %d", count)
	}

	if count := cms.Count([]byte(`x`)); count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

// Ensures that Merge combines the two sketches.
func TestCMSMerge(t *testing.T) {
	cms := NewCountMinSketch(0.001, 0.99)
	cms.Add([]byte(`b`))
	cms.Add([]byte(`c`))
	cms.Add([]byte(`b`))
	cms.Add([]byte(`d`))
	cms.Add([]byte(`a`)).Add([]byte(`a`))

	other := NewCountMinSketch(0.001, 0.99)
	other.Add([]byte(`b`))
	other.Add([]byte(`c`))
	other.Add([]byte(`b`))

	if err := cms.Merge(other); err != nil {
		t.Error(err)
	}

	if count := cms.Count([]byte(`a`)); count != 2 {
		t.Errorf("expected 2, got %d", count)
	}

	if count := cms.Count([]byte(`b`)); count != 4 {
		t.Errorf("expected 4, got %d", count)
	}

	if count := cms.Count([]byte(`c`)); count != 2 {
		t.Errorf("expected 2, got %d", count)
	}

	if count := cms.Count([]byte(`d`)); count != 1 {
		t.Errorf("expected 1, got %d", count)
	}

	if count := cms.Count([]byte(`x`)); count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

// Ensures that Reset restores the sketch to its original state.
func TestCMSReset(t *testing.T) {
	cms := NewCountMinSketch(0.001, 0.99)
	cms.Add([]byte(`b`))
	cms.Add([]byte(`c`))
	cms.Add([]byte(`b`))
	cms.Add([]byte(`d`))
	cms.Add([]byte(`a`)).Add([]byte(`a`))

	if cms.Reset() != cms {
		t.Error("Returned CountMinSketch should be the same instance")
	}

	for i := uint(0); i < cms.depth; i++ {
		for j := uint(0); j < cms.width; j++ {
			if x := cms.matrix[i][j]; x != 0 {
				t.Errorf("expected matrix to be completely empty, got %d", x)
			}
		}
	}
}

func BenchmarkCMSAdd(b *testing.B) {
	b.StopTimer()
	cms := NewCountMinSketch(0.001, 0.99)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		cms.Add(data[n])
	}
}

func BenchmarkCMSCount(b *testing.B) {
	b.StopTimer()
	cms := NewCountMinSketch(0.001, 0.99)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
		cms.Add([]byte(strconv.Itoa(i)))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		cms.Count(data[n])
	}
}
