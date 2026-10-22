package engine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
