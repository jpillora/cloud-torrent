package engine

import (
	"bytes"
	"crypto/sha1"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Real torrent pieces served over HTTP at a bounded speed, allowing lifecycle
// tests to assert transfer behavior rather than only model flags.
func newTransferFixture(t *testing.T) (*Engine, Config, string) {
	t.Helper()
	data := bytes.Repeat([]byte("0123456789abcdef"), 512*1024)
	seed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(throttledWriter{w, r}, r, "fixture.bin", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(seed.Close)
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

	return e, c, ih
}

func waitTransferProgress(t *testing.T, e *Engine, ih string, baseline int64) int64 {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		tr := e.GetTorrents()[ih]
		if tr != nil && tr.Started && tr.Downloaded > baseline {
			return tr.Downloaded
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no progress beyond %d: %+v", baseline, e.GetTorrents()[ih])
	return 0
}

func TestStopStartResumesLiveTransfer(t *testing.T) {
	e, _, ih := newTransferFixture(t)
	if err := e.StartTorrent(ih); err != nil {
		t.Fatal(err)
	}
	waitTransferProgress(t, e, ih, 0)
	e.mut.Lock()
	handle := e.ts[ih].t
	e.mut.Unlock()
	if err := e.StopTorrent(ih); err != nil {
		t.Fatal(err)
	}
	// Allow in-flight chunks to settle; then paused bytes must remain stable.
	time.Sleep(200 * time.Millisecond)
	paused := e.GetTorrents()[ih]
	time.Sleep(300 * time.Millisecond)
	next := e.GetTorrents()[ih]
	if paused.Started || next.Started || next.Downloaded != paused.Downloaded {
		t.Fatalf("transfer did not pause: before=%+v after=%+v", paused, next)
	}
	select {
	case <-handle.Closed():
		t.Fatal("Stop closed the handle")
	default:
	}
	if err := e.StartTorrent(ih); err != nil {
		t.Fatal(err)
	}
	before := waitTransferProgress(t, e, ih, next.Downloaded)
	for range 10 {
		if err := e.StopTorrent(ih); err != nil {
			t.Fatal(err)
		}
		if err := e.StartTorrent(ih); err != nil {
			t.Fatal(err)
		}
	}
	waitTransferProgress(t, e, ih, before)
	e.mut.Lock()
	same := e.ts[ih].t == handle
	e.mut.Unlock()
	if !same {
		t.Fatal("Stop/Start replaced the handle")
	}
	select {
	case <-handle.Closed():
		t.Fatal("immediate restart left a closed handle")
	default:
	}
}
