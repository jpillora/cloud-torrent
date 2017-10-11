package perf

import (
	"sync"
	"time"
)

type event struct {
	mu    sync.RWMutex
	count int64
	total time.Duration
	min   time.Duration
	max   time.Duration
}

func (e *event) Add(t time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if t > e.max {
		e.max = t
	}
	if e.min == 0 || t < e.min {
		e.min = t
	}
	e.count++
	e.total += t
}

func (e *event) mean() time.Duration {
	return e.total / time.Duration(e.count)
}
