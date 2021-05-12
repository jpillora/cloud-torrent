package engine

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	eglog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

type Server interface {
	GetRestAPI() string
}

//the Engine Cloud Torrent engine, backed by anacrolix/torrent
type Engine struct {
	sync.RWMutex // race condition on ts,client
	cldServer    Server
	cacheDir     string
	client       *torrent.Client
	closeSync    chan struct{}
	config       Config
	ts           map[string]*Torrent
	bttracker    []string
	waitList     *syncList
}

func New(s Server) *Engine {
	return &Engine{ts: make(map[string]*Torrent), cldServer: s, waitList: NewSyncList()}
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

	e.Lock()
	defer e.Unlock()
	tc := torrent.NewDefaultClientConfig()
	tc.NoDefaultPortForwarding = c.NoDefaultPortForwarding
	tc.DisableUTP = c.DisableUTP
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
	log.Println("[NewMagnet] called: ", magnetURI)
	spec, err := torrent.TorrentSpecFromMagnetUri(magnetURI)
	if err != nil {
		return err
	}
	e.newMagnetCacheFile(magnetURI, spec.InfoHash.HexString())
	return e.newTorrentBySpec(spec, taskMagnet)
}

func (e *Engine) NewTorrentByReader(r io.Reader) error {
	info, err := metainfo.Load(r)
	if err != nil {
		return err
	}
	spec := torrent.TorrentSpecFromMetaInfo(info)
	e.newTorrentCacheFile(info)
	return e.newTorrentBySpec(spec, taskTorrent)
}

// NewTorrentByFilePath -> NewTorrentBySpec
func (e *Engine) NewTorrentByFilePath(path string) error {
	// torrent.TorrentSpecFromMetaInfo may panic if the info is malformed
	defer func() error {
		if r := recover(); r != nil {
			return fmt.Errorf("Error loading new torrent from file %s: %+v", path, r)
		}
		return nil
	}()

	info, err := metainfo.LoadFromFile(path)
	if err != nil {
		return err
	}
	e.newTorrentCacheFile(info)
	spec := torrent.TorrentSpecFromMetaInfo(info)
	return e.newTorrentBySpec(spec, taskTorrent)
}

// NewTorrentBySpec -> *Torrent -> addTorrentTask
func (e *Engine) newTorrentBySpec(spec *torrent.TorrentSpec, taskT taskType) error {
	ih := spec.InfoHash.HexString()
	log.Println("[newTorrentBySpec] called ", ih)
	if e.config.MaxConcurrentTask > 0 && len(e.client.Torrents()) >= e.config.MaxConcurrentTask {
		if !e.isTaskInList(ih) {
			log.Println("[newTorrentBySpec] reached max task, add as pretask: ", ih, taskT)
			e.waitList.Push(taskElem{ih: ih, tp: taskT})
		} else {
			log.Println("[newTorrentBySpec] reached max task, task already in tasks: ", ih, taskT)
		}
		return e.addPreTask(spec)
	}
	tt, _, err := e.client.AddTorrentSpec(spec)
	if err != nil {
		return err
	}
	return e.addTorrentTask(tt)
}

// addTorrentTask
// add the task to local cache object and wait for GotInfo
func (e *Engine) addTorrentTask(tt *torrent.Torrent) error {
	meta := tt.Metainfo()
	ih := meta.HashInfoBytes().HexString()
	if len(e.bttracker) > 0 && (e.config.AlwaysAddTrackers || len(meta.AnnounceList) == 0) {
		log.Printf("[newTorrent] added %d public trackers\n", len(e.bttracker))
		tt.AddTrackers([][]string{e.bttracker})
	}

	t := e.upsertTorrent(ih, tt.Name())
	t.Update(tt)
	go func() {
		select {
		case <-e.closeSync:
			return
		case <-t.dropWait:
			return
		case <-tt.GotInfo():
		}

		e.removeMagnetCache(ih)
		e.newTorrentCacheFile(&meta)
		if e.config.AutoStart {
			e.StartTorrent(ih)
		}
	}()

	return nil
}

// addPreTask
// add a task not ready to load
func (e *Engine) addPreTask(spec *torrent.TorrentSpec) error {
	e.upsertTorrent(spec.InfoHash.HexString(), spec.DisplayName)
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
		t, err := e.getTorrent(tt.InfoHash().HexString())
		if err != nil {
			log.Println("[TaskRoutine]", err)
			continue
		}

		t.Update(tt)

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
		t.t.AllowDataUpload()
		t.t.AllowDataDownload()

		// start all files by setting the priority to normal
		for _, f := range t.t.Files() {
			f.SetPriority(torrent.PiecePriorityNormal)
		}
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

	if t.t.Info() != nil {
		// stop all files by setting the priority to None
		for _, f := range t.t.Files() {
			f.SetPriority(torrent.PiecePriorityNone)
		}

		t.t.DisallowDataUpload()
		t.t.DisallowDataDownload()
	}

	t.Started = false
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
	if t.Loaded && !t.Deleted {
		close(t.dropWait)
		t.Deleted = true
		t.t.Drop()
		defer e.nextWaitTask()
	} else {
		// task not loaded, it's in the waiting list
		e.waitList.Remove(infohash)
	}
	delete(e.ts, t.InfoHash)
	e.Unlock()

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
