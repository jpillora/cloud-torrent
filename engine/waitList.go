package engine

import (
	"container/list"
	"fmt"
	"sync"
)

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

type taskType int

const (
	taskTorrent taskType = iota
	taskMagnet
)

type taskElem struct {
	ih string
	tp taskType
}

func (t taskElem) Filename() string {

	var suffix string
	switch t.tp {
	case taskTorrent:
		suffix = ".torrent"
	case taskMagnet:
		suffix = ".info"
	}

	return fmt.Sprintf("%s%s%s", cacheSavedPrefix, t.ih, suffix)
}
