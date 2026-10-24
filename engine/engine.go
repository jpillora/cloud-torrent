package engine

import (
	"encoding/hex"
	"errors"
	"fmt"
	alog "github.com/anacrolix/log"
	"io"
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
	"golang.org/x/time/rate"
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
	downloadLimiter *rate.Limiter
	uploadLimiter   *rate.Limiter
	sessionDir      string
	sessionLock     *flock.Flock
	pendingSessions []savedTorrent
	queueSequence   uint64
	clock           func() time.Time
	accountDirty    bool
	mutationDepth   int
}

// New returns an unconfigured engine. Call Configure before adding torrents.
func New(policies ...*outbound.Policy) *Engine {
	policy, _ := outbound.New("")
	if len(policies) > 0 && policies[0] != nil {
		policy = policies[0]
	}
	return &Engine{downloadLimiter: rate.NewLimiter(rate.Inf, bandwidthBurst), uploadLimiter: rate.NewLimiter(rate.Inf, bandwidthBurst), outbound: policy, ts: map[string]*Torrent{}, newClient: torrent.NewClient}
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
	if e.sessionDir != "" && e.sessionLock == nil {
		return errors.New("session store is closed; reopen it before configuring the engine")
	}
	if err := validateSeeding(c); err != nil {
		return err
	}
	e.accountSeedingLocked(e.nowLocked())
	if err := validateQueue(c); err != nil {
		return err
	}
	if err := validateBandwidth(c); err != nil {
		return err
	}
	c.IncomingPort = defaultPort(c.IncomingPort)
	old := e.config
	if e.client != nil {
		sameClient := old
		sameClient.AutoStart = c.AutoStart
		sameClient.DownloadLimitKiB = c.DownloadLimitKiB
		sameClient.UploadLimitKiB = c.UploadLimitKiB
		sameClient.MaxActiveDownloads = c.MaxActiveDownloads
		sameClient.MaxActiveMetadata = c.MaxActiveMetadata
		sameClient.SeedRatioLimit = c.SeedRatioLimit
		sameClient.SeedTimeMinutes = c.SeedTimeMinutes
		if sameClient == c {
			e.applyBandwidthLocked(c)
			e.config = c
			return e.scheduleLocked()
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
	spec           *torrent.TorrentSpec
	started        bool
	autoStart      bool
	files          map[string]bool
	wantStart      bool
	order          uint64
	metadataPaused bool
	account        seedAccount
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
	if t.t != nil {
		mi := t.t.Metainfo()
		if len(mi.InfoBytes) > 0 {
			spec.InfoBytes = slices.Clone(mi.InfoBytes)
		}
		spec.Trackers = mi.UpvertedAnnounceList()
		spec.Webseeds = mi.UrlList
		spec.PieceLayers = mi.PieceLayers
		if len(spec.PieceLayers) == 0 {
			spec.PieceLayers = nil
		}
	}
	return savedTorrent{spec: spec, started: t.Started, autoStart: t.autoStart, files: maps.Clone(t.selection), wantStart: t.wantStart, order: t.order, metadataPaused: t.metadataPaused, account: t.account}
}

func (e *Engine) replaceClientLocked(c Config, saved []savedTorrent) error {
	config := torrent.NewDefaultClientConfig()
	e.applyBandwidthLocked(c)
	config.DownloadRateLimiter = e.downloadLimiter
	config.UploadRateLimiter = e.uploadLimiter
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
	// Publish paused models first; the single scheduler owns all admission.
	models := make(map[string]*Torrent, len(saved))
	for _, state := range saved {
		model := restoreQueuedModel(state)
		models[model.InfoHash] = model
		if model.order > e.queueSequence {
			e.queueSequence = model.order
		}
	}
	e.client, e.config, e.ts = client, c, models
	if err := e.scheduleLocked(); err != nil {
		client.Close()
		e.client = nil
		return err
	}
	return nil
}

// Deletion reconciliation restores known metadata with transfers held until
// the engine scheduler admits it again.
func restoreTorrent(client *torrent.Client, state savedTorrent) (*Torrent, error) {
	model := restoreQueuedModel(state)
	if len(state.spec.InfoBytes) == 0 {
		return model, nil
	}
	spec := cloneSpec(state.spec)
	spec.DisallowDataDownload = true
	spec.DisallowDataUpload = true
	spec.DisableInitialPieceCheck = false
	spec.IgnoreUnverifiedPieceCompletion = true
	tt, _, err := addQueuedSpec(client, spec)
	if err != nil {
		return nil, fmt.Errorf("restore torrent: %w", err)
	}
	model.Update(tt)
	return model, nil
}
func (e *Engine) resumeTorrentLocked(model *Torrent) { _ = e.scheduleLocked() }

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
	if e.client == nil && e.sessionLock == nil {
		return nil
	}
	e.accountSeedingLocked(e.nowLocked())
	var persistErr error
	if e.accountDirty {
		persistErr = e.persistLocked()
	}
	for _, model := range e.ts {
		model.wasSeeding = false
	}
	if e.client != nil {
		e.client.Close()
		e.client = nil
	}
	if e.sessionLock != nil {
		err := e.sessionLock.Unlock()
		e.sessionLock = nil
		return errors.Join(persistErr, err)
	}
	return persistErr
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
	owned := cloneSpec(spec)
	if specIdentity(owned) == "" && len(owned.InfoBytes) > 0 {
		normalized, err := torrent.TorrentSpecFromMetaInfoErr(&metainfo.MetaInfo{InfoBytes: owned.InfoBytes})
		if err != nil {
			return errors.New("invalid metainfo")
		}
		owned.InfoHash = normalized.InfoHash
		owned.InfoHashV2 = normalized.InfoHashV2
	}
	key := specIdentity(owned)
	if key == "" {
		return errors.New("missing torrent identity")
	}
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client == nil {
		return errors.New("engine is not configured")
	}
	return e.sessionMutationLocked(func() error {
		if existing := e.findIdentityLocked(owned); existing != nil {
			if existing.t != nil {
				merge := cloneSpec(owned)
				merge.ChunkSize = 0
				if err := existing.t.MergeSpec(merge); err != nil {
					return err
				}
			}
			for _, source := range owned.Sources {
				if !slices.Contains(existing.spec.Sources, source) {
					existing.spec.Sources = append(existing.spec.Sources, source)
				}
			}
			if len(owned.InfoBytes) > 0 {
				existing.spec.InfoBytes = owned.InfoBytes
				existing.spec.PieceLayers = owned.PieceLayers
			}
			existing.spec.Trackers = append(existing.spec.Trackers, owned.Trackers...)
			existing.spec.Webseeds = append(existing.spec.Webseeds, owned.Webseeds...)
			return e.scheduleLocked()
		}
		if len(e.ts) >= maxSessionTorrents {
			return errors.New("torrent admission limit reached")
		}
		model := &Torrent{InfoHash: key, Name: owned.DisplayName, spec: owned, autoStart: e.config.AutoStart, wantStart: e.config.AutoStart, order: e.nextOrderLocked(), selection: map[string]bool{}}
		e.ts[key] = model
		if err := e.scheduleLocked(); err != nil {
			if model.t != nil {
				model.t.Drop()
			}
			delete(e.ts, key)
			return err
		}
		return nil
	})
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
		// Stale callbacks from a removed or capacity-suspended handle are inert.
		if e.client != nil && e.ts[t.InfoHash] == t && t.t == tt {
			t.Update(tt)
			if err := e.scheduleLocked(); err != nil {
				tt.DisallowDataDownload()
				tt.DisallowDataUpload()
				return
			}
			if err := e.persistLocked(); err != nil {
				tt.DisallowDataDownload()
				tt.DisallowDataUpload()
				t.Started = false
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
	_ = e.scheduleLocked()
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
	for _, model := range e.ts {
		if model.t == tt {
			model.Update(tt)
			return model
		}
	}
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

// StartTorrent requests all files through FIFO admission, including pending metadata.
func (e *Engine) StartTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	return e.sessionMutationLocked(func() error { return e.startTorrentLocked(infohash) })
}
func (e *Engine) startTorrentLocked(infohash string) error {
	model, err := e.getTorrentLocked(infohash)
	if err != nil {
		return err
	}
	return e.queueStartLocked(model, true, "")
}
func (e *Engine) StopTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	model, err := e.getTorrentLocked(infohash)
	if err != nil {
		return err
	}
	if !model.wantStart && (model.Loaded || model.metadataPaused) {
		return errors.New("already stopped")
	}
	return e.sessionMutationLocked(func() error {
		model.wantStart = false
		model.account.Override = false
		model.autoStart = false
		model.metadataPaused = !model.Loaded
		return e.scheduleLocked()
	})
}
func (e *Engine) DeleteTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	model, err := e.getTorrentLocked(infohash)
	if err != nil {
		return err
	}
	return e.sessionMutationLocked(func() error {
		delete(e.ts, model.InfoHash)
		if model.t != nil {
			model.t.Drop()
		}
		return e.scheduleLocked()
	})
}
func (e *Engine) StartFile(infohash, path string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	model, err := e.getTorrentLocked(infohash)
	if err != nil {
		return err
	}
	return e.sessionMutationLocked(func() error { return e.queueStartLocked(model, false, path) })
}

// StopFile deselects an exact torrent file. Shared boundary pieces and already
// admitted writes can still touch its bytes; skipping never removes content.
func (e *Engine) StopFile(infohash, path string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	model, err := e.getTorrentLocked(infohash)
	if err != nil {
		return err
	}
	found := false
	for _, file := range model.Files {
		if file.Path == path {
			found = true
		}
	}
	if !found {
		return errors.New("missing file")
	}
	if !model.selection[path] {
		return errors.New("file already skipped")
	}
	return e.sessionMutationLocked(func() error {
		model.selection[path] = false
		// Upstream coalesces webseed requests across files. A priority change alone
		// lets an already admitted stream keep reading skipped unique pieces. Cancel
		// data requests once, then let the scheduler resume only selected priorities.
		for _, file := range model.Files {
			if file.Path == path {
				file.f.SetPriority(torrent.PiecePriorityNone)
				file.Started = false
				file.Selected = false
			}
		}
		if model.t != nil {
			model.t.DisallowDataDownload()
		}
		model.autoStart = false
		anySelected := false
		for _, selected := range model.selection {
			anySelected = anySelected || selected
		}
		if !anySelected {
			model.account.StopReason = ""
			model.wantStart = false
			model.account.Override = false
		}
		return e.scheduleLocked()
	})
}

// ResumeTorrent preserves the existing selected subset. StartTorrent is the
// explicit Start all action, retained for compatibility with existing clients.
func (e *Engine) ResumeTorrent(infohash string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	model, err := e.getTorrentLocked(infohash)
	if err != nil {
		return err
	}
	if seedPolicyReason(model.account.StopReason) {
		return errors.New("seeding limit reached; use Resume sharing to override")
	}
	if model.wantStart {
		return errors.New("already started or queued")
	}
	selected := false
	for _, want := range model.selection {
		selected = selected || want
	}
	if !selected {
		return errors.New("select at least one file or use Start all")
	}
	return e.sessionMutationLocked(func() error {
		model.wantStart = true
		model.metadataPaused = false
		model.account.StopReason = ""
		model.order = e.nextOrderLocked()
		return e.scheduleLocked()
	})
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
