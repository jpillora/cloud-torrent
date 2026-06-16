package server

import (
	"net"
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
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
