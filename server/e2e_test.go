//go:build e2e

package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jpillora/cloud-torrent/engine"
)

// TestE2ECoreLoop proves the core loop (choose -> track -> download) against a
// live fixture. It downloads real data, so it is gated behind the `e2e` build
// tag:
//
//	go test -tags e2e -run E2E ./server/...
//
// The browser half of the E2E (Omni -> Torrents -> Downloads in a real page) is
// driven via the Chrome DevTools MCP per CLAUDE.md, not a committed suite.
func TestE2ECoreLoop(t *testing.T) {
	dir := t.TempDir()
	s := &Server{engine: engine.New()}
	t.Cleanup(func() { s.engine.Close() })
	if err := s.engine.Configure(engine.Config{
		DownloadDirectory: dir,
		IncomingPort:      freePort(t),
		EnableUpload:      true,
		AutoStart:         true,
	}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)

	// choose: add the fixture magnet
	postJSON(t, srv.URL+"/api/magnet", `{"uri":"`+sintelMagnet+`"}`)

	// track: poll until metadata loads, the torrent starts, and it progresses
	var tr *engine.Torrent
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		tr = s.engine.GetTorrents()[sintelInfoHash]
		if tr != nil && tr.Loaded && tr.Started && tr.Percent > 0 {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if tr == nil || !tr.Loaded {
		t.Fatalf("torrent did not load within timeout (tr=%+v)", tr)
	}
	if !tr.Started || tr.Percent <= 0 {
		t.Fatalf("no download progress: started=%v percent=%v", tr.Started, tr.Percent)
	}
	t.Logf("progress: percent=%v files=%d size=%d", tr.Percent, len(tr.Files), tr.Size)

	// download: the torrent directory streams a valid zip of what's on disk
	// (check the magic bytes rather than draining the whole archive)
	resp := httpGet(t, srv.URL+"/download/"+url.PathEscape(sintelName))
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
		t.Errorf("dir content-type = %q, want application/zip", ct)
	}
	magic := make([]byte, 4)
	if _, err := io.ReadFull(resp.Body, magic); err != nil {
		t.Fatalf("read zip stream: %v", err)
	}
	if string(magic[:2]) != "PK" {
		t.Errorf("dir download is not a zip (magic %q)", magic)
	}

	// cleanup: delete the torrent
	postJSON(t, srv.URL+"/api/torrent", `{"action":"delete","infohash":"`+sintelInfoHash+`"}`)
	if _, ok := s.engine.GetTorrents()[sintelInfoHash]; ok {
		t.Error("torrent still present after delete")
	}
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
