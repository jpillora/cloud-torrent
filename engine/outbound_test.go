package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anacrolix/torrent"
)

func TestAllEngineHTTPPathsUseDestinationPolicy(t *testing.T) {
	var reached atomic.Bool
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Store(true); w.Write([]byte("unexpected")) }))
	defer origin.Close()
	e := New()
	var config *torrent.ClientConfig
	e.newClient = func(c *torrent.ClientConfig) (*torrent.Client, error) { config = c; return torrent.NewClient(c) }
	if err := e.Configure(Config{DownloadDirectory: t.TempDir(), IncomingPort: freePort(t)}); err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if _, err := config.MetainfoSourcesClient.Get(origin.URL); err == nil {
		t.Fatal("magnet metadata source bypass")
	}
	req, _ := http.NewRequest("GET", origin.URL, nil)
	if _, err := config.WebTransport.RoundTrip(req); err == nil {
		t.Fatal("webseed bypass")
	}
	if _, err := config.TrackerDialContext(context.Background(), "tcp", origin.Listener.Addr().String()); err == nil {
		t.Fatal("tracker bypass")
	}
	if err := e.NewMagnet("magnet:?xt=urn:btih:0123456789012345678901234567890123456789&xs=" + origin.URL); err == nil {
		t.Fatal("unsafe source admitted")
	}
	if reached.Load() {
		t.Fatal("forbidden HTTP reached origin")
	}
}
