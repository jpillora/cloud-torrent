package server

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/jpillora/velox"
	veloxgo "github.com/jpillora/velox/go"
)

func syncWriterCount() int {
	var profile bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&profile, 2)
	return bytes.Count(profile.Bytes(), []byte("velox/go.(*eventSourceTransport).send.func1"))
}

func TestSlowSyncReaderDoesNotBlockOtherClients(t *testing.T) {
	baseline := syncWriterCount()
	s := &Server{Log: true}
	s.state.Users = map[string]string{}
	s.state.Throttle = time.Millisecond
	s.state.WriteTimeout = 3 * time.Second
	data := make([]byte, 4<<20)
	_, err := rand.Read(data)
	must(t, err)
	s.state.Stats.Title = base64.StdEncoding.EncodeToString(data)
	_ = velox.SyncHandler(&s.state)
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	client := &http.Client{Timeout: 8 * time.Second}
	request, err := http.NewRequest(http.MethodGet, srv.URL+"/sync?p=3", nil)
	must(t, err)
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Accept-Encoding", "gzip")
	resp, err := client.Do(request)
	must(t, err)
	defer resp.Body.Close()
	gz, err := gzip.NewReader(resp.Body)
	must(t, err)
	defer gz.Close()
	reader := bufio.NewReader(gz)
	next := func() veloxgo.Update {
		t.Helper()
		for {
			line, err := reader.ReadString('\n')
			must(t, err)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var update veloxgo.Update
			must(t, json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &update))
			if !update.Ping {
				return update
			}
		}
	}
	next()
	address, err := net.ResolveTCPAddr("tcp", strings.TrimPrefix(srv.URL, "http://"))
	must(t, err)
	slow, err := net.DialTCP("tcp", nil, address)
	must(t, err)
	defer slow.Close()
	must(t, slow.SetReadBuffer(1024))
	_, err = fmt.Fprintf(slow, "GET /sync?p=3 HTTP/1.1\r\nHost: %s\r\nAccept: text/event-stream\r\nAccept-Encoding: gzip\r\n\r\n", address)
	must(t, err)
	deadline := time.Now().Add(time.Second)
	for s.state.NumConnections() != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.state.NumConnections() != 2 {
		t.Fatal("slow client did not subscribe")
	}
	// Leave the socket completely unread until timeout cleanup has been proved.
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	s.state.Lock()
	s.state.Config.AutoStart = true
	s.state.Unlock()
	s.state.Push()
	for {
		update := next()
		if bytes.Contains(update.Ops, []byte(`"AutoStart":true`)) || bytes.Contains(update.Ops, []byte(`"AutoStart"],true`)) {
			break
		}
	}
	latency := time.Since(start)
	if latency > time.Second {
		t.Fatalf("healthy update blocked for %s", latency)
	}
	// A new subscription must also work while the first slow send is blocked.
	newRequest, err := http.NewRequest(http.MethodGet, srv.URL+"/sync?p=3&path=Config", nil)
	must(t, err)
	newRequest.Header.Set("Accept", "text/event-stream")
	newResp, err := client.Do(newRequest)
	must(t, err)
	defer newResp.Body.Close()
	if newResp.StatusCode != http.StatusOK {
		t.Fatal(newResp.Status)
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s.state.NumConnections() == 2 && syncWriterCount() == baseline {
			t.Logf("healthy update latency=%s; timed-out slow socket removed and send writers returned to baseline=%d", latency, baseline)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("slow reader retained: connections=%d writers=%d baseline=%d", s.state.NumConnections(), syncWriterCount(), baseline)
}
