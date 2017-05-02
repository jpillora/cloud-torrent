package prioritybitmap

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/anacrolix/missinggo/iter"
)

func TestEmpty(t *testing.T) {
	var pb PriorityBitmap
	it := iter.NewIterator(&pb)
	assert.Panics(t, func() { it.Value() })
	assert.False(t, it.Next())
}

func TestIntBounds(t *testing.T) {
	var pb PriorityBitmap
	pb.Set(math.MaxInt32, math.MinInt32)
	pb.Set(math.MinInt32, math.MaxInt32)
	assert.EqualValues(t, []interface{}{math.MaxInt32, math.MinInt32}, iter.IterableAsSlice(&pb))
}

func TestDistinct(t *testing.T) {
	var pb PriorityBitmap
	pb.Set(0, 0)
	pb.Set(1, 1)
	assert.EqualValues(t, []interface{}{0, 1}, iter.IterableAsSlice(&pb))
	pb.Set(0, -1)
	assert.EqualValues(t, []interface{}{0, 1}, iter.IterableAsSlice(&pb))
	pb.Set(1, -2)
	assert.EqualValues(t, []interface{}{1, 0}, iter.IterableAsSlice(&pb))
}

func TestNextAfterIterFinished(t *testing.T) {
	var pb PriorityBitmap
	pb.Set(0, 0)
	it := iter.NewIterator(&pb)
	assert.True(t, it.Next())
	assert.False(t, it.Next())
	assert.False(t, it.Next())
}

func TestRemoveWhileIterating(t *testing.T) {
	var pb PriorityBitmap
	pb.Set(0, 0)
	pb.Set(1, 1)
	it := iter.NewIterator(&pb)
	go it.Stop()
	pb.Remove(0)
	time.Sleep(time.Millisecond)
	// This should return an empty list, as the iterator was stopped before
	// Next was called.
	assert.EqualValues(t, []interface{}(nil), iter.ToSlice(it))
}

func TestDoubleRemove(t *testing.T) {
	var pb PriorityBitmap
	pb.Set(0, 0)
	pb.Remove(0)
	assert.NotPanics(t, func() { pb.Remove(0) })
}
