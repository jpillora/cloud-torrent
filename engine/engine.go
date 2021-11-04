package engine

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"sync"
	"time"

	eglog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/boypt/simple-torrent/common"
	"github.com/fsnotify/fsnotify"
)

type Server interface {
	GetStrAttribute(name string) string
	GetBoolAttribute(name string) bool
}

const (
	CachedTorrentDir = ".cachedTorrents"
	TrashTorrentDir  = ".trashTorrents"
)

var (
	ErrTaskExists    = errors.New("Task already exists")
	ErrWaitListEmpty = errors.New("Wait list empty")
	ErrMaxConnTasks  = errors.New("Max conncurrent task reached")
)

//the Engine Cloud Torrent engine, backed by anacrolix/torrent
type Engine struct {
	sync.RWMutex // race condition on ts,client
	taskMutex    sync.Mutex
	cld          Server
	cacheDir     string
	trashDir     string
	client       *torrent.Client
	closeSync    chan struct{}
	config       Config
	ts           map[string]*Torrent
	TsChanged    chan struct{}
	Trackers     []string
	waitList     *syncList
	//file watcher
	watcher *fsnotify.Watcher
}

func New(s Server) *Engine {
	return &Engine{
		ts:        make(map[string]*Torrent),
		cld:       s,
		waitList:  NewSyncList(),
		TsChanged: make(chan struct{}, 1),
	}
}

func (e *Engine) Config() Config {
	return e.config
}

func (e *Engine) SetConfig(c *Config) {
	e.config = *c
}

func (e *Engine) Configure(c *Config) error {
	//recieve config
	if c.IncomingPort <= 0 {
		return fmt.Errorf("Invalid incoming port (%d)", c.IncomingPort)
	}
	if c.TrackerList == "" {
		c.TrackerList = "remote:" + defaultTrackerListURL
	}

	e.Lock()
	defer e.Unlock()
	tc := torrent.NewDefaultClientConfig()
	tc.NoDefaultPortForwarding = c.NoDefaultPortForwarding
	tc.DisableUTP = c.DisableUTP
	tc.ListenPort = c.IncomingPort
	tc.DataDir = c.DownloadDirectory

	if !(e.cld.GetBoolAttribute("DisableMmap")) {
		// enable MMap on 64bit machines
		if strconv.IntSize == 64 {
			log.Println("[Configure] 64bit arch detected, using MMap for storage")
			tc.DefaultStorage = storage.NewMMap(tc.DataDir)
		}
	} else {
		log.Println("[Configure] mmap disabled")
	}

	if c.MuteEngineLog {
		tc.Logger = eglog.Discard
	}
	tc.Debug = c.EngineDebug
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
	e.cacheDir = path.Join(c.DownloadDirectory, CachedTorrentDir)
	e.trashDir = path.Join(c.DownloadDirectory, TrashTorrentDir)
	mkdir(e.cacheDir)
	mkdir(e.trashDir)
	e.config = *c
	return nil
}

func (e *Engine) IsConfigred() bool {
	e.RLock()
	defer e.RUnlock()
	return e.client != nil
}

// NewMagnet -> newTorrentBySpec
func (e *Engine) NewMagnet(magnetURI string) error {
	log.Println("[NewMagnet] called:", magnetURI)
	spec, err := torrent.TorrentSpecFromMagnetUri(magnetURI)
	if err != nil {
		return err
	}
	e.newMagnetCacheFile(magnetURI, spec.InfoHash.HexString())
	return e.newTorrentBySpec(spec, taskMagnet)
}

// NewTorrentByReader -> newTorrentBySpec
func (e *Engine) NewTorrentByReader(r io.Reader) error {
	info, err := metainfo.Load(r)
	if err != nil {
		return err
	}
	spec := torrent.TorrentSpecFromMetaInfo(info)
	e.newTorrentCacheFile(info)
	return e.newTorrentBySpec(spec, taskTorrent)
}

