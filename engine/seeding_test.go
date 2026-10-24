package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

func seedTestEngine(t *testing.T, dir, state string, seconds *atomic.Int64, c Config) *Engine {
	t.Helper()
	e := New()
	e.clock = func() time.Time { return time.Unix(1700000000, 0).Add(time.Duration(seconds.Load()) * time.Second) }
	e.newClient = func(cfg *torrent.ClientConfig) (*torrent.Client, error) {
		cfg.NoDHT = true
		cfg.DisableTrackers = true
		cfg.NoDefaultPortForwarding = true
		cfg.DisablePEX = true
		cfg.DisableUTP = true
		cfg.DisableWebtorrent = true
		return torrent.NewClient(cfg)
	}
	if state != "" {
		bandwidthMust(t, e.EnablePersistence(state))
	}
	c.DownloadDirectory = dir
	c.IncomingPort = freePort(t)
	bandwidthMust(t, e.Configure(c))
	t.Cleanup(func() { e.Close() })
	return e
}
func selectedSeedFixture(t *testing.T, seconds *atomic.Int64, state string) (*Engine, string, string) {
	t.Helper()
	dir := t.TempDir()
	fixtureDir := filepath.Join(dir, "restart-fixture")
	bandwidthMust(t, os.MkdirAll(fixtureDir, 0755))
	bandwidthMust(t, os.WriteFile(filepath.Join(fixtureDir, "one"), bytes.Repeat([]byte("a"), 16), 0600))
	e := seedTestEngine(t, dir, state, seconds, Config{EnableUpload: true, EnableSeeding: true, SeedTimeMinutes: 1})
	spec := sessionFixture(t)
	bandwidthMust(t, e.NewTorrent(spec))
	ih := spec.InfoHash.HexString()
	waitSessionBytes(t, e, ih, 16)
	bandwidthMust(t, e.StartFile(ih, "restart-fixture/one"))
	return e, ih, dir
}
func TestSeedingTimeBoundaryPauseSelectionAndOverride(t *testing.T) {
	var seconds atomic.Int64
	e, ih, dir := selectedSeedFixture(t, &seconds, "")
	if model := e.GetTorrents()[ih]; model.State != "seeding" || model.Percent != 50 {
		t.Fatal("selected completion did not enter seeding", model)
	}
	seconds.Store(30)
	if e.GetTorrents()[ih].SeedSeconds != 30 {
		t.Fatal("active time not counted")
	}
	bandwidthMust(t, e.StopTorrent(ih))
	seconds.Store(330)
	if model := e.GetTorrents()[ih]; model.SeedSeconds != 30 || model.Started {
		t.Fatal("paused time accrued", model)
	}
	bandwidthMust(t, e.StartFile(ih, "restart-fixture/one"))
	seconds.Store(359)
	if model := e.GetTorrents()[ih]; model.SeedSeconds != 59 || model.State != "seeding" {
		t.Fatal("stopped before boundary", model)
	}
	seconds.Store(360)
	model := e.GetTorrents()[ih]
	if model.State != "paused" || model.StopReason != "seed-time" || model.SeedSeconds != 60 {
		t.Fatal("time boundary missed", model)
	}
	if e.StartTorrent(ih) == nil || e.StartFile(ih, "restart-fixture/one") == nil {
		t.Fatal("ordinary start bypassed policy")
	}
	bytesOnDisk, err := os.ReadFile(filepath.Join(dir, "restart-fixture", "one"))
	bandwidthMust(t, err)
	if !bytes.Equal(bytesOnDisk, bytes.Repeat([]byte("a"), 16)) {
		t.Fatal("policy removed or changed content")
	}
	bandwidthMust(t, e.ResumeSeeding(ih))
	seconds.Store(3960)
	model = e.GetTorrents()[ih]
	if model.State != "seeding" || !model.SeedingOverride || model.SeedSeconds != 3660 {
		t.Fatal("deliberate override not retained", model)
	}
	e.mut.Lock()
	selected := e.ts[ih].selection["restart-fixture/one"] && !e.ts[ih].selection["restart-fixture/two"]
	e.mut.Unlock()
	if !selected {
		t.Fatal("budget lost file selection")
	}
	bandwidthMust(t, e.StopTorrent(ih))
	if e.GetTorrents()[ih].SeedingOverride {
		t.Fatal("Stop did not end override")
	}
}
func TestSeedingAccountingRestartAndUploadControls(t *testing.T) {
	var seconds atomic.Int64
	state := t.TempDir()
	e, ih, dir := selectedSeedFixture(t, &seconds, state)
	seconds.Store(45)
	if e.GetTorrents()[ih].SeedSeconds != 45 {
		t.Fatal("missing time")
	}
	bandwidthMust(t, e.Close())
	seconds.Store(1000)
	restored := seedTestEngine(t, dir, state, &seconds, Config{EnableUpload: true, EnableSeeding: true, SeedTimeMinutes: 1})
	waitSessionBytes(t, restored, ih, 16)
	if restored.GetTorrents()[ih].SeedSeconds != 45 {
		t.Fatal("restart reset time or included downtime")
	}
	seconds.Store(1015)
	if restored.GetTorrents()[ih].StopReason != "seed-time" {
		t.Fatal("restart reset budget")
	}
	beforeFile, err := os.ReadFile(filepath.Join(state, "sessions.json"))
	bandwidthMust(t, err)
	bandwidthMust(t, e.Close()) // an earlier owner must not write after releasing the lock
	afterFile, err := os.ReadFile(filepath.Join(state, "sessions.json"))
	bandwidthMust(t, err)
	if !bytes.Equal(beforeFile, afterFile) {
		t.Fatal("repeated Close overwrote another owner's session")
	}
	bandwidthMust(t, restored.ResumeSeeding(ih))
	bandwidthMust(t, restored.Close())
	restored = seedTestEngine(t, dir, state, &seconds, Config{EnableUpload: true, EnableSeeding: true, SeedTimeMinutes: 1})
	waitSessionBytes(t, restored, ih, 16)
	if !restored.GetTorrents()[ih].SeedingOverride {
		t.Fatal("restart silently removed explicit override")
	}
	c := restored.Config()
	c.EnableUpload = false
	bandwidthMust(t, restored.Configure(c))
	waitSessionBytes(t, restored, ih, 16)
	before := restored.GetTorrents()[ih].SeedSeconds
	seconds.Add(3600)
	if model := restored.GetTorrents()[ih]; model.State != "completed" || model.SeedSeconds != before {
		t.Fatal("upload-disabled time accrued", model)
	}
	c.EnableUpload = true
	c.EnableSeeding = false
	bandwidthMust(t, restored.Configure(c))
	waitSessionBytes(t, restored, ih, 16)
	before = restored.GetTorrents()[ih].SeedSeconds
	seconds.Add(3600)
	if model := restored.GetTorrents()[ih]; model.State != "completed" || model.SeedSeconds != before {
		t.Fatal("seeding-disabled time accrued", model)
	}
}
func TestSeedingRatioFromRealPeerUploadAndRestart(t *testing.T) {
	data, specs := bandwidthSpecs(t)
	dir, state := t.TempDir(), t.TempDir()
	bandwidthMust(t, os.WriteFile(filepath.Join(dir, "bandwidth-0.bin"), data, 0600))
	var seconds atomic.Int64
	seed := seedTestEngine(t, dir, state, &seconds, Config{EnableUpload: true, EnableSeeding: true, UploadLimitKiB: 64, SeedRatioLimit: 0.125})
	spec := specs[0]
	ih := spec.InfoHash.HexString()
	bandwidthMust(t, seed.NewTorrent(spec))
	waitSessionBytes(t, seed, ih, int64(len(data)))
	bandwidthMust(t, seed.StartTorrent(ih))
	leecher := isolatedBandwidthEngine(t, t.TempDir(), 0, 0, false)
	bandwidthMust(t, leecher.NewTorrent(spec))
	bandwidthMust(t, leecher.StartTorrent(ih))
	leecher.mut.Lock()
	leecher.ts[ih].t.AddClientPeer(seed.client)
	leecher.mut.Unlock()
	deadline := time.Now().Add(6 * time.Second)
	var model *Torrent
	for time.Now().Before(deadline) {
		model = seed.GetTorrents()[ih]
		if model.StopReason == "seed-ratio" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if model == nil || model.StopReason != "seed-ratio" || model.Started || model.SeedRatio < 0.125 || math.IsNaN(model.SeedRatio) || math.IsInf(model.SeedRatio, 0) {
		t.Fatal("real upload did not stop at ratio", model)
	}
	time.Sleep(300 * time.Millisecond)
	uploaded := seed.GetTorrents()[ih].Uploaded
	if uploaded < int64(float64(len(data))*0.125) {
		t.Fatal("upload accounting ignored real payload", uploaded)
	}
	bandwidthMust(t, seed.Close())
	restored := seedTestEngine(t, dir, state, &seconds, Config{EnableUpload: true, EnableSeeding: true, SeedRatioLimit: 0.125})
	waitSessionBytes(t, restored, ih, int64(len(data)))
	model = restored.GetTorrents()[ih]
	if model.Uploaded != uploaded || model.StopReason != "seed-ratio" || model.Started {
		t.Fatal("restart lost policy/accounting", model)
	}
}
func TestSeedingDisabledZeroDenominatorAndInvalidValues(t *testing.T) {
	var seconds atomic.Int64
	e := seedTestEngine(t, t.TempDir(), "", &seconds, Config{EnableUpload: true, EnableSeeding: true, SeedRatioLimit: 1})
	info := metainfo.Info{Name: "empty", PieceLength: 16 << 10}
	encoded, err := bencode.Marshal(info)
	bandwidthMust(t, err)
	spec := &torrent.TorrentSpec{InfoHash: metainfo.HashBytes(encoded), InfoBytes: encoded}
	bandwidthMust(t, e.NewTorrent(spec))
	bandwidthMust(t, e.StartTorrent(spec.InfoHash.HexString()))
	model := e.GetTorrents()[spec.InfoHash.HexString()]
	if math.IsNaN(model.SeedRatio) || math.IsInf(model.SeedRatio, 0) || model.StopReason != "" {
		t.Fatal("zero denominator unsafe", model)
	}
	c := e.Config()
	c.SeedRatioLimit = 0
	bandwidthMust(t, e.Configure(c))
	seconds.Store(86400)
	if e.GetTorrents()[spec.InfoHash.HexString()].StopReason != "" {
		t.Fatal("disabled limit stopped sharing")
	}
	before := e.Config()
	for _, ratio := range []float64{-1, 1001, math.NaN(), math.Inf(1)} {
		c := before
		c.SeedRatioLimit = ratio
		if e.Configure(c) == nil {
			t.Fatal("invalid ratio accepted")
		}
		if e.Config() != before {
			t.Fatal("invalid ratio changed settings")
		}
	}
	for _, minutes := range []int{-1, 525601} {
		c := before
		c.SeedTimeMinutes = minutes
		if e.Configure(c) == nil {
			t.Fatal("invalid time accepted")
		}
	}
	for _, key := range []string{"SeedRatioLimit", "SeedTimeMinutes"} {
		for _, raw := range []string{"null", "-1", `"bad"`, "1e100"} {
			c := before
			if json.Unmarshal([]byte(fmt.Sprintf(`{%q:%s}`, key, raw)), &c) == nil {
				t.Fatal("invalid budget JSON accepted", key, raw)
			}
		}
	}
}

func TestSeedingStateFailurePausesAndCanRecover(t *testing.T) {
	var seconds atomic.Int64
	state := t.TempDir()
	e, ih, _ := selectedSeedFixture(t, &seconds, state)
	e.mut.Lock()
	e.sessionDir = filepath.Join(state, "missing")
	e.mut.Unlock()
	seconds.Store(1)
	model := e.GetTorrents()[ih]
	if model.Started || model.State != "paused" || model.StopReason != "state-unavailable" {
		t.Fatal("unpersisted accounting did not pause sharing", model)
	}
	e.mut.Lock()
	e.sessionDir = state
	e.mut.Unlock()
	bandwidthMust(t, e.StartFile(ih, "restart-fixture/one"))
	if model = e.GetTorrents()[ih]; !model.Started || model.StopReason != "" {
		t.Fatal("repaired state store could not resume", model)
	}
}

func TestSeedingExhaustedTimeDoesNotResumeDuringRestartVerification(t *testing.T) {
	var seconds atomic.Int64
	state := t.TempDir()
	e, ih, dir := selectedSeedFixture(t, &seconds, state)
	seconds.Store(60) // shutdown accounts time without a status poll first
	bandwidthMust(t, e.Close())
	seconds.Store(1000)
	restored := seedTestEngine(t, dir, state, &seconds, Config{EnableUpload: true, EnableSeeding: true, SeedTimeMinutes: 1})
	model := restored.GetTorrents()[ih]
	if model.Started || model.StopReason != "seed-time" || model.SeedSeconds != 60 {
		t.Fatal("exhausted sharing resumed during verification", model)
	}
}
