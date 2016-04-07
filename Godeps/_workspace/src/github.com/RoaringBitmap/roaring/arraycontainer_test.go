package roaring

// to run just these tests: go test -run TestArrayContainer*

import (
	"reflect"
	"testing"
)

func TestArrayContainerTransition(t *testing.T) {
	v := container(newArrayContainer())
	arraytype := reflect.TypeOf(v)
	for i := 0; i < arrayDefaultMaxSize; i++ {
		v = v.add(uint16(i))
	}
	if v.getCardinality() != arrayDefaultMaxSize {
		t.Errorf("Bad cardinality.")
	}
	if reflect.TypeOf(v) != arraytype {
		t.Errorf("Should be an array.")
	}
	for i := 0; i < arrayDefaultMaxSize; i++ {
		v = v.add(uint16(i))
	}
	if v.getCardinality() != arrayDefaultMaxSize {
		t.Errorf("Bad cardinality.")
	}
	if reflect.TypeOf(v) != arraytype {
		t.Errorf("Should be an array.")
	}
	v = v.add(uint16(arrayDefaultMaxSize))
	if v.getCardinality() != arrayDefaultMaxSize+1 {
		t.Errorf("Bad cardinality.")
	}
	if reflect.TypeOf(v) == arraytype {
		t.Errorf("Should be a bitmap.")
	}
	v = v.remove(uint16(arrayDefaultMaxSize))
	if v.getCardinality() != arrayDefaultMaxSize {
		t.Errorf("Bad cardinality.")
	}
	if reflect.TypeOf(v) != arraytype {
		t.Errorf("Should be an array.")
	}
}

func TestArrayContainerRank(t *testing.T) {
	v := container(newArrayContainer())
	v = v.add(10)
	v = v.add(100)
	v = v.add(1000)
	if v.getCardinality() != 3 {
		t.Errorf("Bogus cardinality.")
	}
	for i := 0; i <= arrayDefaultMaxSize; i++ {
		thisrank := v.rank(uint16(i))
		if i < 10 {
			if thisrank != 0 {
				t.Errorf("At %d should be zero but is %d ", i, thisrank)
			}
		} else if i < 100 {
			if thisrank != 1 {
				t.Errorf("At %d should be zero but is %d ", i, thisrank)
			}
		} else if i < 1000 {
			if thisrank != 2 {
				t.Errorf("At %d should be zero but is %d ", i, thisrank)
			}
		} else {
			if thisrank != 3 {
				t.Errorf("At %d should be zero but is %d ", i, thisrank)
			}
		}
	}
}

func TestArrayContainerMassiveSetAndGet(t *testing.T) {
	v := container(newArrayContainer())
	for j := 0; j <= arrayDefaultMaxSize; j++ {

		v = v.add(uint16(j))
		if v.getCardinality() != 1+j {
			t.Errorf("Bogus cardinality %d %d. ", v.getCardinality(), j)
		}
		for i := 0; i <= arrayDefaultMaxSize; i++ {
			if i <= j {
				if v.contains(uint16(i)) != true {
					t.Errorf("I added a number in vain.")
				}
			} else {
				if v.contains(uint16(i)) != false {
					t.Errorf("Ghost content")
					break
				}
			}
		}
	}
}
