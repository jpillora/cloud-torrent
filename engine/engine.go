package engine

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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

type Server interface {
	GetRestAPI() string
	GetIsPendingBoot() bool
}

//the Engine Cloud Torrent engine, backed by anacrolix/torrent
type Engine struct {
	sync.RWMutex  // race condition on ts,client
	cldServer     Server
	cacheDir      string
	client        *torrent.Client
	closeSync     chan struct{}
	config        Config
	ts            map[string]*Torrent
	bttracker     []string
	doneThreshold time.Duration
}

func New(s Server) *Engine {
	return &Engine{ts: make(map[string]*Torrent), cldServer: s}
}

func (e *Engine) Config() Config {
	return e.config
}

func (e *Engine) SetConfig(c Config) {
	e.config = c
}

func (e *Engine) Configure(c *Config) error {
	//recieve config
	if c.IncomingPort <= 0 {
		return fmt.Errorf("Invalid incoming port (%d)", c.IncomingPort)
	}
	if c.ScraperURL == "" {
		c.ScraperURL = defaultScraperURL
	}
	if c.TrackerListURL == "" {
		c.TrackerListURL = defaultTrackerListURL
	}

	if th, err := time.ParseDuration(c.DoneCmdThreshold); err == nil {
		e.doneThreshold = th
	} else {
		return fmt.Errorf("fail to parse DoneCmdThreshold %w", err)
	}

	e.Lock()
	defer e.Unlock()
	tc := torrent.NewDefaultClientConfig()
	tc.ListenPort = c.IncomingPort
	tc.DataDir = c.DownloadDirectory
	tc.Debug = c.EngineDebug
	if c.MuteEngineLog {
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
	if c.ProxyURL != "" {
		tc.HTTPProxy = func(*http.Request) (*url.URL, error) {
			return url.Parse(c.ProxyURL)
		}
	}

	{
		if e.client != nil {
			// stop all current torrents
			for _, t := range e.client.Torrents() {
				t.Drop()
			}
			e.client.Close()
			close(e.closeSync)
			log.Println("Configure: old client closed")
			e.client = nil
			e.ts = make(map[string]*Torrent)
			time.Sleep(3 * time.Second)
		}

		// runtime reconfigure need to retry while creating client,
		// wait max for 3 * 10 seconds
		var err error
		max := 10
		for max > 0 {
			max--
			e.client, err = torrent.NewClient(tc)
			if err == nil {
				break
			}
			log.Printf("[Configure] error %s\n", err)
			time.Sleep(time.Second * 3)
		}
		if err != nil {
			return err
		}
	}

	e.closeSync = make(chan struct{})
	e.cacheDir = c.WatchDirectory
	e.config = *c
	return nil
}

func (e *Engine) IsConfigred() bool {
	e.RLock()
	defer e.RUnlock()
	return e.client != nil
}

// NewMagnet -> *Torrent -> addTorrentTask
func (e *Engine) NewMagnet(magnetURI string) error {
	e.RLock()
	tt, err := e.client.AddMagnet(magnetURI)
	if err != nil {
		return err
	}
	e.RUnlock()
	e.newMagnetCacheFile(magnetURI, tt.InfoHash().HexString())
	return e.addTorrentTask(tt)
}

// NewTorrentBySpec -> *Torrent -> addTorrentTask
func (e *Engine) NewTorrentBySpec(spec *torrent.TorrentSpec) error {
	e.RLock()
	tt, _, err := e.client.AddTorrentSpec(spec)
	if err != nil {
		return err
	}
	e.RUnlock()
	return e.addTorrentTask(tt)
}

// NewTorrentByFilePath -> NewTorrentBySpec
func (e *Engine) NewTorrentByFilePath(path string) error {
	info, err := metainfo.LoadFromFile(path)
	if err != nil {
		return err
	}
	spec := torrent.TorrentSpecFromMetaInfo(info)
	return e.NewTorrentBySpec(spec)
}

// addTorrentTask
// add the task to local cache object and wait for GotInfo
func (e *Engine) addTorrentTask(tt *torrent.Torrent) error {
	meta := tt.Metainfo()
	if len(e.bttracker) > 0 && (e.config.AlwaysAddTrackers || len(meta.AnnounceList) == 0) {
		log.Printf("[newTorrent] added %d public trackers\n", len(e.bttracker))
		tt.AddTrackers([][]string{e.bttracker})
	}
	t := e.upsertTorrent(tt)
	go func() {
		select {
		case <-e.closeSync:
			return
		case <-t.dropWait:
			return
		case <-t.t.GotInfo():
		}

		e.removeMagnetCache(t.InfoHash)
		if e.config.AutoStart {
			e.StartTorrent(t.InfoHash)
		}
		e.newTorrentCacheFile(t.InfoHash, t.t.Metainfo())
	}()

	return nil
}

//GetTorrents just get the local infohash->Torrent map
func (e *Engine) GetTorrents() map[string]*Torrent {
	return e.ts
}

// TaskRoutine called by intevaled background goroutine
// moves torrents out of the anacrolix/torrent and into the local cache
// and do condition check and actions on them
func (e *Engine) TaskRoutine() {

	if e.client == nil {
		return
	}

	for _, tt := range e.client.Torrents() {

		// sync engine.Torrent to our Torrent struct
		t := e.upsertTorrent(tt)

		// stops task on reaching ratio
		if e.config.SeedRatio > 0 &&
			t.SeedRatio > e.config.SeedRatio &&
			t.Started &&
			t.Done {
			log.Println("[Task Stoped] due to reaching SeedRatio")
			go e.StopTorrent(t.InfoHash)
		}

		// task started in `DoneCmdThreshold` (default 30s), won't call the DoneCmd below
		if !t.IsDoneReady {
			if t.StartedAt.IsZero() {
				continue
			}

			if time.Since(t.StartedAt) > e.doneThreshold && !t.Done {
				// after 30s and task is not done yet, is ready for DoneCmd
				t.IsDoneReady = true
			}

			continue
		}

		// call DoneCmd on task completed
		if t.Done && !t.DoneCmdCalled {
			t.DoneCmdCalled = true
			go e.callDoneCmd(genEnv(e.config.DownloadDirectory,
				t.Name, t.InfoHash, "torrent",
				e.cldServer.GetRestAPI(), t.Size, t.StartedAt.Unix()))
		}

		// call DoneCmd on each file completed
		// some files might finish before the whole task does
		for _, f := range t.Files {
			if f.Done && !f.DoneCmdCalled {
				f.DoneCmdCalled = true
				go e.callDoneCmd(genEnv(e.config.DownloadDirectory,
					f.Path, "", "file",
					e.cldServer.GetRestAPI(), f.Size, t.StartedAt.Unix()))
			}
		}
	}
}

func genEnv(dir, path, hash, ttype, api string, size int64, ts int64) []string {
	env := append(os.Environ(),
		fmt.Sprintf("CLD_DIR=%s", dir),
		fmt.Sprintf("CLD_PATH=%s", path),
		fmt.Sprintf("CLD_HASH=%s", hash),
		fmt.Sprintf("CLD_TYPE=%s", ttype),
		fmt.Sprintf("CLD_RESTAPI=%s", api),
		fmt.Sprintf("CLD_SIZE=%d", size),
		fmt.Sprintf("CLD_STARTTS=%d", ts),
	)
	return env
}

func (e *Engine) upsertTorrent(tt *torrent.Torrent) *Torrent {
	ih := tt.InfoHash().HexString()
	e.RLock()
	torrent, ok := e.ts[ih]
	e.RUnlock()
	if !ok {
		torrent = &Torrent{
			InfoHash: ih,
			AddedAt:  time.Now(),
			dropWait: make(chan struct{}),
		}
		e.Lock()
		e.ts[ih] = torrent
		e.Unlock()
	}
	//update torrent fields using underlying torrent
	torrent.Update(tt)
	return torrent
}

func (e *Engine) getTorrent(infohash string) (*Torrent, error) {
	e.RLock()
	defer e.RUnlock()
	ih := metainfo.NewHashFromHex(infohash)
	t, ok := e.ts[ih.HexString()]
	if !ok {
		return t, fmt.Errorf("Missing torrent %x", ih)
	}
	return t, nil
}

func (e *Engine) StartTorrent(infohash string) error {
	log.Println("StartTorrent ", infohash)
	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	if t.Started {
		return fmt.Errorf("Already started")
	}
	t.Started = true
	t.StartedAt = time.Now()
	for _, f := range t.Files {
		if f != nil {
			f.Started = true
		}
	}
	if t.t.Info() != nil {
		// start file by setting the priority
		for _, f := range t.t.Files() {
			f.SetPriority(torrent.PiecePriorityNormal)
		}

		// call to DownloadAll cause StartFile/StopFile not working
		// t.t.DownloadAll()
	}
	return nil
}

func (e *Engine) StopTorrent(infohash string) error {
	log.Println("StopTorrent ", infohash)
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
	log.Println("DeleteTorrent ", infohash)
	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}

	e.Lock()
	close(t.dropWait)
	delete(e.ts, t.InfoHash)
	e.Unlock()

	ih := metainfo.NewHashFromHex(infohash)
	if tt, ok := e.client.Torrent(ih); ok {
		tt.Drop()
	}

	e.removeMagnetCache(infohash)
	e.removeTorrentCache(infohash)
	return nil
}

