package engine

import (
	"bytes"
	"crypto/sha1"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// Real torrent pieces served over HTTP at a bounded speed, allowing lifecycle
// tests to assert transfer behavior rather than only model flags.
func newTransferFixture(t *testing.T) (*Engine, Config, string) {
	return newTransferFixtureFiles(t, false)
}

func newTransferFixtureFiles(t *testing.T, multi bool) (*Engine, Config, string) {
	t.Helper()
	data := bytes.Repeat([]byte("0123456789abcdef"), 512*1024)
	seed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := data
		if multi {
			if r.URL.Path == "/fixture/first.bin" {
				content = data[:len(data)/2]
			} else if r.URL.Path == "/fixture/second.bin" {
				content = data[len(data)/2:]
			} else {
				http.NotFound(w, r)
				return
			}
		}
		http.ServeContent(throttledWriter{w, r}, r, "fixture.bin", time.Time{}, bytes.NewReader(content))
	}))
	t.Cleanup(seed.Close)
	const pieceLength = 64 * 1024
	var pieces []byte
	for offset := 0; offset < len(data); offset += pieceLength {
		hash := sha1.Sum(data[offset:min(offset+pieceLength, len(data))])
		pieces = append(pieces, hash[:]...)
	}
	info := metainfo.Info{Name: "fixture.bin", Length: int64(len(data)), PieceLength: pieceLength, Pieces: pieces}
	webseed := seed.URL + "/fixture.bin"
	if multi {
		info.Name, info.Length = "fixture", 0
		info.Files = []metainfo.FileInfo{{Length: int64(len(data) / 2), Path: []string{"first.bin"}}, {Length: int64(len(data) / 2), Path: []string{"second.bin"}}}
		webseed = seed.URL + "/"
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	spec := &torrent.TorrentSpec{AddTorrentOpts: torrent.AddTorrentOpts{InfoHash: metainfo.HashBytes(infoBytes), InfoBytes: infoBytes}, Webseeds: []string{webseed}}
	ih := spec.InfoHash.HexString()
	e := New()
	c := mustConfigureEngine(t, e)
	if err := e.NewTorrent(spec); err != nil {
		t.Fatal(err)
	}

	return e, c, ih
}

func TestStartFileTransfersAndStartAllSchedulesRemaining(t *testing.T) {
	e, _, ih := newTransferFixtureFiles(t, true)
	if err := e.StartFile(ih, "fixture/first.bin"); err != nil {
		t.Fatal(err)
	}
	waitTransferProgress(t, e, ih, 0)
	e.mut.Lock()
	files := e.ts[ih].t.Files()
	firstPriority, secondPriority := files[0].Priority(), files[1].Priority()
	firstBytes, secondBytes := files[0].BytesCompleted(), files[1].BytesCompleted()
	e.mut.Unlock()
	if firstPriority == torrent.PiecePriorityNone || secondPriority != torrent.PiecePriorityNone || firstBytes == 0 || secondBytes != 0 {
		t.Fatalf("selective download: priorities %v/%v bytes %d/%d", firstPriority, secondPriority, firstBytes, secondBytes)
	}
	if err := e.StartTorrent(ih); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if files[1].Priority() != torrent.PiecePriorityNone && files[1].BytesCompleted() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Start all did not transfer the remaining file")
}

func TestStartBeforeMetadataTransfersWhenInfoArrives(t *testing.T) {
	for _, mode := range []string{"explicit", "reconfigured", "stopped", "automatic"} {
		t.Run(mode, func(t *testing.T) {
			e, c, ih := newTransferFixture(t)
			e.mut.Lock()
			spec := cloneSpec(e.ts[ih].spec)
			e.mut.Unlock()
			if err := e.DeleteTorrent(ih); err != nil {
				t.Fatal(err)
			}
			if mode == "automatic" {
				c.AutoStart = true
				if err := e.Configure(c); err != nil {
					t.Fatal(err)
				}
			}
			pending := cloneSpec(spec)
			pending.InfoBytes = nil
			if err := e.NewTorrent(pending); err != nil {
				t.Fatal(err)
			}
			if e.GetTorrents()[ih].Loaded {
				t.Fatal("fixture metadata arrived before explicit Start")
			}
			if mode != "automatic" {
				if err := e.StartTorrent(ih); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "reconfigured" {
				c.EnableUpload = !c.EnableUpload
				if err := e.Configure(c); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "stopped" {
				if err := e.StopTorrent(ih); err != nil {
					t.Fatal(err)
				}
			}
			e.mut.Lock()
			err := e.ts[ih].t.SetInfoBytes(spec.InfoBytes)
			e.mut.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			if mode != "stopped" {
				waitTransferProgress(t, e, ih, 0)
				return
			}
			time.Sleep(300 * time.Millisecond)
			tr := e.GetTorrents()[ih]
			if tr.Started || tr.Downloaded != 0 {
				t.Fatalf("metadata revived cancelled start: %+v", tr)
			}
		})
	}
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
