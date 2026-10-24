package engine

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
)

func assertFiniteTelemetry(t *testing.T, model *Torrent) {
	t.Helper()
	for _, rate := range []float64{float64(model.DownloadRate), float64(model.UploadRate), model.SeedRatio} {
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
			t.Fatalf("invalid telemetry: %+v", model)
		}
	}
	if model.NetworkDownloaded < 0 || model.UsefulDownloaded < 0 || model.UsefulDownloaded > model.NetworkDownloaded || model.Uploaded < 0 || model.ActivePeers < 0 || model.KnownPeers < model.ActivePeers {
		t.Fatalf("invalid counters: %+v", model)
	}
	if _, err := json.Marshal(model); err != nil {
		t.Fatal("nonserializable telemetry", err)
	}
}
func TestTransferSamplingGuardsAndRepeatedProjection(t *testing.T) {
	model := &Torrent{}
	now := time.Now()
	var stats torrent.TorrentStats
	model.updateTransfers(stats, now)
	stats.BytesReadData.Add(1024)
	stats.BytesReadUsefulData.Add(512)
	stats.BytesWrittenData.Add(256)
	stats.ActivePeers = 1
	stats.TotalPeers = 2
	model.updateTransfers(stats, now.Add(time.Second))
	if model.DownloadRate != 1024 || model.UploadRate != 256 || model.UsefulDownloaded != 512 || model.ActivePeers != 1 || model.KnownPeers != 2 {
		t.Fatalf("sample: %+v", model)
	}
	model.updateTransfers(stats, now.Add(time.Second+time.Microsecond))
	if model.DownloadRate != 1024 || model.UploadRate != 256 {
		t.Fatal("duplicate projection erased rates")
	}
	model.updateTransfers(stats, now.Add(2*time.Second))
	if model.DownloadRate != 0 || model.UploadRate != 0 {
		t.Fatal("idle rate did not decay")
	}
	model.updateTransfers(stats, now)
	assertFiniteTelemetry(t, model)
	model.updateTransfers(stats, now)
	assertFiniteTelemetry(t, model)
	// Upstream reset/invalid deltas never produce negative or infinite values.
	stats.BytesReadData.Add(-2048)
	stats.BytesWrittenData.Add(-512)
	stats.ActivePeers = -1
	stats.TotalPeers = -2
	model.updateTransfers(stats, now.Add(time.Second))
	assertFiniteTelemetry(t, model)
	for _, duration := range []time.Duration{0, -time.Second, time.Nanosecond} {
		rate := transferRate(math.MaxInt64, duration)
		if rate < 0 || math.IsInf(float64(rate), 0) || math.IsNaN(float64(rate)) {
			t.Fatal("unsafe rate", rate)
		}
	}
	for _, pair := range [][2]int64{{-1, 1}, {1, -1}, {math.MaxInt64, 1}, {math.MaxInt64, math.MaxInt64}} {
		value := percent(pair[0], pair[1])
		if value < 0 || value > 100 {
			t.Fatal("unsafe percent", value)
		}
	}
}
func TestTransferTelemetryWithLocalPeersAndRestart(t *testing.T) {
	data, specs := bandwidthSpecs(t)
	spec := specs[0]
	ih := spec.InfoHash.HexString()
	dir, state := t.TempDir(), t.TempDir()
	bandwidthMust(t, os.WriteFile(filepath.Join(dir, "bandwidth-0.bin"), data, 0600))
	seed := isolatedBandwidthEngine(t, dir, 128, 0, true, state)
	bandwidthMust(t, seed.NewTorrent(spec))
	waitSessionBytes(t, seed, ih, int64(len(data)))
	bandwidthMust(t, seed.StartTorrent(ih))
	initial := seed.GetTorrents()[ih]
	assertFiniteTelemetry(t, initial)
	if initial.NetworkDownloaded != 0 || initial.UsefulDownloaded != 0 || initial.Downloaded != int64(len(data)) || initial.State != "seeding" {
		t.Fatalf("existing content counted as network traffic: %+v", initial)
	}
	leecher := isolatedBandwidthEngine(t, t.TempDir(), 0, 128, false)
	bandwidthMust(t, leecher.NewTorrent(spec))
	bandwidthMust(t, leecher.StartTorrent(ih))
	leecher.mut.Lock()
	leecher.ts[ih].t.AddClientPeer(seed.client)
	leecher.mut.Unlock()
	var sawUp, sawDown, sawPeers bool
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		up, down := seed.GetTorrents()[ih], leecher.GetTorrents()[ih]
		assertFiniteTelemetry(t, up)
		assertFiniteTelemetry(t, down)
		sawUp = sawUp || up.UploadRate > 0
		sawDown = sawDown || down.DownloadRate > 0
		sawPeers = sawPeers || (up.ActivePeers > 0 && down.ActivePeers > 0)
		if down.Downloaded == int64(len(data)) && sawUp && sawDown && sawPeers {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	up, down := seed.GetTorrents()[ih], leecher.GetTorrents()[ih]
	if !sawUp || !sawDown || !sawPeers || down.Downloaded != int64(len(data)) || down.State != "completed" || up.State != "seeding" {
		t.Fatalf("local transfer telemetry missing: up=%+v down=%+v rates=%v/%v peers=%v", up, down, sawUp, sawDown, sawPeers)
	}
	if down.NetworkDownloaded < int64(len(data)) || down.UsefulDownloaded < int64(len(data)) || up.Uploaded < down.UsefulDownloaded {
		t.Fatalf("payload counters inconsistent: up=%d received=%d useful=%d", up.Uploaded, down.NetworkDownloaded, down.UsefulDownloaded)
	}
	bandwidthMust(t, leecher.StopTorrent(ih))
	bandwidthMust(t, seed.StopTorrent(ih))
	for _, e := range []*Engine{seed, leecher} {
		model := e.GetTorrents()[ih]
		assertFiniteTelemetry(t, model)
		if model.State != "paused" || model.DownloadRate != 0 || model.UploadRate != 0 {
			t.Fatalf("pause retained activity: %+v", model)
		}
	}
	// Serialized snapshots contain only summaries, not raw stats or peer origins.
	encoded, err := json.Marshal(seed.GetTorrents()[ih])
	bandwidthMust(t, err)
	for _, secret := range []string{"127.0.0.1", "PeerConns", "WebSeeds", "InfoBytes", "Trackers", "Sources", "BytesWrittenData"} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatal("snapshot leaked raw transport details", secret)
		}
	}
	uploaded := seed.GetTorrents()[ih].Uploaded
	bandwidthMust(t, seed.Close())
	restored := isolatedBandwidthEngine(t, dir, 0, 0, true, state)
	waitSessionBytes(t, restored, ih, int64(len(data)))
	model := restored.GetTorrents()[ih]
	assertFiniteTelemetry(t, model)
	if model.Uploaded != uploaded || model.NetworkDownloaded != 0 || model.UsefulDownloaded != 0 || model.UploadRate != 0 || model.DownloadRate != 0 || model.State != "paused" {
		t.Fatalf("restart scope inconsistent: %+v", model)
	}
	if strings.Contains(string(encoded), "NaN") {
		t.Fatal("unsafe serialized telemetry")
	}
}
