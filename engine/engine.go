package engine

import (
	"encoding/hex"
	"errors"
	"fmt"
	alog "github.com/anacrolix/log"
	"io"
	"log"
	"log/slog"
	"maps"
	"net"
	"net/url"
	"os"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/gofrs/flock"
	"github.com/jpillora/cloud-torrent/internal/outbound"
)

// Engine wraps anacrolix/torrent and maintains a cache of view-models keyed by
// infohash hex string. mut owns the client, configuration and all cached models.
// Public methods lock it; helpers suffixed Locked require the caller to hold it.
type Engine struct {
	outbound        *outbound.Policy
	mut             sync.Mutex
	newClient       func(*torrent.ClientConfig) (*torrent.Client, error)
	client          *torrent.Client
	config          Config
	ts              map[string]*Torrent
	sessionDir      string
	sessionLock     *flock.Flock
	pendingSessions []savedTorrent
}

// New returns an unconfigured engine. Call Configure before adding torrents.
func New(policies ...*outbound.Policy) *Engine {
	policy, _ := outbound.New("")
	if len(policies) > 0 && policies[0] != nil {
		policy = policies[0]
	}
	return &Engine{outbound: policy, ts: map[string]*Torrent{}, newClient: torrent.NewClient}
}

// Config returns the engine's current configuration.
func (e *Engine) Config() Config {
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.config
}

// Configure applies AutoStart in place. Client-level changes recreate the
// client and rebind every torrent; a failed replacement restores the old setup.
func (e *Engine) Configure(c Config) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	c.IncomingPort = defaultPort(c.IncomingPort)
	old := e.config
	if e.client != nil {
		sameClient := old
		sameClient.AutoStart = c.AutoStart
		if sameClient == c {
			e.config = c
			return nil
		}
	}
	if err := validateDownloadDirectory(c.DownloadDirectory); err != nil {
		return err
	}
	// Catch the common rejected-port case without interrupting the old client.
	if e.client != nil && c.IncomingPort != old.IncomingPort {
		if err := checkListenPort(c.IncomingPort); err != nil {
			return err
		}
	}
	saved := e.saveTorrentsLocked()
	if e.client == nil && e.pendingSessions != nil {
		saved = e.pendingSessions
	}
	hadClient := e.client != nil
	if hadClient {
		e.client.Close()
		e.client = nil
	}
	if err := e.replaceClientLocked(c, saved); err != nil {
		if hadClient {
			if rollback := e.replaceClientLocked(old, saved); rollback != nil {
				return errors.Join(err, fmt.Errorf("restore previous engine: %w", rollback))
			}
		}
		return err
	}
	e.pendingSessions = nil
	return nil
}

func validateDownloadDirectory(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("download directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".cloud-torrent-write-check-*")
	if err != nil {
		return fmt.Errorf("download directory: %w", err)
	}
	name := f.Name()
	return errors.Join(f.Close(), os.Remove(name))
}

func checkListenPort(port int) error {
	address := fmt.Sprintf(":%d", port)
	tcp, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("incoming port: %w", err)
	}
	defer tcp.Close()
	udp, err := net.ListenPacket("udp", address)
	if err != nil {
		return fmt.Errorf("incoming port: %w", err)
	}
	return udp.Close()
}

type savedTorrent struct {
	spec      *torrent.TorrentSpec
	started   bool
	autoStart bool
	files     map[string]bool
}

func (e *Engine) saveTorrentsLocked() []savedTorrent {
	saved := make([]savedTorrent, 0, len(e.ts))
	for _, t := range e.ts {
		saved = append(saved, saveTorrent(t))
	}
	return saved
}

