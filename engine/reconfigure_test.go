package engine

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// Use a real, throttled HTTP webseed and verified torrent pieces. Transitions
// must resume byte progress, not merely preserve Started flags.
func TestReconfigureMaintainsTransfer(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 512*1024)
	seed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(throttledWriter{w, r}, r, "fixture.bin", time.Time{}, bytes.NewReader(data))
	}))
	defer seed.Close()
	const pieceLength = 64 * 1024
	var pieces []byte
	for offset := 0; offset < len(data); offset += pieceLength {
		hash := sha1.Sum(data[offset:min(offset+pieceLength, len(data))])
		pieces = append(pieces, hash[:]...)
	}
	infoBytes, err := bencode.Marshal(metainfo.Info{Name: "fixture.bin", Length: int64(len(data)), PieceLength: pieceLength, Pieces: pieces})
	if err != nil {
		t.Fatal(err)
	}
	spec := &torrent.TorrentSpec{AddTorrentOpts: torrent.AddTorrentOpts{InfoHash: metainfo.HashBytes(infoBytes), InfoBytes: infoBytes}, Webseeds: []string{seed.URL + "/fixture.bin"}}
	ih := spec.InfoHash.HexString()
	e := New()
	c := mustConfigureEngine(t, e)
	if err := e.NewTorrent(spec); err != nil {
		t.Fatal(err)
	}
	if err := e.StartTorrent(ih); err != nil {
		t.Fatal(err)
	}
	waitProgress := func(baseline int64) int64 {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			tr := e.GetTorrents()[ih]
			if tr != nil && tr.Started && tr.Downloaded > baseline {
				return tr.Downloaded
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("no byte progress beyond %d; current=%+v", baseline, e.GetTorrents()[ih])
		return 0
	}
	before := waitProgress(0)
	oldClient, oldHandle := e.client, e.ts[ih].t
	c.AutoStart = true
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	if e.client != oldClient || e.ts[ih].t != oldHandle {
		t.Fatal("AutoStart change replaced the live client")
	}
	before = waitProgress(before)
	c.IncomingPort = freePort(t)
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	if e.client == oldClient || e.ts[ih].t == oldHandle {
		t.Fatal("replacement retained closed handles")
	}
	before = waitProgress(before)
	// A rejected occupied port must leave the working instance untouched.
	busy, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	bad := c
	bad.IncomingPort = busy.Addr().(*net.TCPAddr).Port
	oldClient, oldHandle = e.client, e.ts[ih].t
	if err := e.Configure(bad); err == nil {
		t.Fatal("accepted occupied port")
	}
	if e.Config() != c || e.client != oldClient || e.ts[ih].t != oldHandle {
		t.Fatal("rejected port disturbed the working engine")
	}
	before = waitProgress(before)
	// Simulate a failure after the old sockets close (e.g. a port stolen after
	// preflight). The real rollback client must reopen and keep transferring.
	var attempts atomic.Int32
	e.newClient = func(cfg *torrent.ClientConfig) (*torrent.Client, error) {
		if attempts.Add(1) == 1 {
			return nil, errors.New("injected replacement failure")
		}
		return torrent.NewClient(cfg)
	}
	bad = c
	bad.EnableUpload = !c.EnableUpload
	if err := e.Configure(bad); err == nil {
		t.Fatal("replacement failure not returned")
	}
	if attempts.Load() != 2 || e.Config() != c || e.client == nil {
		t.Fatal("old configuration was not restored")
	}
	waitProgress(before)
	// Moving the configured directory also rebinds storage and restarts actual
	// transfer in the new location without stale wrappers.
	c.DownloadDirectory = t.TempDir()
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	waitProgress(0)
}

type throttledWriter struct {
	http.ResponseWriter
	request *http.Request
}

func (w throttledWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		select {
		case <-w.request.Context().Done():
			return written, w.request.Context().Err()
		case <-time.After(5 * time.Millisecond):
		}
		n, err := w.ResponseWriter.Write(p[:min(len(p), 16*1024)])
		written += n
		p = p[n:]
		if err != nil {
			return written, err
		}
		if f, ok := w.ResponseWriter.(http.Flusher); ok {
			f.Flush()
		}
	}
	return written, nil
}

func TestReconfigurePreservesStoppedAndPendingTorrents(t *testing.T) {
	e := New()
	c := mustConfigureEngine(t, e)
	infoBytes, err := bencode.Marshal(metainfo.Info{Name: "stopped", Length: 1, PieceLength: 1, Pieces: make([]byte, 20)})
	if err != nil {
		t.Fatal(err)
	}
	spec := &torrent.TorrentSpec{AddTorrentOpts: torrent.AddTorrentOpts{InfoHash: metainfo.HashBytes(infoBytes), InfoBytes: infoBytes}}
	if err := e.NewTorrent(spec); err != nil {
		t.Fatal(err)
	}
	pending := "magnet:?xt=urn:btih:" + sintelInfoHash + "&dn=Pending&xs=https%3A%2F%2Fexample.invalid%2Fpending.torrent"
	if err := e.NewMagnet(pending); err != nil {
		t.Fatal(err)
	}
	c.AutoStart = true
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	c.EnableUpload = true
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	stopped := e.GetTorrents()[spec.InfoHash.HexString()]
	if stopped.Started || stopped.Files[0].Started {
		t.Fatal("reconfigure started a stopped torrent")
	}
	model := e.ts[sintelInfoHash]
	if model == nil || model.Started || model.autoStart || len(model.spec.Sources) != 1 || model.spec.Sources[0] != "https://example.invalid/pending.torrent" {
		t.Fatalf("pending policy/sources lost: %+v", model)
	}
}
