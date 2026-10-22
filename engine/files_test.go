package engine

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoveDownloadProtectsActiveFilesAndAncestors(t *testing.T) {
	e, c, ih := newTransferFixtureFiles(t, true)
	if err := e.StartFile(ih, "fixture/first.bin"); err != nil {
		t.Fatal(err)
	}
	waitTransferProgress(t, e, ih, 0)
	root, err := os.OpenRoot(c.DownloadDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Symlink("fixture", filepath.Join(c.DownloadDirectory, "alias")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fixture", "fixture/first.bin.part", "alias", "alias/first.bin.part"} {
		if err := e.RemoveDownload(root, name); !errors.Is(err, ErrActiveDownload) {
			t.Fatalf("active path %q: %v", name, err)
		}
		if _, err := root.Stat("fixture/first.bin.part"); err != nil {
			t.Fatal(err)
		}
	}
	if err := root.Mkdir("fixture-other", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := e.RemoveDownload(root, "fixture-other"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".", "..", "../outside"} {
		if err := e.RemoveDownload(root, name); err == nil {
			t.Fatalf("allowed invalid path %q", name)
		}
	}
	wrongRoot, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer wrongRoot.Close()
	if err := e.RemoveDownload(wrongRoot, "fixture"); err == nil {
		t.Fatal("allowed stale root")
	}
	if err := e.StopTorrent(ih); err != nil {
		t.Fatal(err)
	}
	if err := e.RemoveDownload(root, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Stat("fixture"); !os.IsNotExist(err) {
		t.Fatalf("paused directory not deleted: %v", err)
	}
}

func TestRemoveDownloadRestartRestoresContent(t *testing.T) {
	for _, completed := range []bool{false, true} {
		for _, parent := range []bool{false, true} {
			t.Run(fmt.Sprintf("complete=%v/parent=%v", completed, parent), func(t *testing.T) {
				e, c, ih := newTransferFixtureFiles(t, parent)
				if err := e.StartTorrent(ih); err != nil {
					t.Fatal(err)
				}
				waitTransferProgress(t, e, ih, 0)
				if completed {
					waitDownloadContent(t, e, ih, c.DownloadDirectory, parent)
				}
				if err := e.StopTorrent(ih); err != nil {
					t.Fatal(err)
				}
				root, err := os.OpenRoot(c.DownloadDirectory)
				if err != nil {
					t.Fatal(err)
				}
				defer root.Close()
				name := "fixture.bin.part"
				if completed {
					name = "fixture.bin"
				}
				if parent {
					name = "fixture"
				}
				if err := e.RemoveDownload(root, name); err != nil {
					t.Fatal(err)
				}
				if tr := e.GetTorrents()[ih]; tr.Started || tr.Downloaded != 0 || tr.Percent != 0 {
					t.Fatalf("deleted content still reported complete or started: %+v", tr)
				}
				if err := e.StartTorrent(ih); err != nil {
					t.Fatal(err)
				}
				waitDownloadContent(t, e, ih, c.DownloadDirectory, parent)
			})
		}
	}
}

// Require verified completion AND the complete expected bytes at the final paths.
func waitDownloadContent(t *testing.T, e *Engine, ih, dir string, multi bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if tr := e.GetTorrents()[ih]; tr != nil && tr.Percent == 100 {
			names := []string{"fixture.bin"}
			size := 8 * 1024 * 1024
			if multi {
				names = []string{"fixture/first.bin", "fixture/second.bin"}
				size /= 2
			}
			valid := true
			for _, name := range names {
				got, err := os.ReadFile(filepath.Join(dir, name))
				want := bytes.Repeat([]byte("0123456789abcdef"), size/16)
				if err != nil || !bytes.Equal(got, want) {
					valid = false
					break
				}
			}
			// Upstream publishes completion just before promoting .part files.
			if valid {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("download did not complete: %+v", e.GetTorrents()[ih])
}

func TestRemoveDownloadPreservesSelectionsAndUnrelatedHandles(t *testing.T) {
	e, c, ih := newTransferFixtureFiles(t, true)
	if err := e.StartFile(ih, "fixture/first.bin"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for e.GetTorrents()[ih].Files[0].Percent != 100 {
		if time.Now().After(deadline) {
			t.Fatal("selected file did not complete")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Add an unrelated pending magnet; deleting data must not replace its handle.
	if err := e.NewMagnet("magnet:?xt=urn:btih:" + sintelInfoHash); err != nil {
		t.Fatal(err)
	}
	unrelated := e.ts[sintelInfoHash].t
	root, err := os.OpenRoot(c.DownloadDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := e.RemoveDownload(root, "fixture/first.bin"); err != nil {
		t.Fatal(err)
	}
	if e.ts[sintelInfoHash].t != unrelated {
		t.Fatal("unrelated torrent handle replaced")
	}
	tr := e.GetTorrents()[ih]
	if !tr.Started || !tr.Files[0].Started || tr.Files[1].Started {
		t.Fatalf("file selection changed: %+v", tr)
	}
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		got, err := os.ReadFile(filepath.Join(c.DownloadDirectory, "fixture/first.bin"))
		if err == nil && bytes.Equal(got, bytes.Repeat([]byte("0123456789abcdef"), 256*1024)) && e.GetTorrents()[ih].Files[0].Percent == 100 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("selected file did not redownload")
}
