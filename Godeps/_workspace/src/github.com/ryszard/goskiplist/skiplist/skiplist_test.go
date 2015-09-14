// Copyright 2012 Google Inc. All rights reserved.
// Author: Ric Szopa (Ryszard) <ryszard.szopa@gmail.com>

// Package skiplist implements skip list based maps and sets.
//
// Skip lists are a data structure that can be used in place of
// balanced trees. Skip lists use probabilistic balancing rather than
// strictly enforced balancing and as a result the algorithms for
// insertion and deletion in skip lists are much simpler and
// significantly faster than equivalent algorithms for balanced trees.
//
// Skip lists were first described in Pugh, William (June 1990). "Skip
// lists: a probabilistic alternative to balanced
// trees". Communications of the ACM 33 (6): 668–676
package skiplist

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

func (s *SkipList) printRepr() {

	fmt.Printf("header:\n")
	for i, link := range s.header.forward {
		if link != nil {
			fmt.Printf("\t%d: -> %v\n", i, link.key)
		} else {
			fmt.Printf("\t%d: -> END\n", i)
		}
	}

	for node := s.header.next(); node != nil; node = node.next() {
		fmt.Printf("%v: %v (level %d)\n", node.key, node.value, len(node.forward))
		for i, link := range node.forward {
			if link != nil {
				fmt.Printf("\t%d: -> %v\n", i, link.key)
			} else {
				fmt.Printf("\t%d: -> END\n", i)
			}
		}
	}
	fmt.Println()
}

func TestInitialization(t *testing.T) {
	s := NewCustomMap(func(l, r interface{}) bool {
		return l.(int) < r.(int)
	})
	if !s.lessThan(1, 2) {
		t.Errorf("Less than doesn't work correctly.")
	}
}

func TestEmptyNodeNext(t *testing.T) {
	n := new(node)
	if next := n.next(); next != nil {
		t.Errorf("Next() should be nil for an empty node.")
	}

	if n.hasNext() {
		t.Errorf("hasNext() should be false for an empty node.")
	}
}

func TestEmptyNodePrev(t *testing.T) {
	n := new(node)
	if previous := n.previous(); previous != nil {
		t.Errorf("Previous() should be nil for an empty node.")
	}

	if n.hasPrevious() {
		t.Errorf("hasPrevious() should be false for an empty node.")
	}
}

func TestNodeHasNext(t *testing.T) {
	s := NewIntMap()
	s.Set(0, 0)
	node := s.header.next()
	if node.key != 0 {
		t.Fatalf("We got the wrong node: %v.", node)
	}

	if node.hasNext() {
		t.Errorf("%v should be the last node.", node)
	}
}

func TestNodeHasPrev(t *testing.T) {
	s := NewIntMap()
	s.Set(0, 0)
	node := s.header.previous()
	if node != nil {
		t.Fatalf("Expected no previous entry, got %v.", node)
	}
}

func (s *SkipList) check(t *testing.T, key, wanted int) {
	if got, _ := s.Get(key); got != wanted {
		t.Errorf("For key %v wanted value %v, got %v.", key, wanted, got)
	}
}

func TestGet(t *testing.T) {
	s := NewIntMap()
	s.Set(0, 0)

	if value, present := s.Get(0); !(value == 0 && present) {
		t.Errorf("%v, %v instead of %v, %v", value, present, 0, true)
	}

	if value, present := s.Get(100); value != nil || present {
		t.Errorf("%v, %v instead of %v, %v", value, present, nil, false)
	}
}

func TestGetGreaterOrEqual(t *testing.T) {
	s := NewIntMap()

	if _, value, present := s.GetGreaterOrEqual(5); !(value == nil && !present) {
		t.Errorf("s.GetGreaterOrEqual(5) should have returned nil and false for an empty map, not %v and %v.", value, present)
	}

	s.Set(0, 0)

	if _, value, present := s.GetGreaterOrEqual(5); !(value == nil && !present) {
		t.Errorf("s.GetGreaterOrEqual(5) should have returned nil and false for an empty map, not %v and %v.", value, present)
	}

	s.Set(10, 10)

	if key, value, present := s.GetGreaterOrEqual(5); !(value == 10 && key == 10 && present) {
		t.Errorf("s.GetGreaterOrEqual(5) should have returned 10 and true, not %v and %v.", value, present)
	}
}

