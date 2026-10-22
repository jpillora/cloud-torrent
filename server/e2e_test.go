//go:build e2e

package server

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jpillora/cloud-torrent/engine"
)

// TestE2ECoreLoop is an opt-in public-network backend smoke test. The full UI
// and realtime contract is covered by ./dev.ts e2e (optionally --public-fixture).
// Run this additional test with: go test -tags e2e -run E2E ./server/...
func TestE2ECoreLoop(t *testing.T) {
	const name = "Big Buck Bunny"
	const poster = "poster.jpg"
	const checksum = "d082c4ae86f4f8d323c4a03665cdec24c20ad0dcaddcfef1bcab073457a101ff"
	dir := t.TempDir()
	s := &Server{engine: engine.New()}
	t.Cleanup(func() { s.engine.Close() })
	if err := s.engine.Configure(engine.Config{DownloadDirectory: dir, IncomingPort: freePort(t)}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	// Explicit metainfo provides WebSeeds without requiring DHT/swarm metadata.
	postJSON(t, srv.URL+"/api/url", `{"url":"https://webtorrent.io/torrents/big-buck-bunny.torrent"}`)
	torrents := s.engine.GetTorrents()
	if len(torrents) != 1 {
		t.Fatalf("torrents: %+v", torrents)
	}
	var hash string
	for ih := range torrents {
		hash = ih
	}
	request, _ := json.Marshal(FileRequest{Action: "start", InfoHash: hash, Path: name + "/" + poster})
	postJSON(t, srv.URL+"/api/file", string(request))
	deadline := time.Now().Add(150 * time.Second)
	complete := false
	for time.Now().Before(deadline) {
		tr := s.engine.GetTorrents()[hash]
		for _, f := range tr.Files {
			if f.Path == name+"/"+poster && f.Percent == 100 {
				_, err := os.Stat(filepath.Join(dir, name, poster))
				complete = err == nil
			}
		}
		if complete {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !complete {
		t.Fatal("public poster did not complete")
	}
	verify := func(content []byte) {
		t.Helper()
		if len(content) != 310380 || fmt.Sprintf("%x", sha256.Sum256(content)) != checksum {
			t.Fatal("poster content mismatch")
		}
	}
	resp := httpGet(t, srv.URL+"/download/"+url.PathEscape(name)+"/"+poster)
	content, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	verify(content)
	for _, action := range []string{"stop", "delete"} {
		request, _ := json.Marshal(TorrentRequest{Action: action, InfoHash: hash})
		postJSON(t, srv.URL+"/api/torrent", string(request))
	}
	if len(s.engine.GetTorrents()) != 0 {
		t.Fatal("torrent was not removed")
	}
	// The poster shares a boundary piece with the film. Remove that sparse
	// partial neighbor before archiving; do not download or archive the film.
	entries, err := os.ReadDir(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".part") {
			continue
		}
		req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/download/"+url.PathEscape(name)+"/"+url.PathEscape(entry.Name()), nil)
		deleted, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		deleted.Body.Close()
		if deleted.StatusCode != http.StatusOK {
			t.Fatalf("partial cleanup: %s", deleted.Status)
		}
	}
	resp = httpGet(t, srv.URL+"/download/"+url.PathEscape(name))
	archive, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != poster {
		t.Fatalf("unexpected archive entries: %+v", zr.File)
	}
	f, err := zr.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	content, err = io.ReadAll(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	verify(content)
}

func postJSON(t *testing.T, u, body string) {
	t.Helper()
	resp, err := http.Post(u, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s: %d (%s)", u, resp.StatusCode, b)
	}
}

func httpGet(t *testing.T, u string) *http.Response {
	t.Helper()
	resp, err := http.Get(u)
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("GET %s: %d", u, resp.StatusCode)
	}
	return resp
}
