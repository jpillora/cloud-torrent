package server

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	velox "github.com/jpillora/velox/go"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jpillora/cloud-torrent/engine"
)

func TestAPIRejectsNonPost(t *testing.T) {
	s := &Server{}
	rr := httptest.NewRecorder()
	s.handleAPI(rr, httptest.NewRequest(http.MethodGet, "/api/magnet", nil))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("GET /api/magnet = %d, want 400", rr.Code)
	}
}

func TestAPIUnknownAction(t *testing.T) {
	s := &Server{}
	rr := httptest.NewRecorder()
	s.handleAPI(rr, httptest.NewRequest(http.MethodPost, "/api/bogus", strings.NewReader("{}")))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("unknown action = %d, want 400", rr.Code)
	}
}

// TestAPIMagnetThenStart adds a magnet and starts it through the API. It does
// not wait for metadata/peers, so it runs offline.
func TestAPIMagnetThenStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping engine integration test in -short mode")
	}
	s := &Server{engine: engine.New()}
	t.Cleanup(func() { s.engine.Close() })
	if err := s.engine.Configure(engine.Config{
		DownloadDirectory: t.TempDir(),
		IncomingPort:      freePort(t),
	}); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	post(t, s, "/api/magnet", `{"uri":"`+sintelMagnet+`"}`, http.StatusOK)
	if _, ok := s.engine.GetTorrents()[sintelInfoHash]; !ok {
		t.Fatalf("torrent %s not added via API", sintelInfoHash)
	}

	post(t, s, "/api/torrent", `{"action":"start","infohash":"`+sintelInfoHash+`"}`, http.StatusOK)
	if !s.engine.GetTorrents()[sintelInfoHash].Started {
		t.Error("torrent not started via API")
	}

	// an invalid action is rejected
	post(t, s, "/api/torrent", `{"action":"bogus","infohash":"`+sintelInfoHash+`"}`, http.StatusBadRequest)
}

func post(t *testing.T, s *Server, path, body string, wantCode int) {
	t.Helper()
	rr := httptest.NewRecorder()
	s.handleAPI(rr, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	if rr.Code != wantCode {
		t.Fatalf("POST %s = %d (%s), want %d", path, rr.Code, rr.Body.String(), wantCode)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	for range 100 {
		tcp, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatal(err)
		}
		port := tcp.Addr().(*net.TCPAddr).Port
		udp, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
		tcp.Close()
		if err != nil {
			continue
		}
		udp.Close()
		return port
	}
	t.Fatal("could not find a free TCP and UDP port")
	return 0
}

// Real HTTP actions run while the poller and independent Push callers serialize
// state, exercising the engine -> snapshot -> realtime publication boundary.
func TestConcurrentAPIActionsAndPush(t *testing.T) {
	s := &Server{engine: engine.New()}
	if err := s.engine.Configure(engine.Config{DownloadDirectory: t.TempDir(), IncomingPort: freePort(t)}); err != nil {
		t.Fatal(err)
	}
	defer s.engine.Close()
	_ = velox.SyncHandler(&s.state)
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	client := srv.Client()
	client.Timeout = 10 * time.Second
	postJSON := func(path, body string) {
		t.Helper()
		resp, err := client.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Error(err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Errorf("%s: %d %s", path, resp.StatusCode, b)
		}
	}
	var wg sync.WaitGroup
	for worker := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for iteration := range 10 {
				ih := fmt.Sprintf("%040x", worker*100+iteration+1)
				postJSON("/api/magnet", `{"uri":"magnet:?xt=urn:btih:`+ih+`"}`)
				for _, action := range []string{"start", "stop", "delete"} {
					postJSON("/api/torrent", `{"action":"`+action+`","infohash":"`+ih+`"}`)
				}
			}
		}()
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 100 {
			torrents := s.engine.GetTorrents()
			s.state.Lock()
			s.state.Torrents = torrents
			s.state.Unlock()
			s.state.Push()
		}
	}()
	go func() {
		defer wg.Done()
		for range 100 {
			s.state.Push()
		}
	}()
	wg.Wait()
	// No poll is running now: a successful action itself must publish state.
	postJSON("/api/magnet", `{"uri":"`+sintelMagnet+`"}`)
	postJSON("/api/torrent", `{"action":"start","infohash":"`+sintelInfoHash+`"}`)
	s.state.Lock()
	tr := s.state.Torrents[sintelInfoHash]
	started := tr != nil && tr.Started
	s.state.Unlock()
	if !started {
		t.Fatal("successful API action did not publish its fresh snapshot")
	}
}
