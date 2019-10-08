package engine

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	eglog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

const (
	cacheSavedPrefix = "_CLDAUTOSAVED_"
)

//the Engine Cloud Torrent engine, backed by anacrolix/torrent
type Engine struct {
	mut       sync.Mutex
	cacheDir  string
	client    *torrent.Client
	config    Config
	ts        map[string]*Torrent
	bttracker []string
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
	tc.Debug = c.EngineDebug
	if e.config.MuteEngineLog {
		tc.Logger = eglog.Discard
	}
	tc.NoUpload = !c.EnableUpload
	tc.Seed = c.EnableSeeding
	tc.UploadRateLimiter = c.UploadLimiter()
	tc.DownloadRateLimiter = c.DownloadLimiter()
	tc.HeaderObfuscationPolicy = torrent.HeaderObfuscationPolicy{
		Preferred:        c.ObfsPreferred,
		RequirePreferred: c.ObfsRequirePreferred,
	}
	tc.DisableTrackers = c.DisableTrackers
	tc.DisableIPv6 = c.DisableIPv6
	tc.ProxyURL = c.ProxyURL

	client, err := torrent.NewClient(tc)
	if err != nil {
		return err
	}
	e.mut.Lock()
	e.cacheDir = c.WatchDirectory
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

func (e *Engine) NewFileTorrent(path string) error {
	info, err := metainfo.LoadFromFile(path)
	if err != nil {
		return err
	}
	spec := torrent.TorrentSpecFromMetaInfo(info)
	return e.NewTorrent(spec)
}

func (e *Engine) newTorrent(tt *torrent.Torrent) error {
	meta := tt.Metainfo()
	if len(e.bttracker) > 0 && (e.config.AlwaysAddTrackers || len(meta.AnnounceList) == 0) {
		log.Printf("[newTorrent] added %d public trackers\n", len(e.bttracker))
		tt.AddTrackers([][]string{e.bttracker})
	}
	t := e.upsertTorrent(tt)
	go func() {
		<-t.t.GotInfo()
		if e.config.AutoStart {
			e.StartTorrent(t.InfoHash)
			if w, err := os.Stat(e.cacheDir); err == nil && w.IsDir() {
				cacheFilePath := filepath.Join(e.cacheDir,
					fmt.Sprintf("%s%s.torrent", cacheSavedPrefix, t.InfoHash))
				// only create the cache file if not exists
				// avoid recreating a cache file during booting import
				if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
					cf, err := os.Create(cacheFilePath)
					if err == nil {
						umeta := t.t.Metainfo()
						umeta.Write(cf)
					} else {
						log.Println("[newTorrent] failed to create torrent file ", err)
					}
					cf.Close()
				}
			}
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
		env := append(os.Environ(),
			fmt.Sprintf("CLD_DIR=%s", e.config.DownloadDirectory),
			fmt.Sprintf("CLD_PATH=%s", t.Name),
			fmt.Sprintf("CLD_SIZE=%d", t.Size),
			"CLD_TYPE=torrent",
		)
		go e.callDoneCmd(env)
	}

	// call DoneCmd on each file completed
	for _, f := range t.Files {
		if f.Done && !f.DoneCmdCalled {
			f.DoneCmdCalled = true
			env := append(os.Environ(),
				fmt.Sprintf("CLD_DIR=%s", e.config.DownloadDirectory),
				fmt.Sprintf("CLD_PATH=%s", f.Path),
				fmt.Sprintf("CLD_SIZE=%d", f.Size),
				"CLD_TYPE=file",
			)
			go e.callDoneCmd(env)
		}
	}
}

func (e *Engine) upsertTorrent(tt *torrent.Torrent) *Torrent {
	ih := tt.InfoHash().HexString()
	torrent, ok := e.ts[ih]
	if !ok {
		torrent = &Torrent{
			InfoHash: ih,
			AddedAt:  time.Now(),
		}
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
	t.UploadRate = 0
	t.DownloadRate = 0
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

	cacheFilePath := filepath.Join(e.cacheDir,
		fmt.Sprintf("%s%s.torrent", cacheSavedPrefix, infohash))
	os.Remove(cacheFilePath)
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

func (e *Engine) callDoneCmd(env []string) {
	if e.config.DoneCmd == "" {
		return
	}
	cmd := exec.Command(e.config.DoneCmd)
	cmd.Env = env
	log.Printf("[DoneCmd] [%s] environ:%v", e.config.DoneCmd, cmd.Env)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("[DoneCmd] Err:", err)
	}
	log.Println("[DoneCmd] Output:", string(out))
}

func (e *Engine) UpdateTrackers() error {
	var txtlines []string
	url := e.config.TrackerListURL

	if !strings.HasPrefix(url, "https://") {
		err := fmt.Errorf("UpdateTrackers: trackers url invalid: %s (only https:// supported), extra trackers list now empty.", url)
		log.Print(err.Error())
		e.bttracker = txtlines
		return err
	}

	log.Printf("UpdateTrackers: loading trackers from %s\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		txtlines = append(txtlines, line)
	}

	e.bttracker = txtlines
	log.Printf("UpdateTrackers: loaded %d trackers \n", len(txtlines))
	return nil
}
