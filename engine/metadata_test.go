package engine

import (
	"bytes"
	"fmt"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
)

func metadataWaiterCount() int {
	var profile bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&profile, 2)
	return bytes.Count(profile.Bytes(), []byte("engine.(*Engine).watchMetadataLocked.func1"))
}

func waitMetadataWaiters(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if metadataWaiterCount() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("metadata waiters = %d, want %d", metadataWaiterCount(), want)
}

func TestMetadataWaitersCancelAndDuplicateAddsAreIdempotent(t *testing.T) {
	baseline := metadataWaiterCount()
	e := New()
	e.newClient = func(c *torrent.ClientConfig) (*torrent.Client, error) {
		c.NoDHT, c.DisableTrackers, c.NoDefaultPortForwarding = true, true, true
		c.DisableTCP, c.DisableUTP, c.DisableWebtorrent = true, true, true
		return torrent.NewClient(c)
	}
	c := mustConfigureEngine(t, e)
	add := func() {
		for i := 1; i <= 30; i++ {
			uri := fmt.Sprintf("magnet:?xt=urn:btih:%040x", i)
			if err := e.NewMagnet(uri); err != nil {
				t.Fatal(err)
			}
			if err := e.StartTorrent(fmt.Sprintf("%040x", i)); err != nil {
				t.Fatal(err)
			}
			for range 3 {
				if err := e.NewMagnet(uri); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	add()
	waitMetadataWaiters(t, baseline+30)
	e.mut.Lock()
	for _, model := range e.ts {
		if !model.autoStart || !model.Started {
			e.mut.Unlock()
			t.Fatal("duplicate add reset pending explicit Start")
		}
	}
	e.mut.Unlock()
	c.EnableUpload = !c.EnableUpload
	if err := e.Configure(c); err != nil {
		t.Fatal(err)
	}
	waitMetadataWaiters(t, baseline+30)
	for i := 1; i <= 30; i++ {
		if err := e.DeleteTorrent(fmt.Sprintf("%040x", i)); err != nil {
			t.Fatal(err)
		}
	}
	waitMetadataWaiters(t, baseline)
	if len(e.GetTorrents()) != 0 {
		t.Fatal("removed torrents remain cached")
	}
	add()
	waitMetadataWaiters(t, baseline+30)
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	waitMetadataWaiters(t, baseline)
}