func TestSet(t *testing.T) {
	s := NewIntMap()
	if l := s.Len(); l != 0 {
		t.Errorf("Len is not 0, it is %v", l)
	}

	s.Set(0, 0)
	s.Set(1, 1)
	if l := s.Len(); l != 2 {
		t.Errorf("Len is not 2, it is %v", l)
	}
	s.check(t, 0, 0)
	if t.Failed() {
		t.Errorf("header.Next() after s.Set(0, 0) and s.Set(1, 1): %v.", s.header.next())
	}
	s.check(t, 1, 1)

}

func TestChange(t *testing.T) {
	s := NewIntMap()
	s.Set(0, 0)
	s.Set(1, 1)
	s.Set(2, 2)

	s.Set(0, 7)
	if value, _ := s.Get(0); value != 7 {
		t.Errorf("Value should be 7, not %d", value)
	}
	s.Set(1, 8)
	if value, _ := s.Get(1); value != 8 {
		t.Errorf("Value should be 8, not %d", value)
	}

}

func TestDelete(t *testing.T) {
	s := NewIntMap()
	for i := 0; i < 10; i++ {
		s.Set(i, i)
	}
	for i := 0; i < 10; i += 2 {
		s.Delete(i)
	}

	for i := 0; i < 10; i += 2 {
		if _, present := s.Get(i); present {
			t.Errorf("%d should not be present in s", i)
		}
	}

	if v, present := s.Delete(10000); v != nil || present {
		t.Errorf("Deleting a non-existent key should return nil, false, and not %v, %v.", v, present)
	}

	if t.Failed() {
		s.printRepr()
	}

}

func TestLen(t *testing.T) {
	s := NewIntMap()
	for i := 0; i < 10; i++ {
		s.Set(i, i)
	}
	if length := s.Len(); length != 10 {
		t.Errorf("Length should be equal to 10, not %v.", length)
		s.printRepr()
	}
	for i := 0; i < 5; i++ {
		s.Delete(i)
	}
	if length := s.Len(); length != 5 {
		t.Errorf("Length should be equal to 5, not %v.", length)
	}

	s.Delete(10000)

	if length := s.Len(); length != 5 {
		t.Errorf("Length should be equal to 5, not %v.", length)
	}

}

func TestIteration(t *testing.T) {
	s := NewIntMap()
	for i := 0; i < 20; i++ {
		s.Set(i, i)
	}

	seen := 0
	var lastKey int

	i := s.Iterator()
	defer i.Close()

	for i.Next() {
		seen++
		lastKey = i.Key().(int)
		if i.Key() != i.Value() {
			t.Errorf("Wrong value for key %v: %v.", i.Key(), i.Value())
		}
	}

	if seen != s.Len() {
		t.Errorf("Not all the items in s where iterated through (seen %d, should have seen %d). Last one seen was %d.", seen, s.Len(), lastKey)
	}

	for i.Previous() {
		if i.Key() != i.Value() {
			t.Errorf("Wrong value for key %v: %v.", i.Key(), i.Value())
		}

		if i.Key().(int) >= lastKey {
			t.Errorf("Expected key to descend but ascended from %v to %v.", lastKey, i.Key())
		}

		lastKey = i.Key().(int)
	}

	if lastKey != 0 {
		t.Errorf("Expected to count back to zero, but stopped at key %v.", lastKey)
	}
}

