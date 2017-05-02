package missinggo

import (
	"runtime"
	"testing"
	"time"

	"github.com/bradfitz/iter"
)

// Put defer GoroutineLeakCheck(t)() at the top of your test. Make sure the
// goroutine count is steady before your test begins.
func GoroutineLeakCheck(t testing.TB) func() {
	if !testing.Verbose() {
		return func() {}
	}
	numStart := runtime.NumGoroutine()
	return func() {
		var numNow int
		wait := time.Millisecond
		started := time.Now()
		for range iter.N(10) { // 1 second
			numNow = runtime.NumGoroutine()
			if numNow <= numStart {
				break
			}
			t.Logf("%d excess goroutines after %s", numNow-numStart, time.Since(started))
			time.Sleep(wait)
			wait *= 2
		}
		// I'd print stacks, or treat this as fatal, but I think
		// runtime.NumGoroutine is including system routines for which we are
		// not provided the stacks, and are spawned unpredictably.
		t.Logf("have %d goroutines, started with %d", numNow, numStart)
		// select {}
	}
}
