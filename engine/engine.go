package engine

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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
	tc := torrent.NewDefaultClientConfig()
	tc.ListenPort = c.IncomingPort
	tc.DataDir = c.DownloadDirectory
	tc.NoUpload = !c.EnableUpload
	tc.Seed = c.EnableSeeding
	tc.EncryptionPolicy = torrent.EncryptionPolicy {
		DisableEncryption: c.DisableEncryption,
	}
	tc.UploadRateLimiter = c.UploadLimiter()
	tc.DownloadRateLimiter = c.DownloadLimiter()

	client, err := torrent.NewClient(tc)
	if err != nil {
		return err
	}
	e.mut.Lock()
	e.config = c
	e.client = client
	e.mut.Unlock()
	//reset
	e.GetTorrents()
	return nil
}

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

func (e *Engine) newTorrent(tt *torrent.Torrent) error {
	t := e.upsertTorrent(tt)
	go func() {
		<-t.t.GotInfo()
		if e.config.AutoStart {
			e.StartTorrent(t.InfoHash)
		}
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
		t := e.upsertTorrent(tt)
		e.torrentRoutine(t)
	}
	return e.ts
}


func (e *Engine) torrentRoutine(t *Torrent) {

	// stops task on reaching ratio
	if e.config.SeedRatio > 0 &&
			t.SeedRatio > e.config.SeedRatio &&
			t.Started &&
			t.Done {
		log.Println("[Task Stoped] due to reaching SeedRatio")
		go e.StopTorrent(t.InfoHash)
	}

	// call DoneCmd on task completed
	if t.Done && !t.DoneCmdCalled {
		t.DoneCmdCalled = true
		go e.callDoneCmd(t)
	}
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
	ih := metainfo.NewHashFromHex(infohash)
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
	ih := metainfo.NewHashFromHex(infohash)
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
	f.f.Download()
	return nil
}

func (e *Engine) StopFile(infohash, filepath string) error {
	return fmt.Errorf("Unsupported")
}

func (e *Engine) callDoneCmd(torrent *Torrent) {
	if e.config.DoneCmd == "" {
		return
	}
	cmd := exec.Command(e.config.DoneCmd)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("CLD_DIR=%s", e.config.DownloadDirectory),
		fmt.Sprintf("CLD_PATH=%s", torrent.Name),
		fmt.Sprintf("CLD_SIZE=%d", torrent.Size),
		fmt.Sprintf("CLD_FILECNT=%d", len(torrent.Files)))
	log.Printf("[Task Completed] DoneCmd called: [%s] environ:%v", e.config.DoneCmd, cmd.Env)
	out, err := cmd.CombinedOutput()
	log.Printf("DoneCmd Output: `%s` err: `%s`", out, err)
}
