package engine

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/jpillora/cloud-torrent/internal/outbound"
)

func isolatedBandwidthEngine(t *testing.T, dir string, up, down int64, seed bool) *Engine {
	t.Helper()
	policy, _ := outbound.New("127.0.0.1/32")
	e := New(policy)
	e.newClient = func(c *torrent.ClientConfig) (*torrent.Client, error) {
		c.NoDHT = true
		c.DisableTrackers = true
		c.NoDefaultPortForwarding = true
		c.DisablePEX = true
		c.DisableUTP = true
		c.DisableWebtorrent = true
		return torrent.NewClient(c)
	}
	c := Config{DownloadDirectory: dir, IncomingPort: freePort(t), EnableUpload: seed, EnableSeeding: seed, UploadLimitKiB: up, DownloadLimitKiB: down}
	bandwidthMust(t, e.Configure(c))
	t.Cleanup(func() { e.Close() })
	return e
}
func bandwidthSpecs(t *testing.T) ([]byte, []*torrent.TorrentSpec) {
	t.Helper()
	data := bytes.Repeat([]byte("bandwidth legal fixture 012345678"), 32768)
	var pieces []byte
	for start := 0; start < len(data); start += 16 << 10 {
		hash := sha1.Sum(data[start:min(start+(16<<10), len(data))])
		pieces = append(pieces, hash[:]...)
	}
	var specs []*torrent.TorrentSpec
	for i := range 2 {
		info := metainfo.Info{Name: fmt.Sprintf("bandwidth-%d.bin", i), Length: int64(len(data)), PieceLength: 16 << 10, Pieces: pieces}
		encoded, err := bencode.Marshal(info)
		bandwidthMust(t, err)
		specs = append(specs, &torrent.TorrentSpec{InfoHash: metainfo.HashBytes(encoded), InfoBytes: encoded})
	}
	return data, specs
}

