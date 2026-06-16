package server

import (
	"sync"

	"github.com/jpillora/velox"
)

// State is the root object synced to every connected browser via velox. It
// currently carries only a demo counter; the engine config, torrents,
// downloads, connected users and system stats are added by the root-state task.
type State struct {
	velox.State `json:"-"`
	sync.Mutex  `json:"-"`

	// Tick is a demo counter, incremented server-side, used to prove realtime
	// sync end-to-end. It is removed once the real state lands.
	Tick int
}