func TestRangeIteration(t *testing.T) {
	s := NewIntMap()
	for i := 0; i < 20; i++ {
		s.Set(i, i)
	}

	max, min := 0, 100000
	var lastKey, seen int

	i := s.Range(5, 10)
	defer i.Close()

	for i.Next() {
		seen++
		lastKey = i.Key().(int)
		if lastKey > max {
			max = lastKey
		}
		if lastKey < min {
			min = lastKey
		}
		if i.Key() != i.Value() {
			t.Errorf("Wrong value for key %v: %v.", i.Key(), i.Value())
		}
	}

	if seen != 5 {
		t.Errorf("The number of items yielded is incorrect (should be 5, was %v)", seen)
	}
	if min != 5 {
		t.Errorf("The smallest element should have been 5, not %v", min)
	}

	if max != 9 {
		t.Errorf("The largest element should have been 9, not %v", max)
	}

	if i.Seek(4) {
		t.Error("Allowed to seek to invalid range.")
	}

	if !i.Seek(5) {
		t.Error("Could not seek to an allowed range.")
	}
	if i.Key().(int) != 5 || i.Value().(int) != 5 {
		t.Errorf("Expected 5 for key and 5 for value, got %d and %d", i.Key(), i.Value())
	}

	if !i.Seek(7) {
		t.Error("Could not seek to an allowed range.")
	}
	if i.Key().(int) != 7 || i.Value().(int) != 7 {
		t.Errorf("Expected 7 for key and 7 for value, got %d and %d", i.Key(), i.Value())
	}

	if i.Seek(10) {
		t.Error("Allowed to seek to invalid range.")
	}

	i.Seek(9)

	seen = 0
	min = 100000
	max = -1

	for i.Previous() {
		seen++
		lastKey = i.Key().(int)
		if lastKey > max {
			max = lastKey
		}
		if lastKey < min {
			min = lastKey
		}
		if i.Key() != i.Value() {
			t.Errorf("Wrong value for key %v: %v.", i.Key(), i.Value())
		}
	}

	if seen != 4 {
		t.Errorf("The number of items yielded is incorrect (should be 5, was %v)", seen)
	}
	if min != 5 {
		t.Errorf("The smallest element should have been 5, not %v", min)
	}

	if max != 8 {
		t.Errorf("The largest element should have been 9, not %v", max)
	}
}

func TestSomeMore(t *testing.T) {
	s := NewIntMap()
	insertions := [...]int{4, 1, 2, 9, 10, 7, 3}
	for _, i := range insertions {
		s.Set(i, i)
	}
	for _, i := range insertions {
		s.check(t, i, i)
	}

}

func makeRandomList(n int) *SkipList {
	s := NewIntMap()
	for i := 0; i < n; i++ {
		insert := rand.Int()
		s.Set(insert, insert)
	}
	return s
}

func LookupBenchmark(b *testing.B, n int) {
	b.StopTimer()
	s := makeRandomList(n)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		s.Get(rand.Int())
	}
}

func SetBenchmark(b *testing.B, n int) {
	b.StopTimer()
	values := []int{}
	for i := 0; i < b.N; i++ {
		values = append(values, rand.Int())
	}
	s := NewIntMap()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		s.Set(values[i], values[i])
	}
}

// Make sure that all the keys are unique and are returned in order.
func TestSanity(t *testing.T) {
	s := NewIntMap()
	for i := 0; i < 10000; i++ {
		insert := rand.Int()
		s.Set(insert, insert)
	}
	var last int = 0

	i := s.Iterator()
	defer i.Close()

	for i.Next() {
		if last != 0 && i.Key().(int) <= last {
			t.Errorf("Not in order!")
		}
		last = i.Key().(int)
	}

	for i.Previous() {
		if last != 0 && i.Key().(int) > last {
			t.Errorf("Not in order!")
		}
		last = i.Key().(int)
	}
}

type MyOrdered struct {
	value int
}

func (me MyOrdered) LessThan(other Ordered) bool {
	return me.value < other.(MyOrdered).value
}

func TestOrdered(t *testing.T) {
	s := New()
	s.Set(MyOrdered{0}, 0)
	s.Set(MyOrdered{1}, 1)

	if val, _ := s.Get(MyOrdered{0}); val != 0 {
		t.Errorf("Wrong value for MyOrdered{0}. Should have been %d.", val)
	}
}

func TestNewStringMap(t *testing.T) {
	s := NewStringMap()
	s.Set("a", 1)
	s.Set("b", 2)
	if value, _ := s.Get("a"); value != 1 {
		t.Errorf("Expected 1, got %v.", value)
	}
}

func TestGetNilKey(t *testing.T) {
	s := NewStringMap()
	if v, present := s.Get(nil); v != nil || present {
		t.Errorf("s.Get(nil) should return nil, false (not %v, %v).", v, present)
	}

}

func TestSetNilKey(t *testing.T) {
	s := NewStringMap()

	defer func() {
		if err := recover(); err == nil {
			t.Errorf("s.Set(nil, 0) should have panicked.")
		}
	}()

	s.Set(nil, 0)

}

