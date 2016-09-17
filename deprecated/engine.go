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

//the Engine Cloud Torrent engine, backed by anacrolix/torrent
type Engine struct {
	mut      sync.Mutex
	cacheDir string
	client   *torrent.Client
	config   Config
	ts       map[string]*Torrent
}

func New() *Engine {
	return &Engine{ts: map[string]*Torrent{}}
}

func (e *Engine) Configure(c Config) error {
	//recieve and enforce engine config
	if e.client != nil {
		e.client.Close()
		time.Sleep(1 * time.Second)
	}
	if c.IncomingPort <= 0 {
		return fmt.Errorf("Invalid incoming port (%d)", c.IncomingPort)
	}
	tc := torrent.Config{
		DataDir:           c.DownloadDirectory,
		ListenAddr:        "0.0.0.0:" + strconv.Itoa(c.IncomingPort),
		NoUpload:          !c.EnableUpload,
		Seed:              c.EnableSeeding,
		DisableEncryption: !c.EnableEncryption,
	}
	client, err := torrent.NewClient(&tc)
	if err != nil {
		return err
	}
	e.mut.Lock()
	// e.cacheDir = filepath.Join(tc.ConfigDir, "torrents")
	// if files, err := ioutil.ReadDir(e.cacheDir); err == nil {
	// 	for _, f := range files {
	// 		if filepath.Ext(f.Name()) != ".torrent" {
	// 			continue
	// 		}
	// 		tt, err := client.AddTorrentFromFile(filepath.Join(e.cacheDir, f.Name()))
	// 		if err == nil {
	// 			e.upsertTorrent(tt)
	// 		}
	// 	}
	// }
	e.config = c
	e.client = client
	e.mut.Unlock()
	//reset
	e.GetTorrents()
	return nil
}

func (e *Engine) NewTorrent(magnetURI string) error {
	//adds the torrent but does not start it
	tt, err := e.client.AddMagnet(magnetURI)
	if err != nil {
		return err
	}
	t := e.upsertTorrent(tt)

	go func() {
		<-t.t.GotInfo()

		// if e.config.AutoStart && !loaded && torrent.Loaded && !torrent.Started {
		e.StartTorrent(t.InfoHash)
		// }

	}()

	return nil
}

//GetTorrents moves torrents out of the anacrolix/torrent
//and into the local cache
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
	torrent, ok := e.ts[ih]
	if !ok {
		torrent = &Torrent{InfoHash: ih}
		e.ts[ih] = torrent
	}
	//update torrent fields using underlying torrent
	torrent.Update(tt)
	return torrent
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

func (e *Engine) getOpenTorrent(infohash string) (*Torrent, error) {
	t, err := e.getTorrent(infohash)
	if err != nil {
		return nil, err
	}
	// if t.t == nil {
	// 	newt, err := e.client.AddTorrentFromFile(filepath.Join(e.cacheDir, infohash+".torrent"))
	// 	if err != nil {
	// 		return t, fmt.Errorf("Failed to open torrent %s", err)
	// 	}
	// 	t.t = &newt
	// }
	return t, nil
}

func (e *Engine) StartTorrent(infohash string) error {
	t, err := e.getOpenTorrent(infohash)
	if err != nil {
		return err
	}
	if t.Started {
		return fmt.Errorf("Already started")
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
	f.f.PrioritizeRegion(0, f.Size)
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
