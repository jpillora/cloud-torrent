package perf

import (
	"log"
	"time"
)

type Timer struct {
	started time.Time
	log     bool
	name    string
}

func NewTimer(opts ...timerOpt) (t Timer) {
	t.started = time.Now()
	for _, o := range opts {
		o(&t)
	}
	if t.log && t.name != "" {
		log.Printf("starting timer %q", t.name)
	}
	return
}

type timerOpt func(*Timer)

func Log(t *Timer) {
	t.log = true
}

func Name(name string) func(*Timer) {
	return func(t *Timer) {
		t.name = name
	}
}

func (t *Timer) Mark(events ...string) time.Duration {
	d := time.Since(t.started)
	for _, e := range events {
		if t.name != "" {
			e = t.name + "/" + e
		}
		t.addDuration(e, d)
	}
	return d
}

func (t *Timer) addDuration(desc string, d time.Duration) {
	mu.RLock()
	e := events[desc]
	mu.RUnlock()
	if e == nil {
		mu.Lock()
		e = events[desc]
		if e == nil {
			e = new(event)
			events[desc] = e
		}
		mu.Unlock()
	}
	e.Add(d)
	if t.log {
		if t.name != "" {
			log.Printf("timer %q got event %q after %s", t.name, desc, d)
		} else {
			log.Printf("marking event %q after %s", desc, d)
		}
	}
}
