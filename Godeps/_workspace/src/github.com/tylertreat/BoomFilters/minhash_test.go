package boom

import (
	"strconv"
	"testing"
)

// Ensures that MinHash returns the correct similarity ratio.
func TestMinHash(t *testing.T) {
	bag := []string{
		"bob",
		"alice",
		"frank",
		"tyler",
		"sara",
	}

	if s := MinHash(bag, bag); s != 1 {
		t.Errorf("expected 1, got %f", s)
	}

	dict := dictionary(1000)
	bag2 := []string{}
	for i := 0; i < 1000; i++ {
		bag = append(bag2, strconv.Itoa(i))
	}

	if s := MinHash(dict, bag2); s != 0 {
		t.Errorf("Expected 0, got %f", s)
	}

	bag2 = dictionary(500)

	if s := MinHash(dict, bag2); s > 0.7 || s < 0.5 {
		t.Errorf("Expected between 0.5 and 0.7, got %f", s)
	}
}

func BenchmarkMinHash(b *testing.B) {
	b.StopTimer()
	bag1 := dictionary(500)
	bag2 := dictionary(300)
	b.StartTimer()

	for n := 0; n < b.N; n++ {
		MinHash(bag1, bag2)
	}
}