// saveTorrent retains metadata, sources and selection policy before replacing a handle.
func saveTorrent(t *Torrent) savedTorrent {
	spec := cloneSpec(t.spec)
	mi := t.t.Metainfo()
	spec.InfoBytes = slices.Clone(mi.InfoBytes)
	spec.Trackers = mi.UpvertedAnnounceList()
	spec.Webseeds = mi.UrlList
	// Upstream emits an empty non-nil layer map even for v1 torrents.
	// Feeding that back invokes v2 validation against missing v1 roots.
	spec.PieceLayers = mi.PieceLayers
	if len(spec.PieceLayers) == 0 {
		spec.PieceLayers = nil
	}
	state := savedTorrent{spec: spec, started: t.Started, autoStart: t.autoStart, files: map[string]bool{}}
	for _, f := range t.Files {
		if f != nil {
			state.files[f.Path] = f.Started
		}
	}
	if !t.Started && t.selection != nil {
		state.files = maps.Clone(t.selection)
	}
	return state
}

func (e *Engine) replaceClientLocked(c Config, saved []savedTorrent) error {
	config := torrent.NewDefaultClientConfig()
	config.DataDir = c.DownloadDirectory
	// Upstream diagnostics include complete source/tracker URLs and passkeys.
	// Application errors/status remain available without exposing those details.
	config.Logger.SetHandlers(alog.DiscardHandler)
	config.Slogger = slog.New(slog.NewTextHandler(io.Discard, nil))
	config.HTTPProxy = e.outbound.Proxy
	config.HTTPDialContext = e.outbound.DialContext
	config.TrackerDialContext = e.outbound.DialContext
	config.LookupTrackerIp = e.outbound.LookupTracker
	config.WebTransport = e.outbound.Transport()
	config.MetainfoSourcesClient = e.outbound.Client()
	config.NoUpload = !c.EnableUpload
	config.Seed = c.EnableSeeding
	config.ListenPort = c.IncomingPort
	if c.DisableEncryption {
		config.HeaderObfuscationPolicy = torrent.HeaderObfuscationPolicy{Preferred: false, RequirePreferred: true}
	}
	client, err := e.openClientLocked(config)
	if err != nil {
		return fmt.Errorf("configure torrent client: %w", err)
	}
	// Construct the complete replacement before publishing any handles.
	models := make(map[string]*Torrent, len(saved))
	for _, state := range saved {
		model, err := restoreTorrent(client, state)
		if err != nil {
			client.Close()
			return err
		}
		models[model.InfoHash] = model
	}
	e.client, e.config, e.ts = client, c, models
	for _, model := range models {
		e.resumeTorrentLocked(model)
	}
	return nil
}

func restoreTorrent(client *torrent.Client, state savedTorrent) (*Torrent, error) {
	spec := cloneSpec(state.spec)
	spec.DisallowDataDownload = !state.started
	spec.DisallowDataUpload = !state.started
	spec.DisableInitialPieceCheck = false
	spec.IgnoreUnverifiedPieceCompletion = true
	tt, _, err := client.AddTorrentSpec(spec)
	if err != nil {
		return nil, fmt.Errorf("restore torrent: %w", err)
	}
	model := &Torrent{InfoHash: tt.InfoHash().HexString(), Started: state.started, spec: state.spec, autoStart: state.autoStart, selection: maps.Clone(state.files)}
	model.Update(tt)
	if !model.Started {
		tt.DisallowDataDownload()
		tt.DisallowDataUpload()
	}
	for _, f := range model.Files {
		f.Started = state.started && state.files[f.Path]
	}
	return model, nil
}

func (e *Engine) resumeTorrentLocked(model *Torrent) {
	if model.Loaded {
		for _, f := range model.Files {
			if f.Started {
				f.f.Download()
			}
		}
	} else {
		// Preserve pending explicit start intent and initial AutoStart
		// policy, rather than applying a changed policy to old torrents.
		e.watchMetadataLocked(model)
	}
}

