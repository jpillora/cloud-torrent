package server

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/jpillora/cloud-torrent/engine"
	"github.com/jpillora/velox"
)

func inboxFixture(t *testing.T) (*Server, *inboxWatcher) {
	t.Helper()
	base := t.TempDir()
	s := &Server{engine: engine.New(), StateDirectory: filepath.Join(base, "state"), WatchDirectory: filepath.Join(base, "inbox")}
	if err := s.engine.EnablePersistence(s.StateDirectory); err != nil {
		t.Fatal(err)
	}
	if err := s.engine.Configure(engine.Config{DownloadDirectory: filepath.Join(base, "downloads"), IncomingPort: freePort(t)}); err != nil {
		t.Fatal(err)
	}
	w, err := newInboxWatcher(s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.root.Close(); s.engine.Close() })
	return s, w
}
func inboxBytes(t *testing.T, name string) []byte {
	t.Helper()
	info, err := bencode.Marshal(metainfo.Info{Name: name, Length: 1, PieceLength: 16384, Pieces: bytes.Repeat([]byte{1}, 20)})
	if err != nil {
		t.Fatal(err)
	}
	body, err := bencode.Marshal(metainfo.MetaInfo{InfoBytes: info})
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func inboxWrite(t *testing.T, s *Server, name string, body []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.WatchDirectory, name), body, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestInboxPartialRenameDedupAndRestart(t *testing.T) {
	s, w := inboxFixture(t)
	now := time.Now()
	body := inboxBytes(t, "legal.bin")
	inboxWrite(t, s, "legal.torrent", body[:12])
	w.step(now)
	inboxWrite(t, s, "legal.torrent", body)
	w.step(now.Add(time.Second))
	if n, _ := s.engine.InboxCounts(); n != 0 {
		t.Fatal("imported changing file")
	}
	status := w.step(now.Add(2 * time.Second))
	if status.Accepted != 1 || len(status.Errors) > 0 {
		t.Fatalf("import: %+v", status)
	}
	for _, torrent := range s.engine.GetTorrents() {
		if torrent.Started {
			t.Fatal("ignored AutoStart=false")
		}
	}
	inboxWrite(t, s, "atomic.tmp", body)
	if err := os.Rename(filepath.Join(s.WatchDirectory, "atomic.tmp"), filepath.Join(s.WatchDirectory, "atomic.torrent")); err != nil {
		t.Fatal(err)
	}
	w.step(now.Add(3 * time.Second))
	w.step(now.Add(4 * time.Second))
	if n, r := s.engine.InboxCounts(); n != 1 || r != 1 {
		t.Fatalf("duplicate: %d %d", n, r)
	}
	for hash := range s.engine.GetTorrents() {
		if err := s.engine.DeleteTorrent(hash); err != nil {
			t.Fatal(err)
		}
	}
	config := s.engine.Config()
	s.engine.Close()
	s.engine = engine.New()
	if err := s.engine.EnablePersistence(s.StateDirectory); err != nil {
		t.Fatal(err)
	}
	if err := s.engine.Configure(config); err != nil {
		t.Fatal(err)
	}
	fresh, err := newInboxWatcher(s)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.root.Close()
	fresh.step(now)
	fresh.step(now.Add(time.Second))
	if n, r := s.engine.InboxCounts(); n != 0 || r != 1 {
		t.Fatalf("retained source resurrected removed torrent: %d %d", n, r)
	}
	if _, err := os.Stat(filepath.Join(s.WatchDirectory, "legal.torrent")); err != nil {
		t.Fatal("source removed")
	}
}
func TestInboxRejectsAndCachesUnsafeFiles(t *testing.T) {
	s, w := inboxFixture(t)
	now := time.Now()
	inboxWrite(t, s, "bad.torrent", []byte("not bencode"))
	inboxWrite(t, s, "large.torrent", []byte{1})
	if err := os.Truncate(filepath.Join(s.WatchDirectory, "large.torrent"), maxRequestBytes+1); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(s.WatchDirectory, "dir.torrent"), 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(outside, inboxBytes(t, "outside.bin"), 0600)
	if err := os.Symlink(outside, filepath.Join(s.WatchDirectory, "link.torrent")); err != nil {
		t.Fatal(err)
	}
	w.step(now)
	status := w.step(now.Add(time.Second))
	if len(status.Errors) != 4 || status.Accepted != 0 {
		t.Fatalf("unsafe imports: %+v", status)
	}
	attempts := w.attempts
	w.step(now.Add(10 * time.Second))
	if attempts != w.attempts {
		t.Fatal("retried unchanged bad file")
	}
	inboxWrite(t, s, "bad.torrent", inboxBytes(t, "fixed.bin"))
	w.step(now.Add(11 * time.Second))
	status = w.step(now.Add(12 * time.Second))
	if status.Accepted != 1 {
		t.Fatalf("replacement not retried: %+v", status)
	}
	if _, err := parseTorrentBytes(append(inboxBytes(t, "trailing.bin"), 'x')); err == nil {
		t.Fatal("accepted trailing bytes")
	}
	if _, err := parseTorrentBytes([]byte("d4:infoi1ee")); err == nil {
		t.Fatal("accepted invalid info")
	}
}
func TestInboxBudgetsQueueAndSaveFailure(t *testing.T) {
	t.Run("scan and import budgets", func(t *testing.T) {
		s, w := inboxFixture(t)
		now := time.Now()
		for i := range maxInboxEntries + 1 {
			inboxWrite(t, s, fmt.Sprintf("%03d.tmp", i), nil)
		}
		status := w.step(now)
		if len(status.Errors) != 1 || w.attempts != 0 {
			t.Fatalf("scan budget: %+v", status)
		}
		for i := range maxInboxEntries + 1 {
			os.Remove(filepath.Join(s.WatchDirectory, fmt.Sprintf("%03d.tmp", i)))
		}
		for i := range 4 {
			inboxWrite(t, s, fmt.Sprintf("%d.torrent", i), inboxBytes(t, fmt.Sprintf("%d.bin", i)))
		}
		w.step(now)
		status = w.step(now.Add(time.Second))
		if status.Accepted != 2 || status.Pending != 2 {
			t.Fatalf("import budget: %+v", status)
		}
		status = w.step(now.Add(2 * time.Second))
		if status.Accepted != 4 {
			t.Fatalf("backlog: %+v", status)
		}
	})
	t.Run("capacity", func(t *testing.T) {
		s, w := inboxFixture(t)
		now := time.Now()
		for i := 1; i <= 256; i++ {
			if err := s.engine.NewMagnet(fmt.Sprintf("magnet:?xt=urn:btih:%040x", i)); err != nil {
				t.Fatal(err)
			}
		}
		inboxWrite(t, s, "wait.torrent", inboxBytes(t, "waiting.bin"))
		w.step(now)
		status := w.step(now.Add(time.Second))
		if status.Pending != 1 || status.Accepted != 0 {
			t.Fatalf("full queue: %+v", status)
		}
		attempts := w.attempts
		w.step(now.Add(2 * time.Second))
		if w.attempts != attempts {
			t.Fatal("busy retried full queue")
		}
		if err := s.engine.DeleteTorrent(fmt.Sprintf("%040x", 1)); err != nil {
			t.Fatal(err)
		}
		status = w.step(now.Add(3 * time.Second))
		if status.Accepted != 1 {
			t.Fatalf("slot not retried: %+v", status)
		}
	})
	t.Run("save failure", func(t *testing.T) {
		s, w := inboxFixture(t)
		now := time.Now()
		body := inboxBytes(t, "save.bin")
		inboxWrite(t, s, "save.torrent", body)
		w.step(now)
		moved := s.StateDirectory + "-moved"
		if err := os.Rename(s.StateDirectory, moved); err != nil {
			t.Fatal(err)
		}
		status := w.step(now.Add(time.Second))
		if status.Accepted != 0 || len(status.Errors) != 1 {
			t.Fatalf("save failure acknowledged: %+v", status)
		}
		if n, r := s.engine.InboxCounts(); n != 0 || r != 0 {
			t.Fatalf("save rollback: %d %d", n, r)
		}
		if err := os.Rename(moved, s.StateDirectory); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(s.WatchDirectory, "save.torrent"), filepath.Join(s.WatchDirectory, "retry.torrent")); err != nil {
			t.Fatal(err)
		}
		w.step(now.Add(2 * time.Second))
		status = w.step(now.Add(3 * time.Second))
		if status.Accepted != 1 {
			t.Fatalf("save retry: %+v", status)
		}
	})
}
func TestInboxLayoutAndReplacement(t *testing.T) {
	s, w := inboxFixture(t)
	if err := s.validateInboxLayout(s.WatchDirectory); err == nil {
		t.Fatal("accepted overlapping downloads")
	}
	original := s.StateDirectory
	s.StateDirectory = filepath.Join(s.WatchDirectory, "state")
	if err := s.validateInboxLayout(s.engine.Config().DownloadDirectory); err == nil {
		t.Fatal("accepted overlapping state")
	}
	s.StateDirectory = original
	config := s.engine.Config()
	config.DownloadDirectory = s.WatchDirectory
	if err := s.reconfigure(config); err == nil {
		t.Fatal("accepted unsafe reconfiguration")
	}
	if s.engine.Config().DownloadDirectory == s.WatchDirectory {
		t.Fatal("changed config after rejection")
	}
	if err := os.Rename(s.WatchDirectory, s.WatchDirectory+"-old"); err != nil {
		t.Fatal(err)
	}
	os.Mkdir(s.WatchDirectory, 0700)
	status := w.step(time.Now())
	if len(status.Errors) != 1 || !strings.Contains(status.Errors[0].Message, "changed") {
		t.Fatalf("replacement: %+v", status)
	}
	disabled, err := newInboxWatcher(&Server{})
	if err != nil || disabled != nil {
		t.Fatal("not opt-in")
	}
}

func TestInboxShutdown(t *testing.T) {
	s, _ := inboxFixture(t)
	_ = velox.SyncHandler(&s.state)
	stop, err := s.startInbox()
	if err != nil {
		t.Fatal(err)
	}
	stop()
	inboxWrite(t, s, "after-stop.torrent", inboxBytes(t, "stopped.bin"))
	// Stop joins the worker: no read/import can remain in flight afterwards.
	if n, r := s.engine.InboxCounts(); n != 0 || r != 0 {
		t.Fatalf("import after stop: %d %d", n, r)
	}
	s.state.Lock()
	s.state.Unlock()
}
