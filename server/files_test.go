package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystemNodeKinds(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"empty", "hidden-only", "unreadable", "populated"} {
		must(t, os.Mkdir(filepath.Join(dir, name), 0o755))
	}
	mustWrite(t, filepath.Join(dir, "hidden-only", ".hidden"), "hidden")
	mustWrite(t, filepath.Join(dir, "populated", "file.txt"), "contents")
	mustWrite(t, filepath.Join(dir, "ordinary.txt"), "ordinary")
	must(t, os.Chmod(filepath.Join(dir, "unreadable"), 0))
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, "unreadable"), 0o755) })
	root := listFiles(dir)
	if !root.Directory || len(root.Children) != 5 {
		t.Fatalf("unexpected root: %+v", root)
	}
	for _, n := range root.Children {
		if n.Directory != (n.Name != "ordinary.txt") || n.Children == nil {
			t.Fatalf("incorrect node kind/children: %+v", n)
		}
		if n.Name == "populated" && (len(n.Children) != 1 || n.Children[0].Directory) {
			t.Fatalf("file child misclassified: %+v", n)
		}
	}
	// Force ReadDir to fail even when tests run as root (which can read 000).
	info, err := os.Stat(filepath.Join(dir, "empty"))
	must(t, err)
	node, count := &FsNode{}, 0
	must(t, list(filepath.Join(dir, "missing-directory"), info, node, &count))
	if !node.Directory || node.Children == nil {
		t.Fatalf("unlistable directory lost identity: %+v", node)
	}
	b, err := json.Marshal(root)
	must(t, err)
	if bytes.Contains(b, []byte(`"Children":null`)) {
		t.Fatalf("wire children contradict generated array type: %s", b)
	}
}

func TestListFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "hello") // 5 bytes
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
		"/download/",                             // the root itself
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
	return http.StripPrefix("/download/", downloadHandler(func() string { return dir }, nil))
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

// Exercise the real HTTP transport and on-disk link resolution, including
// archives and deletion, rather than only testing string normalization.
func TestDownloadSymlinkConfinement(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(outside, "secret.txt"), "outside secret")
	must(t, os.Mkdir(filepath.Join(dir, "folder"), 0o755))
	mustWrite(t, filepath.Join(dir, "folder", "safe.txt"), "inside file")
	links := map[string]string{
		"external-file": filepath.Join(outside, "secret.txt"),
		"external-dir":  outside,
		"internal-file": filepath.Join("folder", "safe.txt"),
		"internal-dir":  "folder",
		"root-alias":    ".",
		"folder/leak":   filepath.Join(outside, "secret.txt"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	srv := httptest.NewServer(mount(dir))
	defer srv.Close()
	request := func(method, name string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, srv.URL+"/download/"+name, nil)
		must(t, err)
		resp, err := srv.Client().Do(req)
		must(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		must(t, err)
		return resp.StatusCode, body
	}
	for _, name := range []string{"external-file", "external-dir", "external-dir/secret.txt", "root-alias"} {
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			code, body := request(method, name)
			if code == http.StatusOK || bytes.Contains(body, []byte("outside secret")) {
				t.Fatalf("%s %s escaped: status=%d body=%q", method, name, code, body)
			}
		}
	}
	for _, name := range []string{"folder/safe.txt", "internal-file", "internal-dir/safe.txt"} {
		code, body := request(http.MethodGet, name)
		if code != http.StatusOK || string(body) != "inside file" {
			t.Fatalf("GET %s: status=%d body=%q", name, code, body)
		}
	}
	for _, name := range []string{"folder", "internal-dir"} {
		code, body := request(http.MethodGet, name)
		if code != http.StatusOK {
			t.Fatalf("GET archive %s: %d", name, code)
		}
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		must(t, err)
		if len(zr.File) != 1 || zr.File[0].Name != "safe.txt" {
			t.Fatalf("archive %s must omit links: %+v", name, zr.File)
		}
		f, err := zr.File[0].Open()
		must(t, err)
		contents, err := io.ReadAll(f)
		f.Close()
		must(t, err)
		if string(contents) != "inside file" {
			t.Fatalf("archive contents: %q", contents)
		}
	}
	// Deleting an internal link removes the link, retaining the target.
	for _, name := range []string{"internal-file", "internal-dir"} {
		code, body := request(http.MethodDelete, name)
		if code != http.StatusOK {
			t.Fatalf("DELETE %s: %d %s", name, code, body)
		}
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("link remains: %s", name)
		}
	}
	for _, name := range []string{filepath.Join(outside, "secret.txt"), filepath.Join(dir, "folder", "safe.txt")} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("target lost: %s: %v", name, err)
		}
	}
}
