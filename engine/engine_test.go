package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
)

// Sintel — a well-seeded, WebSeed-backed free torrent from
// https://webtorrent.io/free-torrents, used here as a stable fixture.
const (
	sintelInfoHash = "08ada5a7a6183aae1e09d831df6748d566095a10"
	sintelMagnet   = "magnet:?xt=urn:btih:" + sintelInfoHash + "&dn=Sintel"
)

func TestPercent(t *testing.T) {
	exact := []struct {
		n, total int64
		want     float32
	}{
		{0, 0, 0},
		{0, 100, 0},
		{50, 100, 50},
		{1, 4, 25},
		{3, 4, 75},
		{100, 100, 100},
	}
	for _, c := range exact {
		if got := percent(c.n, c.total); got != c.want {
			t.Errorf("percent(%d, %d) = %v, want %v", c.n, c.total, got, c.want)
		}
	}
	// truncation (not rounding) to 2 dp: 1/3 -> 33.33
	if got := percent(1, 3); got < 33.32 || got > 33.34 {
		t.Errorf("percent(1, 3) = %v, want ~33.33", got)
	}
}

func TestStr2IH(t *testing.T) {
	ih, err := str2ih(sintelInfoHash)
	if err != nil {
		t.Fatalf("str2ih(valid) returned error: %v", err)
	}
	if ih.HexString() != sintelInfoHash {
		t.Errorf("roundtrip = %s, want %s", ih.HexString(), sintelInfoHash)
	}
	// empty, non-hex, too short, too long must all error (and not panic)
	for _, bad := range []string{"", "nothexnothexnothexnothexnothexnothexnoth", "08ada5", sintelInfoHash + "00"} {
		if _, err := str2ih(bad); err == nil {
			t.Errorf("str2ih(%q) expected error, got nil", bad)
		}
	}
}

func TestParseMagnet(t *testing.T) {
	m, err := metainfo.ParseMagnetUri(sintelMagnet)
	if err != nil {
		t.Fatalf("ParseMagnetUri: %v", err)
	}
	if got := m.InfoHash.HexString(); got != sintelInfoHash {
		t.Errorf("infohash = %s, want %s", got, sintelInfoHash)
	}
	if m.DisplayName != "Sintel" {
		t.Errorf("display name = %q, want Sintel", m.DisplayName)
	}
}

// TestEngineAddAndStart spins up a real client, adds a magnet and starts it.
// It does not wait for metadata/peers, so it runs offline.
func TestEngineAddAndStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping engine integration test in -short mode")
	}
	e := New()
	t.Cleanup(func() { e.Close() })

	if err := e.Configure(Config{
		DownloadDirectory: t.TempDir(),
		IncomingPort:      freePort(t),
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if err := e.NewMagnet(sintelMagnet); err != nil {
		t.Fatalf("NewMagnet: %v", err)
	}

	ts := e.GetTorrents()
	if len(ts) != 1 {
		t.Fatalf("got %d torrents, want 1", len(ts))
	}
	if _, ok := ts[sintelInfoHash]; !ok {
		t.Fatalf("torrent %s not found", sintelInfoHash)
	}

	if err := e.StartTorrent(sintelInfoHash); err != nil {
		t.Fatalf("StartTorrent: %v", err)
	}
	if !e.GetTorrents()[sintelInfoHash].Started {
		t.Error("torrent not marked started")
	}
	// starting an already-started torrent should error
	if err := e.StartTorrent(sintelInfoHash); err == nil {
		t.Error("second StartTorrent expected error, got nil")
	}
}

func TestDefaultPort(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 50007}, {-1, 50007}, {65535, 50007}, {70000, 50007},
		{1, 1}, {3000, 3000}, {50007, 50007},
	}
	for _, c := range cases {
		if got := defaultPort(c.in); got != c.want {
			t.Errorf("defaultPort(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestSnapshotsOwnTheirModels(t *testing.T) {
	e := New()
	mustConfigureEngine(t, e)
	info := metainfo.Info{Name: "snapshot-fixture", PieceLength: 16, Length: 16, Pieces: make([]byte, 20)}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	spec := &torrent.TorrentSpec{InfoHash: metainfo.HashBytes(infoBytes), InfoBytes: infoBytes}
	if err := e.NewTorrent(spec); err != nil {
		t.Fatal(err)
	}
	ih := spec.InfoHash.HexString()
	before := e.GetTorrents()
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	if len(before[ih].Files) != 1 {
		t.Fatal("expected loaded file")
	}
	if before[ih].t != nil || before[ih].Files[0].f != nil {
		t.Fatal("snapshot leaked torrent handles")
	}
	if err := e.StartTorrent(ih); err != nil {
		t.Fatal(err)
	}
	afterJSON, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeJSON, afterJSON) {
		t.Fatal("previous snapshot changed after Start")
	}
	// A caller can edit its own DTOs and map without modifying the engine.
	before[ih].Name = "caller mutation"
	before[ih].Files[0].Path = "caller file mutation"
	delete(before, ih)
	next := e.GetTorrents()[ih]
	if next.Name != info.Name || next.Files[0].Path == "caller file mutation" || !next.Started {
		t.Fatalf("caller modified engine: %+v", next)
	}
}

func mustConfigureEngine(t *testing.T, e *Engine) Config {
	t.Helper()
	c := Config{DownloadDirectory: t.TempDir(), IncomingPort: freePort(t)}
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	return c
}

func TestConcurrentEngineLifecycleAndSnapshots(t *testing.T) {
	e := New()
	c := mustConfigureEngine(t, e)
	var wg sync.WaitGroup
	for worker := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := range 12 {
				ih := fmt.Sprintf("%040x", worker*100+iteration+1)
				// Operations may race with Close/Configure and legitimately fail;
				// they must never access unprotected maps, models or client pointers.
				_ = e.NewMagnet("magnet:?xt=urn:btih:" + ih)
				_ = e.StartTorrent(ih)
				_ = e.StopTorrent(ih)
				_ = e.DeleteTorrent(ih)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			snapshot := e.GetTorrents()
			_, _ = json.Marshal(snapshot)
			_ = e.Config()
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 2 {
			if err := e.Configure(c); err != nil {
				t.Error(err)
			}
		}
		_ = e.Close()
	}()
	wg.Wait()
}
