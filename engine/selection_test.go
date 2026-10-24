package engine

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

const skipFirstSize = 2*1024*1024 + 8192
const skipPieceSize = 64 * 1024

func skipFixtureSpec(t *testing.T) ([]byte, *torrent.TorrentSpec) {
	t.Helper()
	data := bytes.Repeat([]byte("0123456789abcdef"), (skipFirstSize+2*1024*1024)/16)
	var pieces []byte
	for start := 0; start < len(data); start += skipPieceSize {
		hash := sha1.Sum(data[start:min(start+skipPieceSize, len(data))])
		pieces = append(pieces, hash[:]...)
	}
	info := metainfo.Info{Name: "skip-fixture", PieceLength: skipPieceSize, Pieces: pieces, Files: []metainfo.FileInfo{{Path: []string{"first.bin"}, Length: skipFirstSize}, {Path: []string{"second.bin"}, Length: int64(len(data) - skipFirstSize)}}}
	encoded, err := bencode.Marshal(info)
	bandwidthMust(t, err)
	return data, &torrent.TorrentSpec{InfoHash: metainfo.HashBytes(encoded), InfoBytes: encoded}
}
func TestSkipSharedBoundaryKeepsSiblingTransferringAndProtectsDeletion(t *testing.T) {
	data, spec := skipFixtureSpec(t)
	var skipped atomic.Bool
	var mu sync.Mutex
	var postSkipFirstStarts []int
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var content []byte
		switch r.URL.Path {
		case "/skip-fixture/first.bin":
			content = data[:skipFirstSize]
			if skipped.Load() {
				start, end := 0, 0
				fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &start, &end)
				mu.Lock()
				postSkipFirstStarts = append(postSkipFirstStarts, start)
				mu.Unlock()
			}
		case "/skip-fixture/second.bin":
			content = data[skipFirstSize:]
		default:
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "fixture", time.Time{}, bytes.NewReader(content))
	}))
	t.Cleanup(origin.Close)
	spec.Webseeds = []string{origin.URL + "/"}
	e := isolatedBandwidthEngine(t, t.TempDir(), 0, 64, false)
	bandwidthMust(t, e.NewTorrent(spec))
	ih := spec.InfoHash.HexString()
	bandwidthMust(t, e.StartTorrent(ih))
	waitTransferProgress(t, e, ih, 0)
	ready := time.Now().Add(15 * time.Second)
	for time.Now().Before(ready) {
		if e.GetTorrents()[ih].Files[0].Percent > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	initial := e.GetTorrents()[ih]
	if initial.Files[0].Percent == 0 || initial.Files[0].Percent == 100 || initial.Files[1].Percent == 100 {
		t.Fatal("fixture must have partial first file and incomplete sibling", initial)
	}
	siblingBefore := initial.Files[1].Percent
	bandwidthMust(t, e.StopFile(ih, "skip-fixture/first.bin"))
	skipped.Store(true)
	before := e.GetTorrents()[ih]
	if before.Files[0].Selected || before.Files[0].Started || !before.Files[1].Selected || !before.Started {
		t.Fatal("skip state inconsistent", before)
	}
	// A shared boundary may still be requested, but unique first-file pieces
	// must no longer be scheduled. Selected siblings keep making real progress.
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		model := e.GetTorrents()[ih]
		if model.Files[1].Percent > siblingBefore {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if e.GetTorrents()[ih].Files[1].Percent <= siblingBefore {
		e.mut.Lock()
		for _, file := range e.ts[ih].Files {
			t.Logf("file %s selected=%t started=%t percent=%f priority=%v", file.Path, file.Selected, file.Started, file.Percent, file.f.Priority())
		}
		e.mut.Unlock()
		var debug bytes.Buffer
		e.client.WriteStatus(&debug)
		t.Log(debug.String())
		t.Fatal("skipping first file stopped its sibling")
	}
	mu.Lock()
	starts := append([]int{}, postSkipFirstStarts...)
	mu.Unlock()
	for _, start := range starts {
		if start < skipFirstSize/skipPieceSize*skipPieceSize {
			t.Fatal("unique skipped pieces requested after skip", start)
		}
	}
	root, err := os.OpenRoot(e.Config().DownloadDirectory)
	bandwidthMust(t, err)
	defer root.Close()
	for _, path := range []string{"skip-fixture/first.bin.part", "skip-fixture"} {
		if err := e.RemoveDownload(root, path); !errors.Is(err, ErrActiveDownload) {
			t.Fatalf("shared-boundary deletion %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(e.Config().DownloadDirectory, "skip-fixture", "first.bin.part")); err != nil {
		t.Fatal("Skip deleted data", err)
	}
	for _, path := range []string{"../first.bin", "/skip-fixture/first.bin", "skip-fixture/./second.bin", "skip-fixture//second.bin", "unknown", `skip-fixture\second.bin`} {
		if e.StopFile(ih, path) == nil || e.StartFile(ih, path) == nil {
			t.Fatal("aliased/escaping path accepted", path)
		}
	}
	bandwidthMust(t, e.StopFile(ih, "skip-fixture/second.bin"))
	if model := e.GetTorrents()[ih]; model.Started || model.State != "paused" || model.Files[0].Selected || model.Files[1].Selected {
		t.Fatal("all-skipped torrent remained active", model)
	}
	if e.ResumeTorrent(ih) == nil {
		t.Fatal("empty selection resumed")
	}
	bandwidthMust(t, e.StartFile(ih, "skip-fixture/second.bin"))
	c := e.Config()
	c.EnableUpload = true
	bandwidthMust(t, e.Configure(c))
	model := e.GetTorrents()[ih]
	if model.Files[0].Selected || !model.Files[1].Selected {
		t.Fatal("reconfigure lost Skip")
	}
	bandwidthMust(t, e.StopTorrent(ih))
	bandwidthMust(t, e.ResumeTorrent(ih))
	model = e.GetTorrents()[ih]
	if model.Files[0].Started || !model.Files[1].Started {
		t.Fatal("Resume selected implicitly selected skipped file")
	}
	c.DownloadLimitKiB = 0
	bandwidthMust(t, e.Configure(c))
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		model = e.GetTorrents()[ih]
		if model.Files[1].Percent == 100 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if model.Files[1].Percent != 100 || model.Files[0].Percent == 100 {
		t.Fatal("selected sibling did not finish independently", model)
	}
	bandwidthMust(t, e.StartTorrent(ih)) // explicit Start all reselects the skipped file
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		model = e.GetTorrents()[ih]
		if model.Percent == 100 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if model.Percent != 100 || !model.Files[0].Selected {
		t.Fatal("reselection failed to finish content", model)
	}
}
func TestSkipSelectionProcessRestart(t *testing.T) {
	if dir := os.Getenv("CT_SKIP_TEST_DIR"); dir != "" {
		_, spec := skipFixtureSpec(t)
		e := New()
		e.newClient = func(cfg *torrent.ClientConfig) (*torrent.Client, error) {
			cfg.NoDHT = true
			cfg.DisableTrackers = true
			cfg.NoDefaultPortForwarding = true
			cfg.DisablePEX = true
			cfg.DisableUTP = true
			cfg.DisableWebtorrent = true
			return torrent.NewClient(cfg)
		}
		bandwidthMust(t, e.EnablePersistence(filepath.Join(dir, "state")))
		c := Config{DownloadDirectory: filepath.Join(dir, "downloads"), IncomingPort: freePort(t), AutoStart: true}
		bandwidthMust(t, e.Configure(c))
		ih := spec.InfoHash.HexString()
		switch os.Getenv("CT_SKIP_TEST_STAGE") {
		case "write":
			bandwidthMust(t, e.NewTorrent(spec))
			bandwidthMust(t, e.StopFile(ih, "skip-fixture/first.bin"))
			bandwidthMust(t, e.StopTorrent(ih))
		case "resume":
			model := e.GetTorrents()[ih]
			if model.Started || model.Files[0].Selected || !model.Files[1].Selected {
				t.Fatal("paused selection lost on process restart", model)
			}
			bandwidthMust(t, e.ResumeTorrent(ih))
			model = e.GetTorrents()[ih]
			if model.Files[0].Started || !model.Files[1].Started {
				t.Fatal("resume forgot skipped file")
			}
			bandwidthMust(t, e.StopFile(ih, "skip-fixture/second.bin"))
		case "all-skipped":
			model := e.GetTorrents()[ih]
			if model.State != "paused" || model.Files[0].Selected || model.Files[1].Selected {
				t.Fatal("all-skipped intent lost on restart", model)
			}
			bandwidthMust(t, e.StartFile(ih, "skip-fixture/first.bin"))
			if !e.GetTorrents()[ih].Files[0].Selected {
				t.Fatal("reselection failed")
			}
		}
		return
	}
	dir := t.TempDir()
	for _, stage := range []string{"write", "resume", "all-skipped"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSkipSelectionProcessRestart$", "-test.timeout=20s")
		cmd.Env = append(os.Environ(), "CT_SKIP_TEST_DIR="+dir, "CT_SKIP_TEST_STAGE="+stage)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", stage, err, strings.TrimSpace(string(out)))
		}
	}
}
