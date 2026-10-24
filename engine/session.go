package engine

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"

	g "github.com/anacrolix/generics"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	infohashv2 "github.com/anacrolix/torrent/types/infohash-v2"
	"github.com/gofrs/flock"
)

const maxSessionBytes = 16 << 20
const maxSessionTorrents = 256

type sessionEntry struct {
	Magnet                                 string
	InfoBytes                              []byte
	Trackers                               [][]string
	Webseeds, Sources, DhtNodes, PeerAddrs []string
	PieceLayers                            []sessionLayer
	Started, AutoStart                     bool
	WantStart                              *bool
	Order                                  uint64
	MetadataPaused                         bool
	Files                                  map[string]bool
	Account                                seedAccount
}
type sessionLayer struct{ Key, Value []byte }

type sessionDisk struct {
	Version       int
	Torrents      []sessionEntry
	InboxReceipts []string
}

// EnablePersistence must be called before Configure. The directory is operator
// configuration, never a browser setting. A process lock prevents lost updates.
func (e *Engine) EnablePersistence(dir string) error {
	e.mut.Lock()
	defer e.mut.Unlock()
	if e.client != nil || e.sessionLock != nil {
		return errors.New("session store already configured")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	st, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("session directory must be a real directory")
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(dir, "sessions.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !locked {
		return errors.New("session directory is already in use")
	}
	e.sessionDir = dir
	e.sessionLock = lock
	pending, err := e.readSessionsLocked()
	if err != nil {
		lock.Unlock()
		e.sessionLock = nil
		e.sessionDir = ""
		return fmt.Errorf("invalid session store: %w; stop the server and restore a backup or move sessions.json aside to start empty", err)
	}
	e.pendingSessions = pending
	return nil
}
func (e *Engine) readSessionsLocked() ([]savedTorrent, error) {
	root, err := os.OpenRoot(e.sessionDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open("sessions.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSessionBytes {
		return nil, errors.New("session file exceeds limits")
	}
	decoder := json.NewDecoder(io.LimitReader(f, maxSessionBytes+1))
	decoder.DisallowUnknownFields()
	var disk sessionDisk
	if err = decoder.Decode(&disk); err != nil {
		return nil, errors.New("malformed session data")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("trailing session data")
	}
	if disk.Version != 1 {
		return nil, errors.New("unsupported session version")
	}
	if len(disk.Torrents) > maxSessionTorrents {
		return nil, errors.New("too many saved torrents")
	}
	if len(disk.InboxReceipts) > maxInboxReceipts {
		return nil, errors.New("too many inbox receipts")
	}
	receipts := map[string]bool{}
	for _, receipt := range disk.InboxReceipts {
		decoded, err := hex.DecodeString(receipt)
		if err != nil || len(decoded) != 32 || receipt != hex.EncodeToString(decoded) || receipts[receipt] {
			return nil, errors.New("invalid inbox receipts")
		}
		receipts[receipt] = true
	}
	saved := make([]savedTorrent, 0, len(disk.Torrents))
	seen := map[string]bool{}
	for _, entry := range disk.Torrents {
		spec, err := torrent.TorrentSpecFromMagnetUri(entry.Magnet)
		if err != nil {
			return nil, errors.New("invalid saved magnet")
		}
		spec.InfoBytes = entry.InfoBytes
		spec.Trackers = entry.Trackers
		spec.Webseeds = entry.Webseeds
		spec.Sources = entry.Sources
		spec.DhtNodes = entry.DhtNodes
		spec.PeerAddrs = entry.PeerAddrs
		if len(entry.PieceLayers) > 0 {
			spec.PieceLayers = map[string]string{}
			for _, layer := range entry.PieceLayers {
				spec.PieceLayers[string(layer.Key)] = string(layer.Value)
			}
		}
		if err = e.validateSpec(spec); err != nil {
			return nil, errors.New("saved source violates outbound policy")
		}
		if len(spec.InfoBytes) > 0 {
			var mi metainfo.Info
			// Verify the content identity before opening any torrent handles.
			meta := metainfo.MetaInfo{InfoBytes: spec.InfoBytes}
			mi, err = meta.UnmarshalInfo()
			if err != nil || (spec.InfoHash != (metainfo.Hash{}) && (!mi.HasV1() || spec.InfoHash != metainfo.HashBytes(spec.InfoBytes))) {
				return nil, errors.New("invalid saved metainfo")
			}
			if spec.InfoHashV2.Ok && (!mi.HasV2() || spec.InfoHashV2.Value != infohashv2.HashBytes(spec.InfoBytes)) {
				return nil, errors.New("invalid saved v2 identity")
			}
		}
		for key := range identityKeys(spec) {
			if seen[key] {
				return nil, errors.New("duplicate saved torrent")
			}
			seen[key] = true
		}

		want := entry.Started || entry.AutoStart
		if entry.WantStart != nil {
			want = *entry.WantStart
		}
		order := entry.Order
		if order == 0 {
			order = uint64(len(saved) + 1)
		}
		if entry.Account.Uploaded < 0 || entry.Account.Uploaded > maxSafeTransferBytes || entry.Account.SeedNanos < 0 {
			return nil, errors.New("invalid seeding accounting")
		}
		saved = append(saved, savedTorrent{spec: spec, started: entry.Started, autoStart: entry.AutoStart, files: entry.Files, wantStart: want, order: order, metadataPaused: entry.MetadataPaused, account: entry.Account})
	}
	e.inboxReceipts = receipts
	return saved, nil
}
func (e *Engine) persistLocked() error {
	if e.sessionDir == "" {
		e.accountDirty = false
		return nil
	}
	states := e.saveTorrentsLocked()
	if len(states) > maxSessionTorrents {
		return errors.New("session torrent limit reached")
	}
	disk := sessionDisk{Version: 1, Torrents: make([]sessionEntry, 0, len(states))}
	for receipt := range e.inboxReceipts {
		disk.InboxReceipts = append(disk.InboxReceipts, receipt)
	}
	sort.Strings(disk.InboxReceipts)
	for _, state := range states {
		spec := state.spec
		magnet := metainfo.MagnetV2{V2InfoHash: spec.InfoHashV2, DisplayName: spec.DisplayName}
		if spec.InfoHash != (metainfo.Hash{}) {
			magnet.InfoHash = g.Some(spec.InfoHash)
		}
		disk.Torrents = append(disk.Torrents, sessionEntry{Magnet: magnet.String(), InfoBytes: spec.InfoBytes, Trackers: spec.Trackers, Webseeds: spec.Webseeds, Sources: spec.Sources, DhtNodes: spec.DhtNodes, PeerAddrs: spec.PeerAddrs, PieceLayers: encodeSessionLayers(spec.PieceLayers), Started: state.started, AutoStart: state.autoStart, Files: state.files, WantStart: &state.wantStart, Order: state.order, MetadataPaused: state.metadataPaused, Account: state.account})
	}
	sort.Slice(disk.Torrents, func(i, j int) bool { return disk.Torrents[i].Magnet < disk.Torrents[j].Magnet })
	data, err := json.Marshal(disk)
	if err != nil {
		return errors.New("cannot encode session state")
	}
	if len(data) > maxSessionBytes {
		return errors.New("session state exceeds size limit")
	}
	f, err := os.CreateTemp(e.sessionDir, ".session-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, filepath.Join(e.sessionDir, "sessions.json")); err != nil {
		return err
	}
	// The file was fsynced before rename; sync directory entries where supported.
	if dir, openErr := os.Open(e.sessionDir); openErr == nil {
		dir.Sync()
		dir.Close()
	}
	e.accountDirty = false
	return nil
}

// Commit intent before acknowledging actions. A failed durable write restores
// the previous handles and priorities, leaving the old atomic file untouched.
func (e *Engine) sessionMutationLocked(change func() error) error {
	e.accountSeedingLocked(e.nowLocked())
	e.mutationDepth++
	defer func() { e.mutationDepth-- }()
	if e.sessionDir == "" {
		return change()
	}
	before := e.saveTorrentsLocked()
	beforeReceipts := maps.Clone(e.inboxReceipts)
	if err := change(); err != nil {
		return err
	}
	if err := e.persistLocked(); err != nil {
		e.inboxReceipts = beforeReceipts
		e.client.Close()
		e.client = nil
		rollback := e.replaceClientLocked(e.config, before)
		return errors.Join(fmt.Errorf("save session: %w", err), rollback)
	}
	return nil
}

func encodeSessionLayers(layers map[string]string) []sessionLayer {
	var result []sessionLayer
	for key, value := range layers {
		result = append(result, sessionLayer{[]byte(key), []byte(value)})
	}
	sort.Slice(result, func(i, j int) bool { return string(result[i].Key) < string(result[j].Key) })
	return result
}
