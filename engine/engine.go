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
// infohash hex string.
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
	return e.config
}

// Configure (re)creates the underlying torrent client from c. An existing
// client is closed first.
func (e *Engine) Configure(c Config) error {
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
	e.mut.Lock()
	e.config = c
	e.client = client
	e.mut.Unlock()
	e.GetTorrents()
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
	tt, err := e.client.AddMagnet(magnetURI)
	if err != nil {
		return err
	}
	return e.newTorrent(tt)
}

// NewTorrent adds a torrent from a parsed .torrent spec.
func (e *Engine) NewTorrent(spec *torrent.TorrentSpec) error {
	tt, _, err := e.client.AddTorrentSpec(spec)
	if err != nil {
		return err
	}
	return e.newTorrent(tt)
}

func (e *Engine) newTorrent(tt *torrent.Torrent) error {
	t := e.upsertTorrent(tt)
	// once metadata arrives, auto-start if configured
	go func() {
		<-t.t.GotInfo()
		if e.config.AutoStart {
			e.StartTorrent(t.InfoHash)
		}
	}()
	return nil
}

// GetTorrents syncs the view-model cache with the client's torrents and returns
// the cache.
func (e *Engine) GetTorrents() map[string]*Torrent {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return nil
	}
	for _, tt := range e.client.Torrents() {
		e.upsertTorrent(tt)
	}
	return e.ts
}

func (e *Engine) upsertTorrent(tt *torrent.Torrent) *Torrent {
	ih := tt.InfoHash().HexString()
	t, ok := e.ts[ih]
	if !ok {
		t = &Torrent{InfoHash: ih}
		e.ts[ih] = t
	}
	t.Update(tt)
	return t
}

func (e *Engine) getTorrent(infohash string) (*Torrent, error) {
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
	t, err := e.getTorrent(infohash)
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
	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	if !t.Started {
		return fmt.Errorf("already stopped")
	}
	t.t.Drop()
	t.Started = false
	for _, f := range t.Files {
		if f != nil {
			f.Started = false
		}
	}
	return nil
}

// DeleteTorrent removes a torrent from the engine and drops it from the client.
func (e *Engine) DeleteTorrent(infohash string) error {
	t, err := e.getTorrent(infohash)
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
	t, err := e.getTorrent(infohash)
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
