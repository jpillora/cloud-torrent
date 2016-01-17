package boom

import (
	"strconv"
	"testing"
)

// Ensures that TopK return the top-k most frequent elements.
func TestTopK(t *testing.T) {

	topk := NewTopK(0.001, 0.99, 5)

	topk.Add([]byte(`bob`)).Add([]byte(`bob`)).Add([]byte(`bob`))
	topk.Add([]byte(`tyler`)).Add([]byte(`tyler`)).Add([]byte(`tyler`)).Add([]byte(`tyler`)).Add([]byte(`tyler`))
	topk.Add([]byte(`fred`))
	topk.Add([]byte(`alice`)).Add([]byte(`alice`)).Add([]byte(`alice`)).Add([]byte(`alice`))
	topk.Add([]byte(`james`))
	topk.Add([]byte(`fred`))
	topk.Add([]byte(`sara`)).Add([]byte(`sara`))

	if topk.Add([]byte(`bill`)) != topk {
		t.Error("Returned TopK should be the same instance")
	}
	// latest one also
	expected := []struct {
		name string
		freq uint64
	}{
		{"bill", 1},
		{"sara", 2},
		{"bob", 3},
		{"alice", 4},
		{"tyler", 5},
	}

	actual := topk.Elements()

	if l := len(actual); l != 5 {
		t.Errorf("Expected len %d, got %d", 5, l)
	}

	for i, element := range actual {
		if e := string((*element).Data); e != expected[i].name {
			t.Errorf("Expected %s, got %s", expected[i].name, e)
		}
		// freq check
		if freq := element.Freq; freq != expected[i].freq {
			t.Errorf("Expected %d, got %d", expected[i].freq, freq)
		}
	}

	if topk.Reset() != topk {
		t.Error("Returned TopK should be the same instance")
	}

	if l := topk.elements.Len(); l != 0 {
		t.Errorf("Expected 0, got %d", l)
	}

	if n := topk.n; n != 0 {
		t.Errorf("Expected 0, got %d", n)
	}
}

func BenchmarkTopKAdd(b *testing.B) {
	b.StopTimer()
	topk := NewTopK(0.001, 0.99, 5)
	data := make([][]byte, b.N)
	for i := 0; i < b.N; i++ {
		data[i] = []byte(strconv.Itoa(i))
	}
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		topk.Add(data[n])
	}
}