// NewTorrentByFilePath -> newTorrentBySpec
func (e *Engine) NewTorrentByFilePath(path string) error {
	// torrent.TorrentSpecFromMetaInfo may panic if the info is malformed
	defer func() error {
		if r := recover(); r != nil {
			err := fmt.Errorf("Error loading new torrent from file %s: %+v", path, r)
			log.Println(err)
			return err
		}
		return nil
	}() // nolint: errcheck

	info, err := metainfo.LoadFromFile(path)
	if err != nil {
		return err
	}
	e.newTorrentCacheFile(info)
	spec := torrent.TorrentSpecFromMetaInfo(info)
	return e.newTorrentBySpec(spec, taskTorrent)
}

func (e *Engine) isReadyAddTask() bool {
	nowTorrentsLen := len(e.client.Torrents())
	if e.config.MaxConcurrentTask > 0 && nowTorrentsLen >= e.config.MaxConcurrentTask {
		return false
	}
	return true
}

// NewTorrentBySpec -> *Torrent -> addTorrentTask
func (e *Engine) newTorrentBySpec(spec *torrent.TorrentSpec, taskT taskType) error {
	ih := spec.InfoHash.HexString()
	log.Println("[newTorrentBySpec] called", ih)

	e.taskMutex.Lock()
	defer e.taskMutex.Unlock()
	// whether add as pretasks
	if !e.isReadyAddTask() {
		if !e.isTaskInList(ih) {
			log.Printf("[newTorrentBySpec] reached max task %d, add as pretask: %s %v", e.config.MaxConcurrentTask, ih, taskT)
			e.pushWaitTask(ih, taskT)
		} else {
			log.Printf("[newTorrentBySpec] reached max task %d, task already in queue: %s %v", e.config.MaxConcurrentTask, ih, taskT)
		}
		_, err := e.upsertTorrent(ih, spec.DisplayName, true) // show queueing task
		common.FancyHandleError(err)
		return ErrMaxConnTasks
	}

	t, _ := e.upsertTorrent(ih, spec.DisplayName, false)
	tt, _, err := e.client.AddTorrentSpec(spec)
	if err != nil {
		return err
	}

	meta := tt.Metainfo()
	if len(e.Trackers) > 0 && (e.config.AlwaysAddTrackers || len(meta.AnnounceList) == 0) {
		log.Printf("[newTorrent] added %d public trackers\n", len(e.Trackers))
		tt.AddTrackers([][]string{e.Trackers})
	}

	go e.torrentEventProcessor(tt, t, ih)
	return nil
}

func (e *Engine) torrentEventProcessor(tt *torrent.Torrent, t *Torrent, ih string) {

	select {
	case <-e.closeSync:
		log.Println("Engine shutdown while waiting Info", ih)
		tt.Drop()
		return
	case <-t.dropWait:
		tt.Drop()
		log.Println("Task Dropped while waiting Info", ih)
		go e.NextWaitTask() // nolint: errcheck
		return
	case <-tt.GotInfo():
		// Already got full torrent info
		// If the origin is from a magnet link, remove it, cache the torrent data
		e.removeMagnetCache(ih)
		m := tt.Metainfo()
		e.newTorrentCacheFile(&m)
		t.updateOnGotInfo(tt)
		e.TsChanged <- struct{}{}
	}

	if e.config.AutoStart {
		go e.StartTorrent(ih) // nolint: errcheck
	}

	timeTk := time.NewTicker(3 * time.Second)
	defer timeTk.Stop()

	// main loop updating the torrent status to our struct
	for {
		select {
		case <-timeTk.C:
			if !t.IsAllFilesDone {
				t.updateFileStatus()
			}
			if !t.Done {
				t.updateTorrentStatus()
			}
			if t.Started {
				e.taskRoutine(t)
			}
			t.updateConnStat()
		case <-t.dropWait:
			tt.Drop()
			log.Println("Task Droped, exit loop:", ih)
			go e.NextWaitTask() // nolint: errcheck
			return
		case <-e.closeSync:
			log.Println("Engine shutdown while downloading", ih)
			tt.Drop()
			return
		}
	}
}

