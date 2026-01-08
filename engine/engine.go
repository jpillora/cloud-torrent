package engine

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

// the Engine Cloud Torrent engine, backed by anacrolix/torrent
type Engine struct {
	mut      sync.Mutex
	cacheDir string
	client   *torrent.Client
	config   Config
	ts       map[string]*Torrent
	pts      map[string]*Torrent
}

func New() *Engine {
	return &Engine{ts: map[string]*Torrent{}, pts: map[string]*Torrent{}}
}

func (e *Engine) Config() Config {
	return e.config
}

func (e *Engine) Configure(c Config) error {
	//recieve config
	if e.client != nil {
		e.client.Close()
		time.Sleep(1 * time.Second)
	}
	if c.IncomingPort <= 0 {
		return fmt.Errorf("Invalid incoming port (%d)", c.IncomingPort)
	}

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
	//reset
	e.GetTorrents()
	e.GetPendingTorrents()
	return nil
}

func (e *Engine) NewMagnetNoStart(magnetURI string) error {
	tt, err := e.client.AddMagnet(magnetURI)
	if err != nil {
		return err
	}
	return e.newTorrentNoStart(tt)
}

// Torrents can either be added by magnet link or by torrent file, each one will call the newTorrent function
func (e *Engine) NewMagnet(magnetURI string) error {
	tt, err := e.client.AddMagnet(magnetURI)

	if err != nil {
		return err
	}
	return e.newTorrent(tt)
}

func (e *Engine) NewTorrent(spec *torrent.TorrentSpec) error {
	tt, _, err := e.client.AddTorrentSpec(spec)
	if err != nil {
		return err
	}
	return e.newTorrent(tt)
}

func (e *Engine) StartTorrentFromPending(infohash string) error {
	t, err := e.transferPendingTorrent(infohash)
	if err != nil {
		return err
	}
	go func() {
		<-t.t.GotInfo()
		e.StartTorrent(t.InfoHash)
	}()

	return nil
}

// private function to add a torrent to the engine
func (e *Engine) newTorrent(tt *torrent.Torrent) error {
	t := e.upsertTorrent(tt)
	go func() {
		<-t.t.GotInfo()
		e.StartTorrent(t.InfoHash)
	}()
	return nil
}

func (e *Engine) newTorrentNoStart(tt *torrent.Torrent) error {
	// Do we need to block here?
	e.upsertPendingTorrent(tt)
	return nil
}

// GetTorrents moves torrents out of the anacrolix/torrent
// and into the local cache
func (e *Engine) GetTorrents() map[string]*Torrent {
	e.mut.Lock()
	defer e.mut.Unlock()

	if e.client == nil {
		return nil
	}
	for _, tt := range e.client.Torrents() {
		// Check if the torrent is in the pending list
		if _, ok := e.pts[tt.InfoHash().HexString()]; !ok {
			e.upsertTorrent(tt)
		}
	}
	return e.ts
}

// Return torrents that are queued but not processed yet
func (e *Engine) GetPendingTorrents() map[string]*Torrent {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return nil
	}
	for _, tt := range e.client.Torrents() {
		if _, ok := e.pts[tt.InfoHash().HexString()]; ok {
			e.upsertPendingTorrent(tt)
		}
	}
	return e.pts
}

func (e *Engine) upsertPendingTorrent(tt *torrent.Torrent) *Torrent {
	ih := tt.InfoHash().HexString()
	torrent, ok := e.pts[ih]
	if !ok {
		torrent = &Torrent{InfoHash: ih}
		e.pts[ih] = torrent
	}

	torrent.Update(tt)
	return torrent
}

func (e *Engine) upsertTorrent(tt *torrent.Torrent) *Torrent {
	ih := tt.InfoHash().HexString()
	torrent, ok := e.ts[ih]
	if !ok {
		torrent = &Torrent{InfoHash: ih}
		e.ts[ih] = torrent
	}
	//update torrent fields using underlying torrent
	torrent.Update(tt)
	return torrent
}

