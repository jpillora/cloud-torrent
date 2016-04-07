package missinggo

import "sync"

type Event struct {
	mu     sync.Mutex
	ch     chan struct{}
	closed bool
}

func (me *Event) lazyInit() {
	if me.ch == nil {
		me.ch = make(chan struct{})
	}
}

func (me *Event) C() <-chan struct{} {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.lazyInit()
	return me.ch
}

func (me *Event) Clear() {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.lazyInit()
	if !me.closed {
		return
	}
	me.ch = make(chan struct{})
	me.closed = false
}

func (me *Event) Set() (first bool) {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.lazyInit()
	if me.closed {
		return false
	}
	close(me.ch)
	me.closed = true
	return true
}

func (me *Event) IsSet() bool {
	me.mu.Lock()
	defer me.mu.Unlock()
	select {
	case <-me.ch:
		return true
	default:
		return false
	}
}

func (me *Event) Wait() {
	<-me.C()
}
