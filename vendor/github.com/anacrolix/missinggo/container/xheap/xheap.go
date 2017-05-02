package xheap

import (
	"container/heap"
	"sort"
)

type pushPopper interface {
	Push(interface{})
	Pop() interface{}
}

func Flipped(h heap.Interface) heap.Interface {
	return struct {
		sort.Interface
		pushPopper
	}{
		sort.Reverse(h),
		h,
	}
}

// type top struct {
//  k int
//  heap.Interface
// }

// func (me top) Push(x interface{}) {
//  heap.Push(me.Interface, x)
//  if me.Len() > me.k {
//      heap.Pop(me)
//  }
// }

// func Bounded(k int, h heap.Interface) heap.Interface {
//  return top{k, Flipped(h)}
// }

type slice struct {
	Slice  *[]interface{}
	Lesser func(l, r interface{}) bool
}

func (me slice) Len() int { return len(*me.Slice) }

func (me slice) Less(i, j int) bool {
	return me.Lesser((*me.Slice)[i], (*me.Slice)[j])
}

func (me slice) Pop() (ret interface{}) {
	i := me.Len() - 1
	ret = (*me.Slice)[i]
	*me.Slice = (*me.Slice)[:i]
	return
}

func (me slice) Push(x interface{}) {
	*me.Slice = append(*me.Slice, x)
}

func (me slice) Swap(i, j int) {
	sl := *me.Slice
	sl[i], sl[j] = sl[j], sl[i]
}

func Slice(sl *[]interface{}, lesser func(l, r interface{}) bool) heap.Interface {
	return slice{sl, lesser}
}
