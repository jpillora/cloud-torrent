package engine

import (
	"container/list"
	"sync"
)

// syncList is a FIFO queue
type syncList struct {
	lst *list.List
	sync.Mutex
}

func NewSyncList() *syncList {
	return &syncList{
		lst: list.New(),
	}
}

func (l *syncList) Push(v interface{}) *list.Element {
	l.Lock()
	defer l.Unlock()
	return l.lst.PushBack(v)
}

func (l *syncList) Pop() interface{} {
	l.Lock()
	defer l.Unlock()
	if elm := l.lst.Front(); elm != nil {
		return l.lst.Remove(elm)
	}
	return nil
}

func (l *syncList) Remove(ih string) {
	l.Lock()
	defer l.Unlock()

	for temp := l.lst.Front(); temp != nil; temp = temp.Next() {
		if elm, ok := temp.Value.(taskElem); ok && elm.ih == ih {
			l.lst.Remove(temp)
			log.Println("syncList removed ih", ih)
			break
		}
	}
}

func (l *syncList) Len() int {
	return l.lst.Len()
}

type taskType uint8

const (
	taskTorrent taskType = iota
	taskMagnet
)

type taskElem struct {
	ih string
	tp taskType
}
