package engine

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

func queueSpecs(t *testing.T) ([]byte, []*torrent.TorrentSpec) {
	t.Helper()
	data := bytes.Repeat([]byte("local queue legal fixture bytes!"), 32768)
	var pieces []byte
	for start := 0; start < len(data); start += 16 << 10 {
		hash := sha1.Sum(data[start:min(start+(16<<10), len(data))])
		pieces = append(pieces, hash[:]...)
	}
	var specs []*torrent.TorrentSpec
	for i := range 4 {
		info := metainfo.Info{Name: fmt.Sprintf("queue-%d.bin", i), Length: int64(len(data)), PieceLength: 16 << 10, Pieces: pieces}
		encoded, err := bencode.Marshal(info)
		bandwidthMust(t, err)
		specs = append(specs, &torrent.TorrentSpec{InfoHash: metainfo.HashBytes(encoded), InfoBytes: encoded})
	}
	return data, specs
}
func queueState(t *testing.T, e *Engine, specs []*torrent.TorrentSpec, expected ...string) {
	t.Helper()
	snapshot := e.GetTorrents()
	for i, state := range expected {
		model := snapshot[specs[i].InfoHash.HexString()]
		if state == "removed" {
			if model != nil {
				t.Fatal("removed torrent retained")
			}
			continue
		}
		if model == nil || model.State != state {
			t.Fatalf("torrent %d: %+v, want %s", i, model, state)
		}
	}
	active := 0
	for _, model := range snapshot {
		if model.State == "downloading" {
			active++
		}
	}
	if limit := e.Config().MaxActiveDownloads; limit > 0 && active > limit {
		t.Fatal("download capacity exceeded", active, limit)
	}
}
func TestQueueFIFOWithRealTransfersAndSeeding(t *testing.T) {
	data, specs := queueSpecs(t)
	requests := map[string]bool{}
	var mu sync.Mutex
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.Path] = true
		mu.Unlock()
		http.ServeContent(w, r, "fixture", time.Time{}, bytes.NewReader(data))
	}))
	defer origin.Close()
	e := isolatedBandwidthEngine(t, t.TempDir(), 0, 64, true)
	c := e.Config()
	c.AutoStart = true
	c.MaxActiveDownloads = 2
	bandwidthMust(t, e.Configure(c))
	for i, spec := range specs {
		spec.Webseeds = []string{fmt.Sprintf("%s/queue-%d.bin", origin.URL, i)}
		bandwidthMust(t, e.NewTorrent(spec))
	}
	queueState(t, e, specs, "downloading", "downloading", "queued", "queued")
	snapshot := e.GetTorrents()
	if snapshot[specs[2].InfoHash.HexString()].QueuePosition != 1 || snapshot[specs[3].InfoHash.HexString()].QueuePosition != 2 {
		t.Fatal("non-FIFO queue positions")
	}
	// Both admitted torrents really receive local bytes; queued torrents have no
	// requests or completed payload. The cap keeps the fixtures incomplete.
	deadline := time.Now().Add(9 * time.Second)
	for time.Now().Before(deadline) {
		snapshot = e.GetTorrents()
		if snapshot[specs[0].InfoHash.HexString()].Downloaded > 0 && snapshot[specs[1].InfoHash.HexString()].Downloaded > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, i := range []int{0, 1} {
		if snapshot[specs[i].InfoHash.HexString()].Downloaded == 0 {
			t.Fatal("admitted fixture never transferred", i)
		}
	}
	mu.Lock()
	queuedRequested := requests["/queue-2.bin"] || requests["/queue-3.bin"]
	mu.Unlock()
	if queuedRequested {
		t.Fatal("queued payload requested")
	}
	bandwidthMust(t, e.StopTorrent(specs[0].InfoHash.HexString()))
	queueState(t, e, specs, "paused", "downloading", "downloading", "queued")
	bandwidthMust(t, e.DeleteTorrent(specs[1].InfoHash.HexString()))
	queueState(t, e, specs, "paused", "removed", "downloading", "downloading")
	bandwidthMust(t, e.StartTorrent(specs[0].InfoHash.HexString()))
	queueState(t, e, specs, "queued", "removed", "downloading", "downloading")
	bandwidthMust(t, e.StopTorrent(specs[3].InfoHash.HexString()))
	c.MaxActiveDownloads = 1
	bandwidthMust(t, e.Configure(c))
	queueState(t, e, specs, "queued", "removed", "downloading", "paused")
	bandwidthMust(t, e.NewTorrent(specs[0]))
	queueState(t, e, specs, "queued", "removed", "downloading", "paused")
	c.DownloadLimitKiB = 0
	bandwidthMust(t, e.Configure(c))
	deadline = time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		snapshot = e.GetTorrents()
		if snapshot[specs[2].InfoHash.HexString()].State == "seeding" && snapshot[specs[0].InfoHash.HexString()].State == "seeding" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	queueState(t, e, specs, "seeding", "removed", "seeding", "paused")
}
func TestQueueMetadataAdmissionAndConcurrentMutations(t *testing.T) {
	e := isolatedBandwidthEngine(t, t.TempDir(), 0, 0, false)
	c := e.Config()
	c.AutoStart = true
	c.MaxActiveDownloads = 2
	c.MaxActiveMetadata = 2
	bandwidthMust(t, e.Configure(c))
	for i := 1; i <= 10; i++ {
		bandwidthMust(t, e.NewMagnet(fmt.Sprintf("magnet:?xt=urn:btih:%040x", i)))
	}
	check := func(want int) {
		t.Helper()
		e.GetTorrents()
		e.mut.Lock()
		defer e.mut.Unlock()
		actual := 0
		for _, model := range e.ts {
			if model.t != nil && !model.Loaded {
				actual++
			}
		}
		if actual != want {
			t.Fatalf("metadata handles %d, want %d", actual, want)
		}
	}
	check(2)
	bandwidthMust(t, e.StopTorrent(fmt.Sprintf("%040x", 1)))
	check(2)
	bandwidthMust(t, e.DeleteTorrent(fmt.Sprintf("%040x", 2)))
	check(2)
	c.MaxActiveMetadata = 1
	bandwidthMust(t, e.Configure(c))
	check(1)
	if e.GetTorrents()[fmt.Sprintf("%040x", 1)].State != "paused" {
		t.Fatal("capacity change resumed paused metadata")
	}
	var wg sync.WaitGroup
	for i := 3; i <= 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = e.StopTorrent(fmt.Sprintf("%040x", i))
			_ = e.StartTorrent(fmt.Sprintf("%040x", i))
			_ = e.NewMagnet(fmt.Sprintf("magnet:?xt=urn:btih:%040x", i))
			e.GetTorrents()
		}(i)
	}
	wg.Wait()
	check(1)
	if len(e.GetTorrents()) != 9 {
		t.Fatal("duplicate entries admitted")
	}
	for i := 11; i <= maxSessionTorrents+1; i++ {
		bandwidthMust(t, e.NewMagnet(fmt.Sprintf("magnet:?xt=urn:btih:%040x", i)))
	}
	if e.NewMagnet(fmt.Sprintf("magnet:?xt=urn:btih:%040x", maxSessionTorrents+2)) == nil {
		t.Fatal("pending queue is unbounded")
	}
	check(1)
}
func TestQueueProcessRestart(t *testing.T) {
	if dir := os.Getenv("CT_QUEUE_TEST_DIR"); dir != "" {
		e := New()
		bandwidthMust(t, e.EnablePersistence(filepath.Join(dir, "state")))
		c := Config{DownloadDirectory: filepath.Join(dir, "downloads"), IncomingPort: freePort(t), AutoStart: true, MaxActiveDownloads: 2, MaxActiveMetadata: 2}
		bandwidthMust(t, e.Configure(c))
		_, specs := queueSpecs(t)
		if os.Getenv("CT_QUEUE_TEST_STAGE") == "write" {
			for _, spec := range specs {
				bandwidthMust(t, e.NewTorrent(spec))
			}
			bandwidthMust(t, e.StopTorrent(specs[0].InfoHash.HexString()))
		}
		queueState(t, e, specs, "paused", "downloading", "downloading", "queued")
		if e.GetTorrents()[specs[3].InfoHash.HexString()].QueuePosition != 1 {
			t.Fatal("lost persisted queue order")
		}
		return
	}
	dir := t.TempDir()
	for _, stage := range []string{"write", "restore"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestQueueProcessRestart$", "-test.timeout=20s")
		cmd.Env = append(os.Environ(), "CT_QUEUE_TEST_DIR="+dir, "CT_QUEUE_TEST_STAGE="+stage)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", stage, err, out)
		}
	}
}

