//go:build !windows && !plan9

package server

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestInboxRejectsFIFOAndSymlinkSwap(t *testing.T) {
	s, w := inboxFixture(t)
	if err := unix.Mkfifo(filepath.Join(s.WatchDirectory, "pipe.torrent"), 0600); err != nil {
		t.Fatal(err)
	}
	status := w.step(time.Now())
	if len(status.Errors) != 1 || w.attempts != 0 {
		t.Fatalf("FIFO: %+v", status)
	}
	inboxWrite(t, s, "swap.torrent", inboxBytes(t, "swap.bin"))
	before, err := w.root.Lstat("swap.torrent")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.WatchDirectory, "swap.torrent")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("pipe.torrent", filepath.Join(s.WatchDirectory, "swap.torrent")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.readStableFile("swap.torrent", before); err == nil {
		t.Fatal("followed replaced symlink")
	}
}