// anacrolix closes listener sockets asynchronously after Client.Close returns.
// Retry only address-in-use briefly; other creation failures roll back at once.
func (e *Engine) openClientLocked(config *torrent.ClientConfig) (*torrent.Client, error) {
	deadline := time.Now().Add(time.Second)
	for {
		client, err := e.newClient(config)
		if !errors.Is(err, syscall.EADDRINUSE) || time.Now().After(deadline) {
			return client, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func cloneSpec(spec *torrent.TorrentSpec) *torrent.TorrentSpec {
	copy := *spec
	copy.InfoBytes = slices.Clone(spec.InfoBytes)
	copy.Trackers = make([][]string, len(spec.Trackers))
	for i, tier := range spec.Trackers {
		copy.Trackers[i] = slices.Clone(tier)
	}
	copy.Webseeds = slices.Clone(spec.Webseeds)
	copy.DhtNodes = slices.Clone(spec.DhtNodes)
	copy.PeerAddrs = slices.Clone(spec.PeerAddrs)
	copy.Sources = slices.Clone(spec.Sources)
	copy.PieceLayers = maps.Clone(spec.PieceLayers)
	return &copy
}

// Close shuts down the underlying torrent client.
func (e *Engine) Close() error {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client != nil {
		e.client.Close()
		e.client = nil
	}
	if e.sessionLock != nil {
		err := e.sessionLock.Unlock()
		e.sessionLock = nil
		return err
	}
	return nil
}

// NewMagnet adds a torrent from a magnet URI.
func (e *Engine) NewMagnet(magnetURI string) error {
	spec, err := torrent.TorrentSpecFromMagnetUri(magnetURI)
	if err != nil {
		return err
	}
	return e.NewTorrent(spec)
}

func (e *Engine) validateSpec(spec *torrent.TorrentSpec) error {
	for _, raw := range append(slices.Clone(spec.Sources), spec.Webseeds...) {
		u, err := url.Parse(raw)
		if err != nil || e.outbound.CheckURL(u) != nil {
			return outbound.ErrDenied
		}
	}
	for _, tier := range spec.Trackers {
		for _, raw := range tier {
			u, err := url.Parse(raw)
			if err != nil || u.User != nil || u.Hostname() == "" {
				return outbound.ErrDenied
			}
			switch u.Scheme {
			case "http", "https", "udp", "ws", "wss":
			default:
				return outbound.ErrDenied
			}
		}
	}

	return nil
}

// NewTorrent adds a torrent from a parsed .torrent spec.
func (e *Engine) NewTorrent(spec *torrent.TorrentSpec) error {
	if err := e.validateSpec(spec); err != nil {
		return err
	}

	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return fmt.Errorf("engine is not configured")
	}
	return e.sessionMutationLocked(func() error {
		if len(e.ts) >= maxSessionTorrents {
			return errors.New("torrent admission limit reached")
		}
		ownedSpec := cloneSpec(spec)
		tt, _, err := e.client.AddTorrentSpec(ownedSpec)
		if err != nil {
			return err
		}
		return e.newTorrentLocked(tt, ownedSpec)
	})
}

func (e *Engine) newTorrentLocked(tt *torrent.Torrent, spec *torrent.TorrentSpec) error {
	t := e.upsertTorrentLocked(tt)
	if t.spec != nil {
		// AddTorrentSpec already merges sources/trackers into the live handle.
		// Re-adding must not reset a pending start or create another waiter.
		for _, source := range spec.Sources {
			if !slices.Contains(t.spec.Sources, source) {
				t.spec.Sources = append(t.spec.Sources, source)
			}
		}
		return nil
	}
	t.spec = spec
	t.autoStart = e.config.AutoStart
	e.watchMetadataLocked(t)
	return nil
}

func (e *Engine) watchMetadataLocked(t *Torrent) {
	tt := t.t
	// once metadata arrives, auto-start if configured
	go func() {
		select {
		case <-tt.GotInfo():
		case <-tt.Closed():
			return
		}
		e.mut.Lock()
		defer e.mut.Unlock()
		// A metadata callback must not act on a removed/replaced torrent.
		if e.client != nil && e.ts[t.InfoHash] == t && t.t == tt {
			// Refresh files before restoring a pending start.
			t.Update(tt)
			if t.autoStart {
				t.Started = false
				e.startTorrentLocked(t.InfoHash)
			}
			if err := e.persistLocked(); err != nil {
				log.Print("Session metadata could not be saved; transfer paused. Check the private state directory and available disk space.")
				tt.DisallowDataDownload()
				tt.DisallowDataUpload()
				t.Started = false
				t.autoStart = false
			}
		}
	}()
}

// GetTorrents returns a detached snapshot. The server may retain and serialize
// it independently; later engine mutations cannot alter it.
func (e *Engine) GetTorrents() map[string]*Torrent {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return nil
	}
	e.refreshLocked()
	snapshot := make(map[string]*Torrent, len(e.ts))
	for ih, t := range e.ts {
		copy := *t
		copy.t = nil
		copy.spec = nil
		copy.autoStart = false
		copy.selection = nil
		copy.updatedAt = time.Time{}
		if t.Files != nil {
			copy.Files = make([]*File, len(t.Files))
			for i, f := range t.Files {
				if f == nil {
					continue
				}
				file := *f
				file.f = nil
				copy.Files[i] = &file
			}
		}
		snapshot[ih] = &copy
	}
	return snapshot
}

func (e *Engine) refreshLocked() {
	for _, tt := range e.client.Torrents() {
		e.upsertTorrentLocked(tt)
	}
}

func (e *Engine) upsertTorrentLocked(tt *torrent.Torrent) *Torrent {
	ih := tt.InfoHash().HexString()
	t, ok := e.ts[ih]
	if !ok {
		t = &Torrent{InfoHash: ih}
		e.ts[ih] = t
	}
	t.Update(tt)
	return t
}

func (e *Engine) getTorrentLocked(infohash string) (*Torrent, error) {
	if e.client == nil {
		return nil, fmt.Errorf("engine is not configured")
	}
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
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.sessionMutationLocked(func() error { return e.startTorrentLocked(infohash) })
}

func (e *Engine) startTorrentLocked(infohash string) error {
	t, err := e.getTorrentLocked(infohash)
	if err != nil {
		return err
	}
	t.Update(t.t)
	allStarted := t.Started
	for _, f := range t.Files {
		if f != nil && !f.Started {
			allStarted = false
		}
	}
	if allStarted {
		return fmt.Errorf("already started")
	}
	t.Started = true
	t.selection = nil
	t.autoStart = !t.Loaded
	t.t.AllowDataDownload()
	if e.config.EnableUpload {
		t.t.AllowDataUpload()
	}
	for _, f := range t.Files {
		if f != nil {
			f.f.Download()
			f.Started = true
		}
	}
	return nil
}

// StopTorrent pauses transfer while retaining metadata and the live handle.
// StartTorrent restores all file priorities and resumes transfer.
func (e *Engine) StopTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.sessionMutationLocked(func() error {
		t, err := e.getTorrentLocked(infohash)
		if err != nil {
			return err
		}
		if !t.Started && !t.autoStart {
			return fmt.Errorf("already stopped")
		}
		t.selection = map[string]bool{}
		for _, f := range t.Files {
			if f != nil {
				t.selection[f.Path] = f.Started
			}
		}
		t.t.DisallowDataDownload()
		t.t.DisallowDataUpload()
		t.Started = false
		t.autoStart = false
		t.DownloadRate = 0
		for _, f := range t.Files {
			if f != nil {
				f.f.SetPriority(torrent.PiecePriorityNone)
				f.Started = false
			}
		}
		return nil
	})
}

// DeleteTorrent removes a torrent from the engine and drops it from the client.
func (e *Engine) DeleteTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.sessionMutationLocked(func() error {
		t, err := e.getTorrentLocked(infohash)
		if err != nil {
			return err
		}
		delete(e.ts, t.InfoHash)
		ih, _ := str2ih(infohash)
		if tt, ok := e.client.Torrent(ih); ok {
			tt.Drop()
		}
		return nil
	})
}

// StartFile marks a single file for download.
func (e *Engine) StartFile(infohash, path string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.sessionMutationLocked(func() error {
		t, err := e.getTorrentLocked(infohash)
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
		f.f.Download()
		t.t.AllowDataDownload()
		if e.config.EnableUpload {
			t.t.AllowDataUpload()
		}
		t.autoStart = false
		t.Started = true
		f.Started = true
		return nil
	})
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