func TestQueueCapacityValidationIsAtomic(t *testing.T) {
	e := isolatedBandwidthEngine(t, t.TempDir(), 0, 0, false)
	before := e.Config()
	client := e.client
	for _, value := range []int{-1, 257} {
		c := before
		c.MaxActiveDownloads = value
		if e.Configure(c) == nil {
			t.Fatal("invalid capacity accepted")
		}
		if e.client != client || e.Config() != before {
			t.Fatal("invalid capacity changed engine")
		}
	}
	for _, value := range []int{-1, 65} {
		c := before
		c.MaxActiveMetadata = value
		if e.Configure(c) == nil {
			t.Fatal("invalid metadata capacity accepted")
		}
	}
	for _, key := range []string{"MaxActiveDownloads", "MaxActiveMetadata"} {
		c := before
		if json.Unmarshal([]byte(fmt.Sprintf(`{%q:null}`, key)), &c) == nil {
			t.Fatal("null capacity accepted")
		}
		if c != before {
			t.Fatal("invalid JSON changed config")
		}
	}
}

func TestQueueHybridIdentitiesShareEntryAndSlot(t *testing.T) {
	spec := queueHybridSpec(t)
	e := isolatedBandwidthEngine(t, t.TempDir(), 0, 0, false)
	first := fmt.Sprintf("magnet:?xt=urn:btih:%s", spec.InfoHash.HexString())
	second := fmt.Sprintf("magnet:?xt=urn:btmh:1220%x", spec.InfoHashV2.Value)
	bandwidthMust(t, e.NewMagnet(first))
	bandwidthMust(t, e.StartTorrent(spec.InfoHash.HexString()))
	bandwidthMust(t, e.NewMagnet(second)) // remains paused; metadata will prove the alias
	bandwidthMust(t, e.NewTorrent(spec))
	if snapshot := e.GetTorrents(); len(snapshot) != 1 {
		t.Fatal("hybrid aliases have duplicate queue slots", len(snapshot))
	}
	if e.GetTorrents()[spec.InfoHash.HexString()].Started {
		t.Fatal("alias consolidation lost explicit pause")
	}
	bandwidthMust(t, e.StartTorrent(spec.InfoHash.HexString()))
	bandwidthMust(t, e.NewMagnet(second))
	if len(e.GetTorrents()) != 1 {
		t.Fatal("resolved v2 magnet duplicated hybrid")
	}
}

