package server

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "hello")    // 5 bytes
	must(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	mustWrite(t, filepath.Join(dir, "sub", "b.txt"), "world!") // 6 bytes
	mustWrite(t, filepath.Join(dir, ".hidden"), "x")           // skipped

	root := listFiles(dir)
	if len(root.Children) != 2 {
		t.Fatalf("got %d children, want 2 (dotfile should be skipped)", len(root.Children))
	}
	if root.Size != 11 {
		t.Errorf("aggregate size = %d, want 11", root.Size)
	}
	var sub *FsNode
	for _, c := range root.Children {
		if c.Name == ".hidden" {
			t.Error("dotfile was not skipped")
		}
		if c.Name == "sub" {
			sub = c
		}
	}
	if sub == nil || len(sub.Children) != 1 || sub.Children[0].Name != "b.txt" {
		t.Fatalf("sub directory tree is wrong: %+v", sub)
	}
}

func TestDownloadFile(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "hello world")
	h := mount(dir)

	rr := do(h, http.MethodGet, "/download/a.txt")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET file: code %d", rr.Code)
	}
	if got := rr.Body.String(); got != "hello world" {
		t.Errorf("GET file body = %q, want %q", got, "hello world")
	}
}

func TestDownloadDirZip(t *testing.T) {
	dir := t.TempDir()
	must(t, os.Mkdir(filepath.Join(dir, "folder"), 0o755))
	mustWrite(t, filepath.Join(dir, "folder", "f.txt"), "zipme")
	h := mount(dir)

	rr := do(h, http.MethodGet, "/download/folder")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET dir: code %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("content-type = %q, want application/zip", ct)
	}
	zr, err := zip.NewReader(bytes.NewReader(rr.Body.Bytes()), int64(rr.Body.Len()))
	if err != nil {
		t.Fatalf("response is not a valid zip: %v", err)
	}
	found := false
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "f.txt") {
			found = true
		}
	}
	if !found {
		t.Error("zip did not contain f.txt")
	}
}

func TestDownloadTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(filepath.Dir(dir), "secret.txt")
	mustWrite(t, outside, "secret")
	defer os.Remove(outside)
	h := mount(dir)

	for _, p := range []string{
		"/download/../" + filepath.Base(outside), // escape via ..
		"/download/",                              // the root itself
	} {
		rr := do(h, http.MethodGet, p)
		if rr.Code == http.StatusOK {
			t.Errorf("expected %q to be blocked, got 200 (body %q)", p, rr.Body.String())
		}
	}
}

func TestDownloadDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "del.txt")
	mustWrite(t, p, "bye")
	h := mount(dir)

	rr := do(h, http.MethodDelete, "/download/del.txt")
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE: code %d (%s)", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Error("file still exists after DELETE")
	}
}

// helpers

func mount(dir string) http.Handler {
	return http.StripPrefix("/download/", downloadHandler(func() string { return dir }))
}

func do(h http.Handler, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	must(t, os.WriteFile(path, []byte(content), 0o644))
}
