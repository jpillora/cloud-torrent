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

	expected := []string{"bill", "sara", "bob", "alice", "tyler"}
	actual := topk.Elements()

	if l := len(actual); l != 5 {
		t.Errorf("Expected len 5, got %d", l)
	}

	for i, element := range actual {
		if e := string(element); e != expected[i] {
			t.Errorf("Expected %s, got %s", expected[i], e)
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
