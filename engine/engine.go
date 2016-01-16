package engine

import (
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/jpillora/cloud-torrent/storage"
)

//the Engine Cloud Torrent engine, backed by anacrolix/torrent
type Engine struct {
	mut      sync.Mutex
	cacheDir string
	client   *torrent.Client
	ts       map[string]*Torrent
	storage  *storage.Storage
}

func New(storage *storage.Storage) *Engine {
	return &Engine{
		ts:      map[string]*Torrent{},
		storage: storage,
	}
}

func (e *Engine) Configure(c *Config) error {
	if c.IncomingPort <= 0 || c.IncomingPort >= 65535 {
		c.IncomingPort = 50007
	}
	if dir, err := filepath.Abs(c.DownloadDirectory); err != nil {
		return fmt.Errorf("Invalid path")
	} else {
		c.DownloadDirectory = dir
	}
	//recieve config
	if e.client != nil {
		e.client.Close()
		time.Sleep(1 * time.Second)
	}
	tc := torrent.Config{
		DataDir:           c.DownloadDirectory,
		ConfigDir:         filepath.Join(c.DownloadDirectory, ".config"),
		ListenAddr:        "0.0.0.0:" + strconv.Itoa(c.IncomingPort),
		NoUpload:          !c.EnableUpload,
		Seed:              c.EnableSeeding,
		DisableEncryption: !c.EnableEncryption,
		TorrentDataOpener: e.CreateStream,
	}
	client, err := torrent.NewClient(&tc)
	if err != nil {
		return err
	}
	e.mut.Lock()
	e.cacheDir = filepath.Join(tc.ConfigDir, "torrents")
	if files, err := ioutil.ReadDir(e.cacheDir); err == nil {
		for _, f := range files {
			if filepath.Ext(f.Name()) != ".torrent" {
				continue
			}
			tt, err := client.AddTorrentFromFile(filepath.Join(e.cacheDir, f.Name()))
			if err == nil {
				e.upsertTorrent(tt)
			}
		}
	}
	e.client = client
	e.mut.Unlock()
	//reset
	e.GetTorrents()
	return nil
}

func (e *Engine) CreateStream(info *metainfo.Info) torrent.Data {
	log.Printf("stream torrent: %s", info.Name)
	return nil
}

func (e *Engine) StartMagnet(magnetURI string) error {
	if tt, err := e.client.AddMagnet(magnetURI); err != nil {
		return err
	} else {
		e.upsertTorrent(tt)
		return nil
	}
}

func (e *Engine) StartTorrentFile(body io.Reader) error {
	info, err := metainfo.Load(body)
	if err != nil {
		return err
	}
	if tt, err := e.client.AddTorrent(info); err != nil {
		return err
	} else {
		e.upsertTorrent(tt)
		return nil
	}
}

//GetTorrents copies torrents out of anacrolix/torrent
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

func (e *Engine) upsertTorrent(tt torrent.Torrent) *Torrent {
	ih := tt.InfoHash().HexString()
	torrent, ok := e.ts[ih]
	if !ok {
		torrent = &Torrent{InfoHash: ih}
		e.ts[ih] = torrent
	}
	//update torrent fields using underlying torrent
	torrent.Update(tt)
	// go func() {
	// 	if tt.Info() != nil {
	// 		return
	// 	}
	// 	<-tt.GotInfo()
	// 	e.StartTorrent(tt.InfoHash)
	// }()
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

func (e *Engine) getFile(infohash, filepath string) (*File, error) {
	t, err := e.getOpenTorrent(infohash)
	if err != nil {
		return nil, err
	}
	var f *File
	for _, file := range t.Files {
		if file.Path == filepath {
			f = file
			break
		}
	}
	if f == nil {
		return nil, fmt.Errorf("Missing file %s", filepath)
	}
	return f, nil
}

func (e *Engine) StartFile(infohash, filepath string) error {
	f, err := e.getFile(infohash, filepath)
	if err != nil {
		return err
	}
	if f.Started {
		return fmt.Errorf("Already started")
	}
	f.Started = true
	f.f.PrioritizeRegionTo(0, f.Size, torrent.PiecePriorityNormal)
	return nil
}

func (e *Engine) StopFile(infohash, filepath string) error {
	f, err := e.getFile(infohash, filepath)
	if err != nil {
		return err
	}
	if !f.Started {
		return fmt.Errorf("Already stopped")
	}
	f.Started = false
	f.f.PrioritizeRegionTo(0, f.Size, torrent.PiecePriorityNone)
	return nil
}

func str2ih(str string) (torrent.InfoHash, error) {
	var ih torrent.InfoHash
	e, err := hex.Decode(ih[:], []byte(str))
	if err != nil {
		return ih, fmt.Errorf("Invalid hex string")
	}
	if e != 20 {
		return ih, fmt.Errorf("Invalid length")
	}
	return ih, nil
}
