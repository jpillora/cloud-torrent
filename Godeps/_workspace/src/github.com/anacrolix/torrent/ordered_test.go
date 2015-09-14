package torrent

import (
	"testing"
)

func TestOrderedList(t *testing.T) {
	ol := newOrderedList(func(a, b interface{}) bool {
		return a.(int) < b.(int)
	})
	if ol.Len() != 0 {
		t.FailNow()
	}
	e := ol.Insert(0)
	if ol.Len() != 1 {
		t.FailNow()
	}
	if e.Value.(int) != 0 {
		t.FailNow()
	}
	e = ol.Front()
	if e.Value.(int) != 0 {
		t.FailNow()
	}
	if e.Next() != nil {
		t.FailNow()
	}
	ol.Insert(1)
	if e.Next().Value.(int) != 1 {
		t.FailNow()
	}
	ol.Insert(-1)
	if e.Prev().Value.(int) != -1 {
		t.FailNow()
	}
	e.Value = -2
	ol.ValueChanged(e)
	if e.Prev() != nil {
		t.FailNow()
	}
	if e.Next().Value.(int) != -1 {
		t.FailNow()
	}
	if ol.Len() != 3 {
		t.FailNow()
	}
}