func (e *Engine) StartFile(infohash, filepath string) error {
	t, err := e.getTorrent(infohash)
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
		return fmt.Errorf("already started")
	}
	t.Started = true
	f.Started = true
	f.f.SetPriority(torrent.PiecePriorityNormal)
	return nil
}

func (e *Engine) StopFile(infohash, filepath string) error {
	t, err := e.getTorrent(infohash)
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
	if !f.Started {
		return fmt.Errorf("already stopped")
	}
	f.Started = false
	f.f.SetPriority(torrent.PiecePriorityNone)

	allStopped := true
	for _, file := range t.Files {
		if file.Started {
			allStopped = false
			break
		}
	}

	if allStopped {
		e.StopTorrent(infohash)
	}

	return nil
}

func (e *Engine) callDoneCmd(env []string) {
	if e.config.DoneCmd == "" {
		return
	}

	if e.cldServer.GetIsPendingBoot() {
		log.Println("[DoneCmd] program is pending boot, skiping")
		return
	}

	cmd := exec.Command(e.config.DoneCmd)
	cmd.Env = env
	log.Printf("[DoneCmd] [%s] environ:%v", e.config.DoneCmd, cmd.Env)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("[DoneCmd] Err:", err)
		return
	}
	log.Println("[DoneCmd] Exit:", cmd.ProcessState.ExitCode(), "Output:", string(out))
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

