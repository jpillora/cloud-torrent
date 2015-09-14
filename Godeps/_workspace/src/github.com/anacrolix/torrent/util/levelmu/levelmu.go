package levelmu

import (
	"sync"
)

type LevelMutex struct {
	mus []sync.Mutex
	// Protected by the very last mutex.
	lastLevel int
}

func (lm *LevelMutex) Init(levels int) {
	if lm.mus != nil {
		panic("level mutex already initialized")
	}
	lm.mus = make([]sync.Mutex, levels)
}

func (lm *LevelMutex) Lock() {
	lm.LevelLock(0)
}

func (lm *LevelMutex) Unlock() {
	stopLevel := lm.lastLevel
	for i := len(lm.mus) - 1; i >= stopLevel; i-- {
		lm.mus[i].Unlock()
	}
}

func (lm *LevelMutex) LevelLock(level int) {
	if level >= len(lm.mus) {
		panic("lock level exceeds configured level count")
	}
	for l := level; l < len(lm.mus); l++ {
		lm.mus[l].Lock()
	}
	lm.lastLevel = level
}
