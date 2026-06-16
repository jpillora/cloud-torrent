package engine

import (
	"net"
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
