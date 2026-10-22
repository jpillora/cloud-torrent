package engine

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// Engine wraps anacrolix/torrent and maintains a cache of view-models keyed by
// infohash hex string. mut owns the client, configuration and all cached models.
// Public methods lock it; helpers suffixed Locked require the caller to hold it.
type Engine struct {
	mut      sync.Mutex
	cacheDir string
	client   *torrent.Client
	config   Config
	ts       map[string]*Torrent
}

// New returns an unconfigured engine. Call Configure before adding torrents.
func New() *Engine {
	return &Engine{ts: map[string]*Torrent{}}
}

// Config returns the engine's current configuration.
func (e *Engine) Config() Config {
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.config
}

// Configure (re)creates the underlying torrent client from c. An existing
// client is closed first.
func (e *Engine) Configure(c Config) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client != nil {
		e.client.Close()
		time.Sleep(1 * time.Second)
	}
	c.IncomingPort = defaultPort(c.IncomingPort)
	config := torrent.NewDefaultClientConfig()
	config.DataDir = c.DownloadDirectory
	config.NoUpload = !c.EnableUpload
	config.Seed = c.EnableSeeding
	config.ListenPort = c.IncomingPort
	client, err := torrent.NewClient(config)
	if err != nil {
		return err
	}
	e.config = c
	e.client = client
	e.refreshLocked()
	return nil
}

// Close shuts down the underlying torrent client.
func (e *Engine) Close() error {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client != nil {
		e.client.Close()
		e.client = nil
	}
	return nil
}

// NewMagnet adds a torrent from a magnet URI.
func (e *Engine) NewMagnet(magnetURI string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return fmt.Errorf("engine is not configured")
	}
	tt, err := e.client.AddMagnet(magnetURI)
	if err != nil {
		return err
	}
	return e.newTorrentLocked(tt)
}

// NewTorrent adds a torrent from a parsed .torrent spec.
func (e *Engine) NewTorrent(spec *torrent.TorrentSpec) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return fmt.Errorf("engine is not configured")
	}
	tt, _, err := e.client.AddTorrentSpec(spec)
	if err != nil {
		return err
	}
	return e.newTorrentLocked(tt)
}

func (e *Engine) newTorrentLocked(tt *torrent.Torrent) error {
	t := e.upsertTorrentLocked(tt)
	// once metadata arrives, auto-start if configured
	go func() {
		<-tt.GotInfo()
		e.mut.Lock()
		defer e.mut.Unlock()
		// A metadata callback must not act on a removed/replaced torrent.
		if e.client != nil && e.ts[t.InfoHash] == t && t.t == tt && e.config.AutoStart {
			e.startTorrentLocked(t.InfoHash)
		}
	}()
	return nil
}

// GetTorrents returns a detached snapshot. The server may retain and serialize
// it independently; later engine mutations cannot alter it.
func (e *Engine) GetTorrents() map[string]*Torrent {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return nil
	}
	e.refreshLocked()
	snapshot := make(map[string]*Torrent, len(e.ts))
	for ih, t := range e.ts {
		copy := *t
		copy.t = nil
		copy.updatedAt = time.Time{}
		if t.Files != nil {
			copy.Files = make([]*File, len(t.Files))
			for i, f := range t.Files {
				if f == nil {
					continue
				}
				file := *f
				file.f = nil
				copy.Files[i] = &file
			}
		}
		snapshot[ih] = &copy
	}
	return snapshot
}

func (e *Engine) refreshLocked() {
	for _, tt := range e.client.Torrents() {
		e.upsertTorrentLocked(tt)
	}
}

func (e *Engine) upsertTorrentLocked(tt *torrent.Torrent) *Torrent {
	ih := tt.InfoHash().HexString()
	t, ok := e.ts[ih]
	if !ok {
		t = &Torrent{InfoHash: ih}
		e.ts[ih] = t
	}
	t.Update(tt)
	return t
}

func (e *Engine) getTorrentLocked(infohash string) (*Torrent, error) {
	if e.client == nil {
		return nil, fmt.Errorf("engine is not configured")
	}
	ih, err := str2ih(infohash)
	if err != nil {
		return nil, err
	}
	t, ok := e.ts[ih.HexString()]
	if !ok {
		return nil, fmt.Errorf("missing torrent %x", ih)
	}
	return t, nil
}

// StartTorrent begins downloading a torrent (and all its files).
func (e *Engine) StartTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.startTorrentLocked(infohash)
}

func (e *Engine) startTorrentLocked(infohash string) error {
	t, err := e.getTorrentLocked(infohash)
	if err != nil {
		return err
	}
	if t.Started {
		return fmt.Errorf("already started")
	}
	t.Started = true
	for _, f := range t.Files {
		if f != nil {
			f.Started = true
		}
	}
	if t.t.Info() != nil {
		t.t.DownloadAll()
	}
	return nil
}

// StopTorrent stops a torrent. anacrolix/torrent has no pause, so the
// underlying torrent is dropped.
func (e *Engine) StopTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	t, err := e.getTorrentLocked(infohash)
	if err != nil {
		return err
	}
	if !t.Started {
		return fmt.Errorf("already stopped")
	}
	t.Started = false
	for _, f := range t.Files {
		if f != nil {
			f.Started = false
		}
	}
	// Drop can block until the torrent's goroutines unwind (notably under active
	// download). Run it in the background so the API call returns immediately and
	// the UI flips to stopped/"Remove" without a multi-second hang.
	go t.t.Drop()
	return nil
}

// DeleteTorrent removes a torrent from the engine and drops it from the client.
func (e *Engine) DeleteTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	t, err := e.getTorrentLocked(infohash)
	if err != nil {
		return err
	}
	os.Remove(filepath.Join(e.cacheDir, infohash+".torrent"))
	delete(e.ts, t.InfoHash)
	ih, _ := str2ih(infohash)
	if tt, ok := e.client.Torrent(ih); ok {
		tt.Drop()
	}
	return nil
}

// StartFile marks a single file for download.
func (e *Engine) StartFile(infohash, path string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	t, err := e.getTorrentLocked(infohash)
	if err != nil {
		return err
	}
	var f *File
	for _, file := range t.Files {
		if file.Path == path {
			f = file
			break
		}
	}
	if f == nil {
		return fmt.Errorf("missing file %s", path)
	}
	if f.Started {
		return fmt.Errorf("already started")
	}
	t.Started = true
	f.Started = true
	return nil
}

// StopFile is not supported by the underlying engine (parity with master).
func (e *Engine) StopFile(infohash, path string) error {
	return fmt.Errorf("unsupported")
}

// defaultPort falls back to 50007 when the configured port is out of range,
// matching the original cloud-torrent behaviour.
func defaultPort(p int) int {
	if p <= 0 || p >= 65535 {
		return 50007
	}
	return p
}

// str2ih parses a 40-character hex infohash into a metainfo.Hash.
func str2ih(str string) (metainfo.Hash, error) {
	var ih metainfo.Hash
	// guard length first: hex.Decode into the fixed 20-byte array would panic
	// on an over-long input.
	if len(str) != 40 {
		return ih, fmt.Errorf("invalid length")
	}
	n, err := hex.Decode(ih[:], []byte(str))
	if err != nil {
		return ih, fmt.Errorf("invalid hex string")
	}
	if n != 20 {
		return ih, fmt.Errorf("invalid length")
	}
	return ih, nil
}
