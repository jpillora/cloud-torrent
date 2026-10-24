package server

import (
	"archive/zip"
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestProtectedContentAcrossListingHTTPArchivesAndDeletion(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "folder")
	must(t, os.Mkdir(folder, 0o700))
	config := filepath.Join(folder, "custom-settings.json")
	mustWrite(t, config, "private configuration")
	mustWrite(t, filepath.Join(folder, "safe.txt"), "public")
	must(t, os.Link(config, filepath.Join(folder, "hard-alias")))
	must(t, os.Symlink("custom-settings.json", filepath.Join(folder, "link-alias")))
	must(t, os.Symlink("folder", filepath.Join(dir, "folder-alias")))
	h := http.StripPrefix("/download/", downloadHandler(func() string { return dir }, nil, nil, func() []string { return []string{config} }))
	tree := listFiles(dir, config)
	if len(tree.Children) != 1 || tree.Children[0].Size != 6 || len(tree.Children[0].Children) != 1 {
		t.Fatalf("exposed control files in tree: %+v", tree)
	}
	for _, name := range []string{"folder/custom-settings.json", "folder/hard-alias", "folder/link-alias", "folder-alias/custom-settings.json"} {
		for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodDelete} {
			rr := do(h, method, "/download/"+name)
			if rr.Code != http.StatusForbidden {
				t.Fatalf("%s %s: %d", method, name, rr.Code)
			}
		}
	}
	for _, name := range []string{"folder", "folder-alias"} {
		rr := do(h, http.MethodGet, "/download/"+name)
		if rr.Code != 200 {
			t.Fatal(rr.Code)
		}
		zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
		must(t, err)
		if len(zr.File) != 1 || zr.File[0].Name != "safe.txt" {
			t.Fatalf("archive exposed state: %+v", zr.File)
		}
	}
	if rr := do(h, http.MethodDelete, "/download/folder"); rr.Code != http.StatusForbidden {
		t.Fatal(rr.Code)
	}
	if _, err := os.Stat(config); err != nil {
		t.Fatal("configuration lost", err)
	}
	rr := do(h, http.MethodHead, "/download/folder/safe.txt")
	if rr.Code != 200 || rr.Body.Len() != 0 {
		t.Fatalf("HEAD: %d %q", rr.Code, rr.Body.String())
	}
	rr = do(h, http.MethodDelete, "/download/folder/safe.txt")
	if rr.Code != 200 {
		t.Fatal(rr.Code)
	}
}

func TestStateDirectoryLayout(t *testing.T) {
	dir := t.TempDir()
	must(t, os.Mkdir(filepath.Join(dir, "downloads"), 0o700))
	must(t, os.Symlink("downloads", filepath.Join(dir, "alias")))
	for _, state := range []string{"downloads", "downloads/state", "alias/not-created", "."} {
		if err := validateStateDirectory(filepath.Join(dir, "downloads"), filepath.Join(dir, state)); err == nil {
			t.Fatalf("overlap allowed: %s", state)
		}
	}
	must(t, validateStateDirectory(filepath.Join(dir, "downloads"), filepath.Join(dir, "private-state")))
}

func TestLegacyStateSubtreeExcludedFromArchiveAndDeletion(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "folder", "state")
	must(t, os.MkdirAll(state, 0o700))
	mustWrite(t, filepath.Join(state, "session.json"), "private magnets")
	mustWrite(t, filepath.Join(dir, "folder", "public.txt"), "public")
	must(t, os.Symlink("folder", filepath.Join(dir, "alias")))
	h := http.StripPrefix("/download/", downloadHandler(func() string { return dir }, nil, nil, func() []string { return []string{state} }))
	for _, name := range []string{"folder/state/session.json", "alias/state/session.json"} {
		if rr := do(h, http.MethodGet, "/download/"+name); rr.Code != http.StatusForbidden {
			t.Fatalf("state alias %s: %d", name, rr.Code)
		}
	}
	for _, name := range []string{"folder", "alias"} {
		rr := do(h, http.MethodGet, "/download/"+name)
		zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
		must(t, err)
		if len(zr.File) != 1 || zr.File[0].Name != "public.txt" {
			t.Fatal("state subtree leaked")
		}
	}
	if rr := do(h, http.MethodDelete, "/download/folder"); rr.Code != http.StatusForbidden {
		t.Fatal("parent deletion allowed")
	}
}
