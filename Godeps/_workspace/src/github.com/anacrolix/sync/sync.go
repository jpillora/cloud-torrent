package sync

import (
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
)

var enabled = false

var (
	lockHolders  *pprof.Profile
	lockBlockers *pprof.Profile
	// mu           sync.Mutex
)

func init() {
	if os.Getenv("PPROF_SYNC") != "" {
		enabled = true
	}
	if enabled {
		lockHolders = pprof.NewProfile("lockHolders")
		lockBlockers = pprof.NewProfile("lockBlockers")
	}
}

type lockAction struct {
	time.Time
	*Mutex
	Stack string
}

func stack() string {
	var buf [0x1000]byte
	n := runtime.Stack(buf[:], false)
	return string(buf[:n])
}

type Mutex struct {
	mu   sync.Mutex
	hold *int
}

func (m *Mutex) newAction() *lockAction {
	return &lockAction{
		time.Now(),
		m,
		stack(),
	}
}
func (m *Mutex) Lock() {
	if !enabled {
		m.mu.Lock()
		return
	}
	v := new(int)
	lockBlockers.Add(v, 0)
	m.mu.Lock()
	lockBlockers.Remove(v)
	m.hold = v
	lockHolders.Add(v, 0)
}
func (m *Mutex) Unlock() {
	if enabled {
		lockHolders.Remove(m.hold)
	}
	m.mu.Unlock()
}

type WaitGroup struct {
	sync.WaitGroup
}

type Cond struct {
	sync.Cond
}

// This RWMutex's RLock and RUnlock methods don't allow shared reading because
// there's no way to determine what goroutine has stopped holding the read
// lock when RUnlock is called. So for debugging purposes, it's just like
// Mutex.
type RWMutex struct {
	Mutex
}

func (me *RWMutex) RLock() {
	me.Lock()
}
func (me *RWMutex) RUnlock() {
	me.Unlock()
}
