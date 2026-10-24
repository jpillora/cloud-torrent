package engine

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

func sessionFixture(t *testing.T) *torrent.TorrentSpec {
	t.Helper()
	info := metainfo.Info{Name: "restart-fixture", PieceLength: 16, Files: []metainfo.FileInfo{{Path: []string{"one"}, Length: 16}, {Path: []string{"two"}, Length: 16}}, Pieces: make([]byte, 40)}
	hash := sha1.Sum(bytes.Repeat([]byte("a"), 16))
	info.Pieces = append(append([]byte{}, hash[:]...), hash[:]...)
	data, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	return &torrent.TorrentSpec{InfoHash: metainfo.HashBytes(data), InfoBytes: data}
}

// Each stage is a separate process. The writer exits without Engine.Close,
// proving successful actions are durable before graceful shutdown.
func TestSessionProcessRestart(t *testing.T) {
	if dir := os.Getenv("CT_SESSION_TEST_DIR"); dir != "" {
		e := New()
		if err := e.EnablePersistence(filepath.Join(dir, "state")); err != nil {
			t.Fatal(err)
		}
		c := Config{DownloadDirectory: filepath.Join(dir, "downloads"), IncomingPort: freePort(t)}
		if err := e.Configure(c); err != nil {
			t.Fatal(err)
		}
		spec := sessionFixture(t)
		ih := spec.InfoHash.HexString()
		pending := "magnet:?xt=urn:btih:1111111111111111111111111111111111111111"
		stage := os.Getenv("CT_SESSION_TEST_STAGE")
		switch stage {
		case "write":
			fixtureDir := filepath.Join(dir, "downloads", "restart-fixture")
			if err := os.MkdirAll(fixtureDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(fixtureDir, "one"), bytes.Repeat([]byte("a"), 16), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(fixtureDir, "two"), []byte("a"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := e.NewTorrent(spec); err != nil {
				t.Fatal(err)
			}
			if err := e.StartFile(ih, "restart-fixture/one"); err != nil {
				t.Fatal(err)
			}
			if err := e.NewMagnet(pending); err != nil {
				t.Fatal(err)
			}
			if err := e.StartTorrent(strings.Repeat("1", 40)); err != nil {
				t.Fatal(err)
			}
		case "restore":
			waitSessionBytes(t, e, ih, 16)
			snapshot := e.GetTorrents()
			if len(snapshot) != 2 {
				t.Fatal("missing/duplicate sessions", len(snapshot))
			}
			model := snapshot[ih]
			if model == nil || !model.Started || !model.Files[0].Started || model.Files[1].Started {
				t.Fatalf("lost selection: %+v", model)
			}
			if !snapshot[strings.Repeat("1", 40)].Started {
				t.Fatal("lost pending start")
			}
			if err := e.StopTorrent(ih); err != nil {
				t.Fatal(err)
			}
			if err := e.StopTorrent(strings.Repeat("1", 40)); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "downloads", "restart-fixture", "two.part"), bytes.Repeat([]byte("a"), 16), 0600); err != nil {
				t.Fatal(err)
			}
		case "paused":
			waitSessionBytes(t, e, ih, 32)
			for _, model := range e.GetTorrents() {
				if model.Started {
					t.Fatal("paused torrent restarted")
				}
			}
			e.mut.Lock()
			selection := e.ts[ih].selection
			e.mut.Unlock()
			if !selection["restart-fixture/one"] || selection["restart-fixture/two"] {
				t.Fatal("lost paused selection", selection)
			}
			for _, suffix := range []string{"", ".part"} {
				if err := os.WriteFile(filepath.Join(dir, "downloads", "restart-fixture", "one"+suffix), bytes.Repeat([]byte("z"), 16), 0600); err != nil {
					t.Fatal(err)
				}
			}
		case "corrupt":
			waitSessionBytes(t, e, ih, 16)
			if err := e.DeleteTorrent(ih); err != nil {
				t.Fatal(err)
			}
		case "deleted":
			if snapshot := e.GetTorrents(); len(snapshot) != 1 || snapshot[ih] != nil {
				t.Fatal("removed torrent resurrected")
			}
			if err := e.DeleteTorrent(strings.Repeat("1", 40)); err != nil {
				t.Fatal(err)
			}
		case "empty":
			if len(e.GetTorrents()) != 0 {
				t.Fatal("removed magnet resurrected")
			}
		}
		// Let testing exit the subprocess without a shutdown persistence hook.
		return
	}
	dir := t.TempDir()
	for _, stage := range []string{"write", "restore", "paused", "corrupt", "deleted", "empty"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSessionProcessRestart$", "-test.timeout=20s")
		cmd.Env = append(os.Environ(), "CT_SESSION_TEST_DIR="+dir, "CT_SESSION_TEST_STAGE="+stage)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", stage, err, output)
		}
	}
	for _, name := range []string{"state", "state/sessions.json"} {
		st, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm()&0077 != 0 {
			t.Fatal("public state permissions", name)
		}
	}
}

