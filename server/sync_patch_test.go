package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jpillora/cloud-torrent/engine"
	"github.com/jpillora/velox"
	veloxgo "github.com/jpillora/velox/go"
)

func TestSyncPatchesIndividualArrayElements(t *testing.T) {
	s := &Server{}
	s.state.Users = map[string]string{}
	s.state.Torrents = map[string]*engine.Torrent{"fixture": {Name: "fixture", Files: make([]*engine.File, 5000)}}
	s.state.Downloads = &FsNode{Directory: true, Children: make([]*FsNode, 5000)}
	for i := range 5000 {
		s.state.Torrents["fixture"].Files[i] = &engine.File{Path: fmt.Sprintf("file-%04d", i), Size: 100}
		s.state.Downloads.Children[i] = &FsNode{Name: fmt.Sprintf("file-%04d", i), Size: 100, Children: []*FsNode{}}
	}
	h := velox.SyncHandler(&s.state)
	srv := httptest.NewServer(h)
	defer srv.Close()
	r, err := http.NewRequest(http.MethodGet, srv.URL+"?p=3", nil)
	must(t, err)
	r.Header.Set("Accept", "text/event-stream")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(r)
	must(t, err)
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	next := func() (veloxgo.Update, []byte) {
		t.Helper()
		for {
			line, err := reader.ReadString('\n')
			must(t, err)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			b := []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			var update veloxgo.Update
			must(t, json.Unmarshal(b, &update))
			if update.Ping {
				continue
			}
			return update, b
		}
	}
	initial, full := next()
	if len(initial.Body) == 0 || initial.Proto != 3 {
		t.Fatalf("expected v3 snapshot: %s", full[:min(200, len(full))])
	}
	s.state.Lock()
	s.state.Torrents["fixture"].Files[2500].Percent = 1
	s.state.Torrents["fixture"].Files[2500].Completed = 1
	s.state.Downloads.Children[2500].Modified = time.Unix(123, 0)
	s.state.Unlock()
	s.state.Push()
	update, delta := next()
	if len(update.Body) != 0 || len(delta) > 2048 || !bytes.Contains(update.Ops, []byte(`"Files",2500]`)) || !bytes.Contains(update.Ops, []byte(`"Children",2500]`)) {
		t.Fatalf("single-element update replaced collections (%d bytes): %.2000s", len(delta), delta)
	}
	t.Logf("5000 torrent files + 5000 filesystem nodes: snapshot=%d bytes, single-file delta=%d bytes", len(full), len(delta))
}