func queueHybridSpec(t *testing.T) *torrent.TorrentSpec {
	t.Helper()
	data := []byte("a")
	v1Piece := sha1.Sum(data)
	v2Piece := sha256.Sum256(data)
	info := metainfo.Info{Name: "hybrid.bin", Length: 1, PieceLength: 16 << 10, Pieces: v1Piece[:], MetaVersion: 2, FileTree: metainfo.FileTree{Dir: map[string]metainfo.FileTree{"hybrid.bin": {File: metainfo.FileTreeFile{Length: 1, PiecesRoot: string(v2Piece[:])}}}}}
	encoded, err := bencode.Marshal(&info)
	bandwidthMust(t, err)
	spec, err := torrent.TorrentSpecFromMetaInfoErr(&metainfo.MetaInfo{InfoBytes: encoded})
	bandwidthMust(t, err)
	return spec
}
func TestQueueV2FirstHybridRestoresStableIdentity(t *testing.T) {
	spec := queueHybridSpec(t)
	stateDir, downloadDir := t.TempDir(), t.TempDir()
	e := isolatedBandwidthEngine(t, downloadDir, 0, 0, false, stateDir)
	magnet := fmt.Sprintf("magnet:?xt=urn:btmh:1220%x", spec.InfoHashV2.Value[:])
	parsed, err := torrent.TorrentSpecFromMagnetUri(magnet)
	bandwidthMust(t, err)
	key := specIdentity(parsed)
	bandwidthMust(t, e.NewMagnet(magnet))
	bandwidthMust(t, e.StartTorrent(key))
	bandwidthMust(t, e.NewTorrent(spec))
	snapshot := e.GetTorrents()
	if len(snapshot) != 1 || snapshot[key] == nil {
		t.Fatal("hybrid promotion changed model identity or duplicated it")
	}
	bandwidthMust(t, e.Close())
	restored := isolatedBandwidthEngine(t, downloadDir, 0, 0, false, stateDir)
	if snapshot = restored.GetTorrents(); len(snapshot) != 1 || snapshot[key] == nil || !snapshot[key].Started {
		t.Fatal("v2-first hybrid session lost on restart")
	}
}