func (e *Engine) newMagnetCacheFile(magnetURI, infohash string) {
	// create .info file with hash as filename
	if w, err := os.Stat(e.cacheDir); err == nil && w.IsDir() {
		cacheInfoPath := filepath.Join(e.cacheDir,
			fmt.Sprintf("%s%s.info", cacheSavedPrefix, infohash))
		if _, err := os.Stat(cacheInfoPath); os.IsNotExist(err) {
			cf, err := os.Create(cacheInfoPath)
			defer cf.Close()
			if err == nil {
				cf.WriteString(magnetURI)
				log.Println("created magnet cache info file", infohash)
			}
		}
	}
}

func (e *Engine) newTorrentCacheFile(infohash string, meta metainfo.MetaInfo) {
	// create .torrent file
	if w, err := os.Stat(e.cacheDir); err == nil && w.IsDir() {
		cacheFilePath := filepath.Join(e.cacheDir,
			fmt.Sprintf("%s%s.torrent", cacheSavedPrefix, infohash))
		// only create the cache file if not exists
		// avoid recreating cache files during boot import
		if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
			cf, err := os.Create(cacheFilePath)
			defer cf.Close()
			if err == nil {
				meta.Write(cf)
				log.Println("created torrent cache file", infohash)
			} else {
				log.Println("failed to create torrent file ", err)
			}
		}
	}
}

func (e *Engine) removeMagnetCache(infohash string) {
	// remove both magnet and torrent cache if exists.
	cacheInfoPath := filepath.Join(e.cacheDir,
		fmt.Sprintf("%s%s.info", cacheSavedPrefix, infohash))
	if err := os.Remove(cacheInfoPath); err == nil {
		log.Printf("removed magnet info file %s", infohash)
	}
}

func (e *Engine) removeTorrentCache(infohash string) {
	cacheFilePath := filepath.Join(e.cacheDir,
		fmt.Sprintf("%s%s.torrent", cacheSavedPrefix, infohash))
	if err := os.Remove(cacheFilePath); err == nil {
		log.Printf("removed torrent file %s", infohash)
	} else {
		log.Printf("fail to removed torrent file %s, %s", infohash, err)
	}
}

func (e *Engine) WriteStauts(_w io.Writer) {
	e.RLock()
	defer e.RUnlock()
	if e.client != nil {
		e.client.WriteStatus(_w)
	}
}
