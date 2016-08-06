package fs

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/jpillora/backoff"
	"github.com/jpillora/velox"
)

//shared file system state
type State struct {
	sync.Locker  `json:"-"`
	velox.Pusher `json:"-"`
	Enabled      bool           `json:",omitempty"`
	Syncing      bool           `json:",omitempty"`
	Config       interface{}    `json:",omitempty"`
	Root         json.Marshaler `json:",omitempty"`
	Error        string         `json:",omitempty"`
}

//startFSSync runs once after the first
//successful configure, then loops fs.Update()
//forever, with exponential backoff on failures.
func (s *State) Sync(f FS) {
	name := f.Name()
	updates := make(chan Node)
	//monitor and sync updates
	go func() {
		for node := range updates {
			s.Lock()
			log.Printf("[%s] updated", name)
			s.Root = node
			s.Unlock()
			s.Push()
		}
	}()
	//sync loop forever
	go func() {
		b := backoff.Backoff{Max: 2 * time.Minute}
		for {
			//retrieve updates
			err := f.Update(updates)
			e := ""
			d := 30 * time.Second
			if err == nil {
				b.Reset()
			} else {
				log.Printf("[%s] sync failed: %s", name, err)
				e = err.Error()
				d = b.Duration()
			}
			//show result
			s.Lock()
			s.Error = e
			s.Unlock()
			s.Push()
			//retry after sleep
			time.Sleep(d)
		}
	}()
	log.Printf("[%s] Sync started", name)
}
