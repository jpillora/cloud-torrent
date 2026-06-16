package server

import (
	"sync"

	"github.com/jpillora/cloud-torrent/engine"
	"github.com/jpillora/velox"
)

// State is the root object synced to every connected browser via velox. Search
// providers are added with the search backend task.
type State struct {
	velox.State `json:"-"`
	sync.Mutex  `json:"-"`

	Config    engine.Config
	Downloads *FsNode
	Torrents  map[string]*engine.Torrent
	Users     map[string]string
	Stats     Stats
}
