package torrent

import (
	"container/list"
)

// This was used to maintain pieces in order of bytes left to download. I
// don't think it's currently in use.

type orderedList struct {
	list     *list.List
	lessFunc func(a, b interface{}) bool
}

func (me *orderedList) Len() int {
	return me.list.Len()
}

func newOrderedList(lessFunc func(a, b interface{}) bool) *orderedList {
	return &orderedList{
		list:     list.New(),
		lessFunc: lessFunc,
	}
}

func (me *orderedList) ValueChanged(e *list.Element) {
	for prev := e.Prev(); prev != nil && me.lessFunc(e.Value, prev.Value); prev = e.Prev() {
		me.list.MoveBefore(e, prev)
	}
	for next := e.Next(); next != nil && me.lessFunc(next.Value, e.Value); next = e.Next() {
		me.list.MoveAfter(e, next)
	}
}

func (me *orderedList) Insert(value interface{}) (ret *list.Element) {
	ret = me.list.PushFront(value)
	me.ValueChanged(ret)
	return
}

func (me *orderedList) Front() *list.Element {
	return me.list.Front()
}

func (me *orderedList) Remove(e *list.Element) interface{} {
	return me.list.Remove(e)
}