func TestSetMaxLevelInFlight(t *testing.T) {
	s := NewIntMap()
	s.MaxLevel = 2
	for i := 0; i < 64; i++ {
		insert := 2 * rand.Int()
		s.Set(insert, insert)
	}

	s.MaxLevel = 64
	for i := 0; i < 65536; i++ {
		insert := 2*rand.Int() + 1
		s.Set(insert, insert)
	}

	i := s.Iterator()
	defer i.Close()

	for i.Next() {
		if v, _ := s.Get(i.Key()); v != i.Key() {
			t.Errorf("Bad values in the skip list (%v). Inserted before the call to s.SetMax(): %t.", v, i.Key().(int)%2 == 0)
		}
	}
}

func TestDeletingHighestLevelNodeDoesntBreakSkiplist(t *testing.T) {
	s := NewIntMap()
	elements := []int{1, 3, 5, 7, 0, 4, 5, 10, 11}

	for _, i := range elements {
		s.Set(i, i)
	}

	highestLevelNode := s.header.forward[len(s.header.forward)-1]

	s.Delete(highestLevelNode.key)

	seen := 0
	i := s.Iterator()
	defer i.Close()

	for i.Next() {
		seen++
	}
	if seen == 0 {
		t.Errorf("Iteration is broken (no elements seen).")
	}
}

func TestNewSet(t *testing.T) {
	set := NewIntSet()
	elements := []int{1, 3, 5, 7, 0, 4, 5}

	for _, i := range elements {
		set.Add(i)
	}

	if length := set.Len(); length != 6 {
		t.Errorf("set.Len() should be equal to 6, not %v.", length)
	}

	if !set.Contains(3) {
		t.Errorf("set should contain 3.")
	}

	if set.Contains(1000) {
		t.Errorf("set should not contain 1000.")
	}

	removed := set.Remove(1)

	if !removed {
		t.Errorf("Remove returned false for element that was present in set.")
	}

	seen := 0
	i := set.Iterator()
	defer i.Close()

	for i.Next() {
		seen++
	}

	if seen != 5 {
		t.Errorf("Iterator() iterated through %v elements. Should have been 5.", seen)
	}

	if set.Contains(1) {
		t.Errorf("1 was removed, set should not contain 1.")
	}

	if length := set.Len(); length != 5 {
		t.Errorf("After removing one element, set.Len() should be equal to 5, not %v.", length)
	}

	set.SetMaxLevel(10)
	if ml := set.GetMaxLevel(); ml != 10 {
		t.Errorf("MaxLevel for set should be 10, not %v", ml)
	}

}

func TestSetRangeIterator(t *testing.T) {
	set := NewIntSet()
	elements := []int{0, 1, 3, 5}

	for _, i := range elements {
		set.Add(i)
	}

	seen := 0
	for i := set.Range(2, 1000); i.Next(); {
		seen++
	}
	if seen != 2 {
		t.Errorf("There should have been 2 elements in Range(2, 1000), not %v.", seen)
	}

}

func TestNewStringSet(t *testing.T) {
	set := NewStringSet()
	strings := []string{"ala", "ma", "kota"}
	for _, v := range strings {
		set.Add(v)
	}

	if !set.Contains("ala") {
		t.Errorf("set should contain \"ala\".")
	}
}

func TestIteratorPrevHoles(t *testing.T) {
	m := NewIntMap()

	i := m.Iterator()
	defer i.Close()

	m.Set(0, 0)
	m.Set(1, 1)
	m.Set(2, 2)

	if !i.Next() {
		t.Errorf("Expected iterator to move successfully to the next.")
	}

	if !i.Next() {
		t.Errorf("Expected iterator to move successfully to the next.")
	}

	if !i.Next() {
		t.Errorf("Expected iterator to move successfully to the next.")
	}

	if i.Key().(int) != 2 || i.Value().(int) != 2 {
		t.Errorf("Expected iterator to reach key 2 and value 2, got %v and %v.", i.Key(), i.Value())
	}

	if !i.Previous() {
		t.Errorf("Expected iterator to move successfully to the previous.")
	}

	if i.Key().(int) != 1 || i.Value().(int) != 1 {
		t.Errorf("Expected iterator to reach key 1 and value 1, got %v and %v.", i.Key(), i.Value())
	}

	if !i.Next() {
		t.Errorf("Expected iterator to move successfully to the next.")
	}

	m.Delete(1)

	if !i.Previous() {
		t.Errorf("Expected iterator to move successfully to the previous.")
	}

	if i.Key().(int) != 0 || i.Value().(int) != 0 {
		t.Errorf("Expected iterator to reach key 0 and value 0, got %v and %v.", i.Key(), i.Value())
	}
}

