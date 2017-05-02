package sync

import (
	"bytes"
	"testing"
)

func init() {
	Enable()
}

func TestLog(t *testing.T) {
	var mu Mutex
	mu.Lock()
	mu.Unlock()
}

func TestRWMutex(t *testing.T) {
	var mu RWMutex
	mu.RLock()
	mu.RUnlock()
}

func TestPointerCompare(t *testing.T) {
	a, b := new(int), new(int)
	if a == b {
		t.FailNow()
	}
}

func TestLockTime(t *testing.T) {
	var buf bytes.Buffer
	PrintLockTimes(&buf)
	t.Log(buf.String())
}