//GetTorrents just get the local infohash->Torrent map
func (e *Engine) GetTorrents() *map[string]*Torrent {
	return &e.ts
}

// TaskRoutine
func (e *Engine) taskRoutine(t *Torrent) {

	// stops task on reaching ratio
	if e.config.SeedRatio > 0 && t.SeedRatio > e.config.SeedRatio &&
		t.Started && !t.ManualStarted && t.Done {
		log.Printf("[TaskRoutine]%s Stopped and Drop due to reaching SeedRatio %f", t.InfoHash, t.SeedRatio)
		go e.stopRemoveTask(t.InfoHash)
	}

	// stops task when there're tasks waiting after `SeedTime`
	if e.config.SeedTime > 0 && e.waitList.Len() > 0 &&
		t.Done && t.Started && !t.ManualStarted &&
		!t.FinishedAt.IsZero() &&
		time.Since(t.FinishedAt) > e.config.SeedTime {
		log.Printf("[TaskRoutine]%s Stopped and Drop due to timed up for SeedTime %s", t.InfoHash, e.config.SeedTime)
		go e.stopRemoveTask(t.InfoHash)
	}
}

func (e *Engine) stopRemoveTask(ih string) {
	common.FancyHandleError(e.StopTorrent(ih))
	e.RemoveCache(ih)
	common.FancyHandleError(e.DeleteTorrent(ih))
}

func (e *Engine) ManualStartTorrent(infohash string) error {
	if err := e.StartTorrent(infohash); err == nil {
		t, _ := e.getTorrent(infohash)
		t.Lock()
		defer t.Unlock()
		t.ManualStarted = true
	} else {
		return err
	}
	return nil
}

func (e *Engine) StartTorrent(infohash string) error {
	log.Println("StartTorrent", infohash)
	e.Lock()
	defer e.Unlock()

	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	t.Lock()
	defer t.Unlock()

	if t.Started {
		return fmt.Errorf("already started")
	}
	t.Started = true
	t.StartedAt = time.Now()
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
	log.Println("StopTorrent", infohash)
	e.Lock()
	defer e.Unlock()
	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	t.Lock()
	defer t.Unlock()

	if !t.Started {
		return fmt.Errorf("already stopped")
	}

	if t.t.Info() != nil {
		t.t.CancelPieces(0, t.t.NumPieces())
	}

	t.Started = false
	t.StoppedAt = time.Now()
	for _, f := range t.Files {
		f.Started = false
	}

	return nil
}

func (e *Engine) DeleteTorrent(infohash string) error {
	log.Println("DeleteTorrent", infohash)
	e.Lock()
	defer e.Unlock()

	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	close(t.dropWait)
	e.waitList.Remove(infohash)
	e.deleteTorrent(infohash)
	return nil
}

func (e *Engine) StartFile(infohash, filepath string) error {
	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	t.Lock()
	defer t.Unlock()
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
	if !t.Started {
		t.Started = true
	}
	f.Started = true
	f.f.SetPriority(torrent.PiecePriorityNormal)
	return nil
}

func (e *Engine) StopFile(infohash, filepath string) error {
	t, err := e.getTorrent(infohash)
	if err != nil {
		return err
	}
	t.Lock()
	defer t.Unlock()
	var f *File
	for _, file := range t.Files {
		if file.Path == filepath {
			f = file
			break
		}
	}
	if f == nil {
		return fmt.Errorf("missing file %s", filepath)
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
		t.Started = false
		t.StoppedAt = time.Now()
	}

	return nil
}

func (e *Engine) RemoveCache(infohash string) {
	e.removeMagnetCache(infohash)
	e.removeTorrentCache(infohash, true)
}
