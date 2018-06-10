// Package sync is an extension of the stdlib "sync" package. It has extra
// functionality that helps debug the use of synchronization primitives. The
// package should be importable in place of "sync". The extra functionality
// can be enabled by calling Enable() or passing a non-empty PPROF_SYNC
// environment variable to the process.
//
// Several profiles are exposed on the default HTTP muxer (and to
// "/debug/pprof" when "net/http/pprof" is imported by the process).
// "lockHolders" lists the stack traces of goroutines that called Mutex.Lock
// that haven't subsequently been Unlocked. "lockBlockers" contains goroutines
// that are waiting to obtain locks. "/debug/lockTimes" or PrintLockTimes()
// shows the longest time a lock is held for each stack trace.
//
// Note that currently RWMutex is treated like a Mutex when the package is
// enabled.
package sync

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/anacrolix/missinggo"
)

var (
	// Protects initialization and enabling of the package.
	enableMu sync.Mutex
	// Whether any of this package is to be active.
	enabled = false
	// Current lock holders.
	lockHolders *pprof.Profile
	// Those blocked on acquiring a lock.
	lockBlockers *pprof.Profile

	// Stats on lock usage by call graph.
	lockStatsMu      sync.Mutex
	lockStatsByStack map[lockStackKey]lockStats
)

type (
	lockStats struct {
		min   time.Duration
		max   time.Duration
		total time.Duration
		count lockCount
	}
	lockStackKey = [32]uintptr
	lockCount    = int64
)

func (me *lockStats) MeanTime() time.Duration {
	return me.total / time.Duration(me.count)
}

type stackLockStats struct {
	stack lockStackKey
	lockStats
}

func sortedLockTimes() (ret []stackLockStats) {
	lockStatsMu.Lock()
	for stack, stats := range lockStatsByStack {
		ret = append(ret, stackLockStats{stack, stats})
	}
	lockStatsMu.Unlock()
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].total > ret[j].total
	})
	return
}

// Writes out the longest time a Mutex remains locked for each stack trace
// that locks a Mutex.
func PrintLockTimes(w io.Writer) {
	lockTimes := sortedLockTimes()
	tw := tabwriter.NewWriter(w, 1, 8, 1, '\t', 0)
	defer tw.Flush()
	w = tw
	for _, elem := range lockTimes {
		fmt.Fprintf(w, "%s (%s * %d [%s, %s])\n", elem.total, elem.MeanTime(), elem.count, elem.min, elem.max)
		missinggo.WriteStack(w, elem.stack[:])
	}
}

func Enable() {
	enableMu.Lock()
	defer enableMu.Unlock()
	if enabled {
		return
	}
	lockStatsByStack = make(map[lockStackKey]lockStats)
	lockHolders = pprof.NewProfile("lockHolders")
	lockBlockers = pprof.NewProfile("lockBlockers")
	http.DefaultServeMux.HandleFunc("/debug/lockTimes", func(w http.ResponseWriter, r *http.Request) {
		PrintLockTimes(w)
	})
	enabled = true
}

func init() {
	if os.Getenv("PPROF_SYNC") != "" {
		Enable()
	}
}

type Mutex struct {
	mu      sync.Mutex
	hold    *int        // Unique value for passing to pprof.
	stack   [32]uintptr // The stack for the current holder.
	start   time.Time   // When the lock was obtained.
	entries int         // Number of entries returned from runtime.Callers.
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
	m.entries = runtime.Callers(2, m.stack[:])
	m.start = time.Now()
}

func (m *Mutex) Unlock() {
	if enabled {
		d := time.Since(m.start)
		var key [32]uintptr
		copy(key[:], m.stack[:m.entries])
		go func() {
			lockStatsMu.Lock()
			defer lockStatsMu.Unlock()
			v, ok := lockStatsByStack[key]
			if !ok {
				v.min = math.MaxInt64
			}
			if d > v.max {
				v.max = d
			}
			if d < v.min {
				v.min = d
			}
			v.total += d
			v.count++
			lockStatsByStack[key] = v
		}()
		go lockHolders.Remove(m.hold)
	}
	m.mu.Unlock()
}

type (
	WaitGroup = sync.WaitGroup
	Cond      = sync.Cond
)