// Moves the pending torrent into the active list
func (e *Engine) transferPendingTorrent(infohash string) (*Torrent, error) {
	e.mut.Lock()
	defer e.mut.Unlock()
	tt, ok := e.pts[infohash]
	if !ok {
		return nil, fmt.Errorf("Missing pending torrent %x", infohash)
	}
	delete(e.pts, infohash)
	e.ts[infohash] = tt
	return e.ts[infohash], nil
}

func (e *Engine) getTorrent(infohash string) (*Torrent, error) {
	ih, err := str2ih(infohash)
	if err != nil {
		return nil, err
	}
	t, ok := e.ts[ih.HexString()]
	if !ok {
		return t, fmt.Errorf("Missing torrent %x", ih)
	}
	return t, nil
}

func (e *Engine) getPendingTorrent(infohash string) (*Torrent, error) {
	ih, err := str2ih(infohash)
	if err != nil {
		return nil, err
	}
	t, ok := e.pts[ih.HexString()]
	if !ok {
		return nil, fmt.Errorf("Missing pending torrent %x", ih)
	}
	return t, nil
}

func (e *Engine) getOpenTorrent(infohash string) (*Torrent, error) {
	t, err := e.getTorrent(infohash)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (e *Engine) UpdateTorrentFilesToDownload(infohash string, filePositions []string) error {
	t, err := e.getPendingTorrent(infohash)
	if err != nil {
		return err
	}
	filePositionsInt := make([]int, len(filePositions))
	for i, filePosition := range filePositions {
		filePositionInt, err := strconv.Atoi(filePosition)
		if err != nil {
			return err
		}
		filePositionsInt[i] = filePositionInt
	}
	t.FilesToDownload = filePositionsInt
	return nil
}

// Note: StartTorrent can only be called on torrents that are in the open list
func (e *Engine) StartTorrent(infohash string) error {
	t, err := e.getOpenTorrent(infohash)
	if err != nil {
		return err
	}
	if t.Started {
		return fmt.Errorf("Already started")
	}
	t.Started = true

	// Condition to check if the torrent has specific files to download
	if len(t.FilesToDownload) == 0 {
		return fmt.Errorf("No files to download")
	}
	for _, filePosition := range t.FilesToDownload {
		t.Files[filePosition].Started = true
	}
	if t.t.Info() != nil {
		for _, filePosition := range t.FilesToDownload {
			t.t.Files()[filePosition].Download()
		}
	}
	return nil
}

func (e *Engine) StopTorrent(infohash string) error {
	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	if !t.Started {
		return fmt.Errorf("Already stopped")
	}
	//there is no stop - kill underlying torrent
	t.t.Drop()
	t.Started = false
	for _, f := range t.Files {
		if f != nil {
			f.Started = false
		}
	}
	return nil
}

func (e *Engine) DeletePendingTorrent(infohash string) error {
	t, err := e.getPendingTorrent(infohash)
	if err != nil {
		return err
	}
	delete(e.pts, t.InfoHash)
	ih, _ := str2ih(infohash)
	if tt, ok := e.client.Torrent(ih); ok {
		tt.Drop()
	}
	return nil
}

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

func (e *Engine) StartFile(infohash, filepath string) error {
	t, err := e.getOpenTorrent(infohash)
	if err != nil {
		return err
	}
	var f *File
	for _, file := range t.Files {
		if file.Path == filepath {
			f = file
			break
		}
	}
	if f == nil {
		return fmt.Errorf("Missing file %s", filepath)
	}
	if f.Started {
		return fmt.Errorf("Already started")
	}
	t.Started = true
	f.Started = true
	return nil
}

func (e *Engine) StopFile(infohash, filepath string) error {
	return fmt.Errorf("Unsupported")
}

func str2ih(str string) (metainfo.Hash, error) {
	var ih metainfo.Hash
	e, err := hex.Decode(ih[:], []byte(str))
	if err != nil {
		return ih, fmt.Errorf("Invalid hex string")
	}
	if e != 20 {
		return ih, fmt.Errorf("Invalid length")
	}
	return ih, nil
}
