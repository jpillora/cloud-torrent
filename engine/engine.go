package engine

import (
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/jpillora/cloud-torrent/storage"
)

//the Engine Cloud Torrent engine, backed by anacrolix/torrent
type Engine struct {
	//public torrents
	Torrents map[string]*Torrent
	//internal
	mut         sync.Mutex
	cacheDir    string
	configuring bool
	client      *torrent.Client
	ts          map[torrent.InfoHash]*Torrent
	lastConfig  Config
}

func New(storage *storage.Storage) *Engine {
	return &Engine{
		Torrents: map[string]*Torrent{},
		ts:       map[torrent.InfoHash]*Torrent{},
	}
}

func (e *Engine) Configure(c *Config) error {
	//ensure locks
	e.mut.Lock()
	defer func() {
		e.mut.Unlock()
		e.Update()
	}()
	if e.configuring {
		return fmt.Errorf("Configuration in progress")
	}
	//configuring...
	defer func() {
		e.configuring = false
	}()
	e.configuring = true
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
		//wait until disconnected
		conn, err := net.Dial("tcp", "0.0.0.0:"+strconv.Itoa(e.lastConfig.IncomingPort))
		if err == nil {
			b := make([]byte, 0xff)
			for {
				if _, err := conn.Read(b); err != nil {
					break
				}
			}
		}
	}
	tc := torrent.Config{
		DataDir:           c.DownloadDirectory,
		ConfigDir:         filepath.Join(c.DownloadDirectory, ".config"),
		ListenAddr:        "0.0.0.0:" + strconv.Itoa(c.IncomingPort),
		NoUpload:          !c.EnableUpload,
		Seed:              c.EnableSeeding,
		DisableEncryption: !c.EnableEncryption,
		TorrentDataOpener: e.OpenTorrent,
	}
	client, err := torrent.NewClient(&tc)
	if err != nil {
		return err
	}
	e.lastConfig = *c
	e.client = client
	e.cacheDir = filepath.Join(tc.ConfigDir, "torrents")
	log.Printf("torrent cache loading...")
	if files, err := ioutil.ReadDir(e.cacheDir); err == nil {
		for _, f := range files {
			if filepath.Ext(f.Name()) != ".torrent" {
				continue
			}
			file, err := os.Open(filepath.Join(e.cacheDir, f.Name()))
			if err != nil {
				return err
			}
			e.NewByFile(file)
		}
	}
	log.Printf("torrent cache loaded")
	return nil
}

//OpenTorrent implements the torrent.Openner interface
//and Torrent implements the torrent.Data interface
func (e *Engine) OpenTorrent(info *metainfo.Info) torrent.Data {

	var ih torrent.InfoHash
	//calculate infohash
	b, err := bencode.Marshal(info)
	if err != nil {
		panic("invalid bencode")
	}
	ihx := metainfo.InfoEx{}
	ihx.UnmarshalBencode(b)

	copy(ih[:], ihx.Bytes)

	//load by infohash (cant error - valid ih and upserting)
	t, ok := e.ts[ih]
	if !ok {
		t = e.newTorrent(ih)
		t.init(info)
	}
	log.Printf("open torrent %s (%x)", info.Name, ihx.Hash)
	//provide the torrent as its own "openner"
	return t
}

//GetTorrents copies torrents out of anacrolix/torrent
//and into the local cache
func (e *Engine) Update() {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return
	}
	for _, tt := range e.client.Torrents() {
		ih := tt.InfoHash()
		t, ok := e.ts[ih]
		if !ok {
			t = e.newTorrent(ih)
		}
		t.Update(tt)
	}
}

func (e *Engine) newTorrent(ih torrent.InfoHash) *Torrent {
	t := &Torrent{}
	e.ts[ih] = t
	e.Torrents[ih.HexString()] = t
	return t
}

func (e *Engine) Get(hex string) (*Torrent, bool) {
	e.mut.Lock()
	defer e.mut.Unlock()
	ih, err := validateInfohash(hex)
	if err != nil {
		return nil, false
	}
	t, ok := e.ts[ih]
	return t, ok
}

func (e *Engine) NewByMagnet(magnetURI string) error {
	_, err := e.client.AddMagnet(magnetURI)
	if err != nil {
		return err
	}
	return nil
}

func (e *Engine) NewByFile(body io.Reader) error {
	info, err := metainfo.Load(body)
	if err != nil {
		return err
	}
	log.Printf("new file HASH: %x", info.Info.Hash)
	_, err = e.client.AddTorrent(info)
	if err != nil {
		return err
	}
	return nil
}

func (e *Engine) Remove(rmt *Torrent) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	id := rmt.id
	t, ok := e.ts[id]
	if !ok {
		return fmt.Errorf("Missing")
	}
	for _, f := range t.Files {
		f.Stop()
	}
	t.tt.Drop()
	delete(e.ts, id)
	delete(e.Torrents, id.HexString())
	return nil
}

func validateInfohash(str string) (torrent.InfoHash, error) {
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