func TestIteratorSeek(t *testing.T) {
	m := NewIntMap()

	i := m.Seek(0)

	if i != nil {
		t.Errorf("Expected nil iterator, but got %v.", i)
	}

	i = m.SeekToFirst()

	if i != nil {
		t.Errorf("Expected nil iterator, but got %v.", i)
	}

	i = m.SeekToLast()

	if i != nil {
		t.Errorf("Expected nil iterator, but got %v.", i)
	}

	m.Set(0, 0)

	i = m.SeekToFirst()
	defer i.Close()

	if i.Key().(int) != 0 || i.Value().(int) != 0 {
		t.Errorf("Expected iterator to reach key 0 and value 0, got %v and %v.", i.Key(), i.Value())
	}

	i = m.SeekToLast()
	defer i.Close()

	if i.Key().(int) != 0 || i.Value().(int) != 0 {
		t.Errorf("Expected iterator to reach key 0 and value 0, got %v and %v.", i.Key(), i.Value())
	}

	m.Set(1, 1)

	i = m.SeekToFirst()
	defer i.Close()

	if i.Key().(int) != 0 || i.Value().(int) != 0 {
		t.Errorf("Expected iterator to reach key 0 and value 0, got %v and %v.", i.Key(), i.Value())
	}

	i = m.SeekToLast()
	defer i.Close()

	if i.Key().(int) != 1 || i.Value().(int) != 1 {
		t.Errorf("Expected iterator to reach key 1 and value 1, got %v and %v.", i.Key(), i.Value())
	}

	m.Set(2, 2)

	i = m.SeekToFirst()
	defer i.Close()

	if i.Key().(int) != 0 || i.Value().(int) != 0 {
		t.Errorf("Expected iterator to reach key 0 and value 0, got %v and %v.", i.Key(), i.Value())
	}

	i = m.SeekToLast()
	defer i.Close()

	if i.Key().(int) != 2 || i.Value().(int) != 2 {
		t.Errorf("Expected iterator to reach key 2 and value 2, got %v and %v.", i.Key(), i.Value())
	}

	i = m.Seek(0)
	defer i.Close()

	if i.Key().(int) != 0 || i.Value().(int) != 0 {
		t.Errorf("Expected iterator to reach key 0 and value 0, got %v and %v.", i.Key(), i.Value())
	}

	i = m.Seek(2)
	defer i.Close()

	if i.Key().(int) != 2 || i.Value().(int) != 2 {
		t.Errorf("Expected iterator to reach key 2 and value 2, got %v and %v.", i.Key(), i.Value())
	}

	i = m.Seek(1)
	defer i.Close()

	if i.Key().(int) != 1 || i.Value().(int) != 1 {
		t.Errorf("Expected iterator to reach key 1 and value 1, got %v and %v.", i.Key(), i.Value())
	}

	i = m.Seek(3)

	if i != nil {
		t.Errorf("Expected to receive nil iterator, got %v.", i)
	}

	m.Set(4, 4)

	i = m.Seek(4)
	defer i.Close()

	if i.Key().(int) != 4 || i.Value().(int) != 4 {
		t.Errorf("Expected iterator to reach key 4 and value 4, got %v and %v.", i.Key(), i.Value())
	}

	i = m.Seek(3)
	defer i.Close()

	if i.Key().(int) != 4 || i.Value().(int) != 4 {
		t.Errorf("Expected iterator to reach key 4 and value 4, got %v and %v.", i.Key(), i.Value())
	}

	m.Delete(4)

	i = m.SeekToFirst()
	defer i.Close()

	if i.Key().(int) != 0 || i.Value().(int) != 0 {
		t.Errorf("Expected iterator to reach key 0 and value 0, got %v and %v.", i.Key(), i.Value())
	}

	i = m.SeekToLast()
	defer i.Close()

	if i.Key().(int) != 2 || i.Value().(int) != 2 {
		t.Errorf("Expected iterator to reach key 2 and value 2, got %v and %v.", i.Key(), i.Value())
	}

	if !i.Seek(2) {
		t.Error("Expected iterator to seek to key.")
	}

	if i.Key().(int) != 2 || i.Value().(int) != 2 {
		t.Errorf("Expected iterator to reach key 2 and value 2, got %v and %v.", i.Key(), i.Value())
	}

	if !i.Seek(1) {
		t.Error("Expected iterator to seek to key.")
	}

	if i.Key().(int) != 1 || i.Value().(int) != 1 {
		t.Errorf("Expected iterator to reach key 1 and value 1, got %v and %v.", i.Key(), i.Value())
	}

	if !i.Seek(0) {
		t.Error("Expected iterator to seek to key.")
	}

	if i.Key().(int) != 0 || i.Value().(int) != 0 {
		t.Errorf("Expected iterator to reach key 0 and value 0, got %v and %v.", i.Key(), i.Value())
	}

	i = m.SeekToFirst()
	defer i.Close()

	if !i.Seek(0) {
		t.Error("Expected iterator to seek to key.")
	}

	if i.Key().(int) != 0 || i.Value().(int) != 0 {
		t.Errorf("Expected iterator to reach key 0 and value 0, got %v and %v.", i.Key(), i.Value())
	}
}

