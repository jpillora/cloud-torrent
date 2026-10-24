package engine

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"sort"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	infohashv2 "github.com/anacrolix/torrent/types/infohash-v2"
)

func validateQueue(c Config) error {
	if c.MaxActiveDownloads < 0 || c.MaxActiveDownloads > maxSessionTorrents {
		return errors.New("active downloads must be 0 (unlimited) to 256")
	}
	if c.MaxActiveMetadata < 0 || c.MaxActiveMetadata > 64 {
		return errors.New("metadata concurrency must be 0 (default four) to 64")
	}
	return nil
}
func (e *Engine) nextOrderLocked() uint64 { e.queueSequence++; return e.queueSequence }
func (e *Engine) orderedLocked() []*Torrent {
	models := make([]*Torrent, 0, len(e.ts))
	for _, model := range e.ts {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].order == models[j].order {
			return models[i].InfoHash < models[j].InfoHash
		}
		return models[i].order < models[j].order
	})
	return models
}
func specIdentity(spec *torrent.TorrentSpec) string {
	if spec.InfoHash != (metainfo.Hash{}) {
		return spec.InfoHash.HexString()
	}
	if spec.InfoHashV2.Ok {
		var hash metainfo.Hash
		copy(hash[:], spec.InfoHashV2.Value[:])
		return hash.HexString()
	}
	return ""
}
func (e *Engine) attachLocked(model *Torrent) error {
	spec := cloneSpec(model.spec)
	spec.DisallowDataDownload = true
	spec.DisallowDataUpload = true
	spec.DisableInitialPieceCheck = false
	spec.IgnoreUnverifiedPieceCompletion = true
	tt, _, err := addQueuedSpec(e.client, spec)
	if err != nil {
		return fmt.Errorf("add queued torrent: %w", err)
	}
	model.Update(tt)
	if !model.Loaded {
		e.watchMetadataLocked(model)
	}
	return nil
}

// All queue transitions share Engine.mut. Completed selections may seed without
// consuming a download slot. Pending magnets consume only metadata slots and
// wait without a client handle when metadata capacity is full.
func (e *Engine) scheduleLocked() error {
	if e.client == nil {
		return nil
	}
	e.accountSeedingLocked(e.nowLocked())
	ordered := e.orderedLocked()
	metadataLimit := e.config.MaxActiveMetadata
	if metadataLimit == 0 {
		metadataLimit = 4
	}
	pending := 0
	for _, model := range ordered {
		if model.t != nil {
			model.Update(model.t)
		}
		if !model.Loaded && model.t != nil {
			if model.metadataPaused || pending >= metadataLimit {
				model.t.Drop()
				model.t = nil
				model.Started = false
			} else {
				pending++
			}
		}
	}
	for _, model := range ordered {
		if model.t == nil {
			if len(model.spec.InfoBytes) == 0 && (model.metadataPaused || pending >= metadataLimit) {
				continue
			}
			if err := e.attachLocked(model); err != nil {
				return err
			}
			if !model.Loaded {
				pending++
			}
		}
	}
	active, position := 0, 0
	for _, model := range ordered {
		model.QueuePosition = 0
		if !model.Loaded {
			model.Started = model.wantStart && model.t != nil
			switch {
			case model.metadataPaused:
				model.State = "paused"
			case model.t != nil:
				model.State = "metadata"
			default:
				model.State = "queued"
			}
			if !model.metadataPaused && model.t == nil {
				position++
				model.QueuePosition = position
			}
			continue
		}
		if model.autoStart {
			if model.selection == nil {
				model.selection = map[string]bool{}
			}
			for _, file := range model.Files {
				model.selection[file.Path] = true
			}
			model.autoStart = false
		}
		selected, complete := false, true
		for _, file := range model.Files {
			if model.selection[file.Path] {
				selected = true
				if file.Size > 0 && file.Percent < 100 {
					complete = false
				}
			}
		}
		projectSeedAccount(model)
		if model.wantStart && selected {
			if reason := e.seedLimitLocked(model, complete); reason != "" {
				model.wantStart = false
				model.autoStart = false
				model.account.StopReason = reason
				e.accountDirty = true
			}
		}
		admit := model.wantStart && selected && (complete || e.config.MaxActiveDownloads == 0 || active < e.config.MaxActiveDownloads)
		if admit && !complete {
			active++
		}
		model.Started = admit
		if admit {
			model.t.AllowDataDownload()
			if e.config.EnableUpload {
				model.t.AllowDataUpload()
			} else {
				model.t.DisallowDataUpload()
			}
			if complete {
				if e.config.EnableSeeding && e.config.EnableUpload {
					model.State = "seeding"
				} else {
					model.State = "completed"
				}
			} else {
				model.State = "downloading"
			}
		} else {
			model.t.DisallowDataDownload()
			model.t.DisallowDataUpload()
			model.DownloadRate = 0
			model.State = "paused"
			if model.wantStart {
				model.State = "queued"
				position++
				model.QueuePosition = position
			}
		}
		model.wasSeeding = admit && complete && e.config.EnableSeeding && e.config.EnableUpload
		projectSeedAccount(model)
		for _, file := range model.Files {
			started := admit && model.selection[file.Path]
			// Avoid re-issuing priorities on every status poll.
			if started != file.Started {
				if started {
					file.f.Download()
				} else {
					file.f.SetPriority(torrent.PiecePriorityNone)
				}
			}
			file.Started = started
		}
	}
	if e.accountDirty && e.mutationDepth == 0 {
		if err := e.persistLocked(); err != nil {
			for _, model := range ordered {
				if model.wasSeeding {
					model.wantStart = false
					model.Started = false
					model.wasSeeding = false
					model.State = "paused"
					for _, file := range model.Files {
						file.Started = false
						file.f.SetPriority(torrent.PiecePriorityNone)
					}
					model.account.StopReason = "state-unavailable"
					projectSeedAccount(model)
					model.t.DisallowDataUpload()
					model.t.DisallowDataDownload()
				}
			}
			return err
		}
	}
	return nil
}
func (e *Engine) queueStartLocked(model *Torrent, all bool, path string) error {
	if seedPolicyReason(model.account.StopReason) {
		return errors.New("seeding limit reached; use Resume sharing to override")
	}
	if model.selection == nil {
		model.selection = map[string]bool{}
	}
	if all {
		if model.wantStart && (model.autoStart || (model.Loaded && len(model.selection) == len(model.Files))) {
			allSelected := true
			for _, file := range model.Files {
				if !model.selection[file.Path] {
					allSelected = false
				}
			}
			if allSelected {
				return errors.New("already started or queued")
			}
		}
		if !model.Loaded {
			model.autoStart = true
		} else {
			for _, file := range model.Files {
				model.selection[file.Path] = true
			}
		}
	} else {
		found := false
		for _, file := range model.Files {
			if file.Path == path {
				found = true
			}
		}
		if !found {
			return errors.New("missing file")
		}
		if model.wantStart && model.selection[path] {
			return errors.New("already selected")
		}
		model.selection[path] = true
		model.autoStart = false
	}
	if !model.wantStart {
		model.order = e.nextOrderLocked()
	}
	model.account.StopReason = ""
	model.wantStart = true
	model.metadataPaused = false
	return e.scheduleLocked()
}