func TestSessionCorruptionAndLock(t *testing.T) {
	for _, data := range []string{`{`, `{"Version":999}`, `{"Version":1} {}`, fmt.Sprintf(`{"Version":1,"Torrents":[{"Magnet":%q}]}`, "bad"), strings.Repeat(" ", maxSessionBytes+1)} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "sessions.json"), []byte(data), 0600)
		e := New()
		if err := e.EnablePersistence(dir); err == nil {
			e.Close()
			t.Fatal("corrupt state accepted")
		}
		actual, _ := os.ReadFile(filepath.Join(dir, "sessions.json"))
		if string(actual) != data {
			t.Fatal("corrupt store overwritten")
		}
	}
	dir := t.TempDir()
	first := New()
	if err := first.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	second := New()
	if err := second.EnablePersistence(dir); err == nil {
		second.Close()
		t.Fatal("concurrent writer accepted")
	}
	first.Close()
	if err := second.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	second.Close()
}

func TestSessionFailedWriteRollsBackAndKeepsPreviousFile(t *testing.T) {
	dir := t.TempDir()
	e := New()
	if err := e.EnablePersistence(dir); err != nil {
		t.Fatal(err)
	}
	mustConfigureEngine(t, e)
	spec := sessionFixture(t)
	if err := e.NewTorrent(spec); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Point updates at a missing directory, simulating an unavailable state mount.
	e.mut.Lock()
	e.sessionDir = filepath.Join(dir, "missing")
	e.mut.Unlock()
	if err := e.StartTorrent(spec.InfoHash.HexString()); err == nil {
		t.Fatal("unpersisted intent acknowledged")
	}
	if e.GetTorrents()[spec.InfoHash.HexString()].Started {
		t.Fatal("failed action not rolled back")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "sessions.json"))
	if string(before) != string(after) {
		t.Fatal("last valid session changed")
	}
	var disk sessionDisk
	if err := json.Unmarshal(after, &disk); err != nil {
		t.Fatal(err)
	}
}

func TestSessionRechecksStoredData(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "downloads")
	stateDir := filepath.Join(dir, "state")
	e := New()
	if err := e.EnablePersistence(stateDir); err != nil {
		t.Fatal(err)
	}
	c := Config{DownloadDirectory: dataDir, IncomingPort: freePort(t)}
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	spec := sessionFixture(t)
	if err := e.NewTorrent(spec); err != nil {
		t.Fatal(err)
	}
	e.Close()
	// No completion counters are persisted. Even stale storage completion must
	// not turn missing/changed bytes into completed content after reopening.
	e = New()
	if err := e.EnablePersistence(stateDir); err != nil {
		t.Fatal(err)
	}
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if e.GetTorrents()[spec.InfoHash.HexString()].Downloaded != 0 {
			t.Fatal("trusted stale completion")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitSessionBytes(t *testing.T, e *Engine, ih string, want int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if model := e.GetTorrents()[ih]; model != nil && model.Downloaded == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("restored bytes: %+v, want %d", e.GetTorrents()[ih], want)
}