func BenchmarkLookup16(b *testing.B) {
	LookupBenchmark(b, 16)
}

func BenchmarkLookup256(b *testing.B) {
	LookupBenchmark(b, 256)
}

func BenchmarkLookup65536(b *testing.B) {
	LookupBenchmark(b, 65536)
}

func BenchmarkSet16(b *testing.B) {
	SetBenchmark(b, 16)
}

func BenchmarkSet256(b *testing.B) {
	SetBenchmark(b, 256)
}

func BenchmarkSet65536(b *testing.B) {
	SetBenchmark(b, 65536)
}

func BenchmarkRandomSeek(b *testing.B) {
	b.StopTimer()
	values := []int{}
	s := NewIntMap()
	for i := 0; i < b.N; i++ {
		r := rand.Int()
		values = append(values, r)
		s.Set(r, r)
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		iterator := s.Seek(values[i])
		if iterator == nil {
			b.Errorf("got incorrect value for index %d", i)
		}
	}
}

const (
	lookAhead = 10
)

// This test is used for the baseline comparison of Iterator.Seek when
// performing forward sequential seek operations.
func BenchmarkForwardSeek(b *testing.B) {
	b.StopTimer()

	values := []int{}
	s := NewIntMap()
	valueCount := b.N
	for i := 0; i < valueCount; i++ {
		r := rand.Int()
		values = append(values, r)
		s.Set(r, r)
	}
	sort.Ints(values)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		key := values[i]
		iterator := s.Seek(key)
		if i < valueCount-lookAhead {
			nextKey := values[i+lookAhead]

			iterator = s.Seek(nextKey)
			if iterator.Key().(int) != nextKey || iterator.Value().(int) != nextKey {
				b.Errorf("%d. expected %d key and %d value, got %d key and %d value", i, nextKey, nextKey, iterator.Key(), iterator.Value())
			}
		}
	}
}

// This test demonstrates the amortized cost of a forward sequential seek.
func BenchmarkForwardSeekReusedIterator(b *testing.B) {
	b.StopTimer()

	values := []int{}
	s := NewIntMap()
	valueCount := b.N
	for i := 0; i < valueCount; i++ {
		r := rand.Int()
		values = append(values, r)
		s.Set(r, r)

	}
	sort.Ints(values)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		key := values[i]
		iterator := s.Seek(key)
		if i < valueCount-lookAhead {
			nextKey := values[i+lookAhead]

			if !iterator.Seek(nextKey) {
				b.Errorf("%d. expected iterator to seek to %d key; failed.", i, nextKey)
			} else if iterator.Key().(int) != nextKey || iterator.Value().(int) != nextKey {
				b.Errorf("%d. expected %d key and %d value, got %d key and %d value", i, nextKey, nextKey, iterator.Key(), iterator.Value())
			}
		}
	}
}