func restoreQueuedModel(state savedTorrent) *Torrent {
	return &Torrent{InfoHash: specIdentity(state.spec), Name: state.spec.DisplayName, spec: state.spec, wantStart: state.wantStart, autoStart: state.autoStart, selection: maps.Clone(state.files), order: state.order, metadataPaused: state.metadataPaused, account: state.account}
}

// Metadata proves cross-version identities; unresolved magnets with unrelated
// hashes cannot be treated as duplicates until that proof arrives.
func identityKeys(spec *torrent.TorrentSpec) map[string]bool {
	keys := map[string]bool{}
	if spec.InfoHash != (metainfo.Hash{}) {
		keys["v1:"+spec.InfoHash.HexString()] = true
	}
	if spec.InfoHashV2.Ok {
		keys[fmt.Sprintf("v2:%x", spec.InfoHashV2.Value)] = true
	}
	if len(spec.InfoBytes) > 0 {
		info, err := (&metainfo.MetaInfo{InfoBytes: spec.InfoBytes}).UnmarshalInfo()
		if err == nil {
			if info.HasV1() {
				keys["v1:"+metainfo.HashBytes(spec.InfoBytes).HexString()] = true
			}
			if info.HasV2() {
				keys[fmt.Sprintf("v2:%x", infohashv2.HashBytes(spec.InfoBytes))] = true
			}
		}
	}
	return keys
}
func sharedIdentity(a, b *torrent.TorrentSpec) bool {
	keys := identityKeys(a)
	for key := range identityKeys(b) {
		if keys[key] {
			return true
		}
	}
	return false
}
func (e *Engine) findIdentityLocked(spec *torrent.TorrentSpec) *Torrent {
	var match *Torrent
	for _, candidate := range e.orderedLocked() {
		saved := saveTorrent(candidate)
		if !sharedIdentity(spec, saved.spec) {
			continue
		}
		if match == nil {
			match = candidate
			continue
		}
		// Keep the earlier FIFO entry. Explicit pause wins when previously unknown
		// aliases collapse, so resolving metadata cannot resume a paused torrent.
		if candidate.account.Uploaded > maxSafeTransferBytes-match.account.Uploaded {
			match.account.Uploaded = maxSafeTransferBytes
		} else {
			match.account.Uploaded += candidate.account.Uploaded
		}
		if candidate.account.SeedNanos > math.MaxInt64-match.account.SeedNanos {
			match.account.SeedNanos = math.MaxInt64
		} else {
			match.account.SeedNanos += candidate.account.SeedNanos
		}
		match.wantStart = match.wantStart && candidate.wantStart
		match.metadataPaused = match.metadataPaused || candidate.metadataPaused
		if match.selection == nil {
			match.selection = map[string]bool{}
		}
		for path, selected := range candidate.selection {
			if selected {
				match.selection[path] = true
			}
		}
		if candidate.t != nil && candidate.t != match.t {
			candidate.t.Drop()
		}
		delete(e.ts, candidate.InfoHash)
	}
	return match
}

// v1.59.1 indexes AddTorrentOpts.InfoHash even for a v2-only add, leaving an
// orphan zero key on Drop/Close. Its supported truncated-v2 path avoids that
// bug and fills both full identities when metadata arrives. Retain the original
// full hashes in our model/store; only adapt the client-bound copy.
func addQueuedSpec(client *torrent.Client, spec *torrent.TorrentSpec) (*torrent.Torrent, bool, error) {
	adapted := cloneSpec(spec)
	if adapted.InfoHashV2.Ok {
		if adapted.InfoHash == (metainfo.Hash{}) {
			copy(adapted.InfoHash[:], adapted.InfoHashV2.Value[:20])
		}
		adapted.InfoHashV2.SetNone()
	}
	return client.AddTorrentSpec(adapted)
}