// Local TCP peers exercise actual peer upload and download independently. A
// webseed variant checks the separate HTTP response-body payload path.
func TestBandwidthAggregateAndRuntimeUpdates(t *testing.T) {
	for _, mode := range []string{"peer-upload", "peer-download", "webseed-download"} {
		t.Run(mode, func(t *testing.T) {
			data, specs := bandwidthSpecs(t)
			seedDir := t.TempDir()
			up, down := int64(0), int64(128)
			if mode == "peer-upload" {
				up, down = 128, 0
			}
			var seed *Engine
			if mode != "webseed-download" {
				for i := range specs {
					bandwidthMust(t, os.WriteFile(filepath.Join(seedDir, fmt.Sprintf("bandwidth-%d.bin", i)), data, 0600))
				}
				seed = isolatedBandwidthEngine(t, seedDir, up, 0, true)
				for _, spec := range specs {
					bandwidthMust(t, seed.NewTorrent(spec))
					bandwidthMust(t, seed.StartTorrent(spec.InfoHash.HexString()))
					waitSessionBytes(t, seed, spec.InfoHash.HexString(), int64(len(data)))
				}
			} else {
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.ServeContent(w, r, "fixture", time.Time{}, bytes.NewReader(data))
				}))
				defer origin.Close()
				for i, spec := range specs {
					spec.Webseeds = []string{fmt.Sprintf("%s/bandwidth-%d.bin", origin.URL, i)}
				}
			}
			leecher := isolatedBandwidthEngine(t, t.TempDir(), 0, down, false)
			for _, spec := range specs {
				bandwidthMust(t, leecher.NewTorrent(spec))
				bandwidthMust(t, leecher.StartTorrent(spec.InfoHash.HexString()))
				if seed != nil {
					leecher.ts[spec.InfoHash.HexString()].t.AddClientPeer(seed.client)
				}
			}
			target := leecher
			if mode == "peer-upload" {
				target = seed
			}
			clientBefore := target.client
			sumBytes := func() int64 {
				var total int64
				leecher.mut.Lock()
				defer leecher.mut.Unlock()
				for _, model := range leecher.ts {
					total += model.t.BytesCompleted()
				}
				return total
			}
			// Upstream starts webseed scheduling on a five-second timer.
			deadlineStart := time.Now().Add(8 * time.Second)
			for sumBytes() == 0 && time.Now().Before(deadlineStart) {
				time.Sleep(10 * time.Millisecond)
			}
			baseline := sumBytes()
			started := time.Now()
			time.Sleep(2 * time.Second)
			done := sumBytes() - baseline
			elapsed := time.Since(started)
			upper := int64(128*1024*elapsed.Seconds()) + bandwidthBurst + (64 << 10)
			if done <= 16<<10 || done > upper {
				t.Fatalf("aggregate %s bytes %d outside (16KiB,%d] in %s", mode, done, upper, elapsed)
			}
			c := target.Config()
			c.UploadLimitKiB = 0
			c.DownloadLimitKiB = 0
			bandwidthMust(t, target.Configure(c))
			if target.client != clientBefore {
				t.Fatal("limiter change replaced client")
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if sumBytes() == int64(len(data)*len(specs)) {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if sumBytes() != int64(len(data)*len(specs)) {
				t.Fatal("unlimited runtime update did not complete payload", sumBytes())
			}
			for _, spec := range specs {
				if !leecher.GetTorrents()[spec.InfoHash.HexString()].Started {
					t.Fatal("limit change lost start intent")
				}
			}
		})
	}
}
func TestBandwidthInvalidValuesAreAtomic(t *testing.T) {
	e := isolatedBandwidthEngine(t, t.TempDir(), 64, 128, false)
	before := e.Config()
	client := e.client
	for _, value := range []int64{-1, maxBandwidthKiB + 1, 1 << 62} {
		for _, upload := range []bool{false, true} {
			c := before
			if upload {
				c.UploadLimitKiB = value
			} else {
				c.DownloadLimitKiB = value
			}
			if e.Configure(c) == nil {
				t.Fatal("invalid cap accepted", value)
			}
			if e.Config() != before || e.client != client {
				t.Fatal("invalid cap changed active settings")
			}
		}
	}
	c := before
	bandwidthMust(t, json.Unmarshal([]byte(`{"DownloadLimitKiB":16}`), &c))
	expected := before
	expected.DownloadLimitKiB = 16
	if c != expected {
		t.Fatal("decoding a setting lost existing defaults")
	}
	for _, raw := range []string{`null`, `-1`, `1.5`, `"invalid"`, `1e100`, `9223372036854775808`, `NaN`, `Infinity`} {
		for _, key := range []string{"DownloadLimitKiB", "UploadLimitKiB", "downloadlimitkib"} {
			c := before
			if err := json.Unmarshal([]byte(fmt.Sprintf(`{%q:%s}`, key, raw)), &c); err == nil {
				t.Fatal("invalid JSON cap accepted", key, raw)
			}
			if c != before {
				t.Fatal("decode changed config on error")
			}
		}
	}
}

func bandwidthMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestBandwidthUpdatePreservesSelectionAndPausedTorrent(t *testing.T) {
	e := isolatedBandwidthEngine(t, t.TempDir(), 0, 0, false)
	selected := sessionFixture(t)
	bandwidthMust(t, e.NewTorrent(selected))
	ih := selected.InfoHash.HexString()
	bandwidthMust(t, e.StartFile(ih, "restart-fixture/one"))
	bandwidthMust(t, e.NewMagnet("magnet:?xt=urn:btih:2222222222222222222222222222222222222222"))
	client := e.client
	c := e.Config()
	c.DownloadLimitKiB = 64
	c.UploadLimitKiB = 32
	bandwidthMust(t, e.Configure(c))
	if client != e.client {
		t.Fatal("limit update replaced client")
	}
	snapshot := e.GetTorrents()
	if !snapshot[ih].Files[0].Started || snapshot[ih].Files[1].Started || snapshot["2222222222222222222222222222222222222222"].Started {
		t.Fatal("limit update lost selection or paused intent")
	}
}
