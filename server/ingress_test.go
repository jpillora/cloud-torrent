package server

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jpillora/cloud-torrent/engine"
	"github.com/jpillora/velox"
)

func ingressTestServer(t *testing.T, s *Server, wrap func(http.Handler) http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(nil)
	srv.Config = s.httpServer("")
	if wrap != nil {
		srv.Config.Handler = wrap(srv.Config.Handler)
	}
	srv.Start()
	t.Cleanup(func() { srv.CloseClientConnections(); srv.Close() })
	return srv
}
func ingressSocket(t *testing.T, srv *httptest.Server) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	must(t, err)
	t.Cleanup(func() { conn.Close() })
	must(t, conn.SetDeadline(time.Now().Add(3*time.Second)))
	return conn
}
func fastIngress() ingressLimits {
	return ingressLimits{header: 100 * time.Millisecond, idle: 150 * time.Millisecond, apiBody: 80 * time.Millisecond, uploadBody: 120 * time.Millisecond}
}
func TestIngressStalledHeadersIdleAndBodies(t *testing.T) {
	s := &Server{ingress: fastIngress()}
	srv := ingressTestServer(t, s, nil)
	t.Run("header", func(t *testing.T) {
		c := ingressSocket(t, srv)
		start := time.Now()
		_, err := io.WriteString(c, "GET / HTTP/1.1\r\nHost: incomplete")
		must(t, err)
		_, err = bufio.NewReader(c).ReadString('\n')
		if err == nil {
			t.Fatal("incomplete headers accepted")
		}
		if time.Since(start) > time.Second {
			t.Fatal("header deadline not enforced")
		}
	})
	t.Run("idle keepalive", func(t *testing.T) {
		c := ingressSocket(t, srv)
		_, err := fmt.Fprintf(c, "GET /js/velox.js HTTP/1.1\r\nHost: %s\r\n\r\n", srv.Listener.Addr())
		must(t, err)
		reader := bufio.NewReader(c)
		resp, err := http.ReadResponse(reader, nil)
		must(t, err)
		_, err = io.Copy(io.Discard, resp.Body)
		must(t, err)
		resp.Body.Close()
		start := time.Now()
		_, err = reader.ReadByte()
		if err == nil || time.Since(start) > time.Second {
			t.Fatal("idle connection remained open")
		}
	})
	for _, path := range []string{"/api/magnet", "/api/torrentfile", "/api/configure"} {
		t.Run(path, func(t *testing.T) {
			c := ingressSocket(t, srv)
			start := time.Now()
			_, err := fmt.Fprintf(c, "POST %s HTTP/1.1\r\nHost: %s\r\nContent-Length: 20\r\n\r\n{", path, srv.Listener.Addr())
			must(t, err)
			resp, err := http.ReadResponse(bufio.NewReader(c), nil)
			must(t, err)
			defer resp.Body.Close()
			if resp.StatusCode != 408 || !resp.Close || time.Since(start) > time.Second {
				t.Fatalf("stalled body: %d close=%v elapsed=%v", resp.StatusCode, resp.Close, time.Since(start))
			}
		})
	}
	t.Run("chunked", func(t *testing.T) {
		c := ingressSocket(t, srv)
		_, err := fmt.Fprintf(c, "POST /api/magnet HTTP/1.1\r\nHost: %s\r\nTransfer-Encoding: chunked\r\n\r\n1\r\n{\r\n", srv.Listener.Addr())
		must(t, err)
		resp, err := http.ReadResponse(bufio.NewReader(c), nil)
		must(t, err)
		defer resp.Body.Close()
		if resp.StatusCode != 408 {
			t.Fatalf("chunked stall: %d", resp.StatusCode)
		}
	})
}
func TestIngressOversizeAndDisconnectReleaseHandler(t *testing.T) {
	s := &Server{ingress: fastIngress()}
	finished := make(chan struct{}, 8)
	srv := ingressTestServer(t, s, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() { finished <- struct{}{} }()
			next.ServeHTTP(w, r)
		})
	})
	c := ingressSocket(t, srv)
	_, err := fmt.Fprintf(c, "POST /api/torrentfile HTTP/1.1\r\nHost: %s\r\nContent-Length: %d\r\n\r\n", srv.Listener.Addr(), maxRequestBytes+1)
	must(t, err)
	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	must(t, err)
	resp.Body.Close()
	if resp.StatusCode != 413 {
		t.Fatalf("oversize: %d", resp.StatusCode)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("oversize handler retained")
	}
	c = ingressSocket(t, srv)
	_, err = fmt.Fprintf(c, "POST /api/magnet HTTP/1.1\r\nHost: %s\r\nContent-Length: 100\r\n\r\n{", srv.Listener.Addr())
	must(t, err)
	c.Close()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("disconnected body retained handler")
	}
	// Unknown/chunked lengths must not silently truncate an oversized JSON body.
	request := httptest.NewRequest("POST", "/api/magnet", bytes.NewReader(bytes.Repeat([]byte(" "), maxRequestBytes+1)))
	request.ContentLength = -1
	recorder := httptest.NewRecorder()
	s.handler().ServeHTTP(recorder, request)
	if recorder.Code != 413 {
		t.Fatalf("streamed oversize: %d", recorder.Code)
	}
}
func TestIngressBodyDeadlineClearsAndKeepsAuthentication(t *testing.T) {
	s := &Server{ingress: fastIngress()}
	handler := s.protectIngress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		time.Sleep(3 * s.ingress.apiBody)
		io.WriteString(w, "complete")
	}))
	srv := httptest.NewUnstartedServer(handler)
	srv.Config = s.httpServer("")
	srv.Config.Handler = handler
	srv.Start()
	defer srv.Close()
	resp, err := http.Post(srv.URL, "text/plain", strings.NewReader("legal"))
	must(t, err)
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	must(t, err)
	if string(body) != "complete" {
		t.Fatalf("completed body deadline affected processing: %s", body)
	}
	secured := &Server{Auth: "jaime:local-test", ingress: fastIngress()}
	auth := ingressTestServer(t, secured, nil)
	resp, err = http.Get(auth.URL + "/js/velox.js")
	must(t, err)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("auth bypass: %d", resp.StatusCode)
	}
	req, err := http.NewRequest("GET", auth.URL+"/js/velox.js", nil)
	must(t, err)
	req.SetBasicAuth("jaime", "local-test")
	resp, err = http.DefaultClient.Do(req)
	must(t, err)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("healthy auth: %d", resp.StatusCode)
	}
}

type ingressSlowWriter struct{ http.ResponseWriter }

func (w ingressSlowWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w ingressSlowWriter) Write(p []byte) (int, error) {
	total := 0
	for len(p) > 0 {
		size := min(len(p), 8192)
		time.Sleep(5 * time.Millisecond)
		n, err := w.ResponseWriter.Write(p[:size])
		total += n
		p = p[n:]
		if err != nil {
			return total, err
		}
		if n != size {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}
func TestIngressLongSyncFileZipAndRange(t *testing.T) {
	s := &Server{engine: engine.New(), ingress: fastIngress()}
	defer s.engine.Close()
	dir := t.TempDir()
	must(t, s.engine.Configure(engine.Config{DownloadDirectory: dir, IncomingPort: freePort(t)}))
	must(t, os.Mkdir(filepath.Join(dir, "folder"), 0700))
	data := make([]byte, 512<<10)
	_, err := rand.Read(data)
	must(t, err)
	must(t, os.WriteFile(filepath.Join(dir, "folder", "legal.bin"), data, 0600))
	s.state.Users = map[string]string{}
	s.state.Throttle = time.Millisecond
	_ = velox.SyncHandler(&s.state)
	srv := ingressTestServer(t, s, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/download/") {
				w = ingressSlowWriter{w}
			}
			next.ServeHTTP(w, r)
		})
	})
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	for _, kind := range []string{"file", "zip", "range"} {
		t.Run(kind, func(t *testing.T) {
			path := "/download/folder/legal.bin"
			if kind == "zip" {
				path = "/download/folder"
			}
			req, err := http.NewRequest("GET", srv.URL+path, nil)
			must(t, err)
			if kind == "range" {
				req.Header.Set("Range", "bytes=0-262143")
			}
			start := time.Now()
			resp, err := client.Do(req)
			must(t, err)
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			must(t, err)
			if time.Since(start) <= s.ingress.apiBody {
				t.Fatal("fixture did not outlast short ingress deadline")
			}
			switch kind {
			case "file":
				if resp.StatusCode != 200 || !bytes.Equal(body, data) {
					t.Fatal("long file truncated")
				}
			case "range":
				if resp.StatusCode != 206 || !bytes.Equal(body, data[:262144]) {
					t.Fatal("long range truncated")
				}
			case "zip":
				archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
				must(t, err)
				if len(archive.File) != 1 {
					t.Fatal("zip entries")
				}
				file, err := archive.File[0].Open()
				must(t, err)
				got, err := io.ReadAll(file)
				file.Close()
				must(t, err)
				if !bytes.Equal(got, data) {
					t.Fatal("long zip truncated")
				}
			}
		})
	}
	req, err := http.NewRequest("GET", srv.URL+"/sync?p=3", nil)
	must(t, err)
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	must(t, err)
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	next := func() string {
		t.Helper()
		for {
			line, err := reader.ReadString('\n')
			must(t, err)
			if strings.HasPrefix(line, "data:") {
				return line
			}
		}
	}
	next()
	time.Sleep(3 * s.ingress.apiBody)
	s.state.Lock()
	s.state.Stats.Title = "sync beyond body timeout"
	s.state.Unlock()
	s.state.Push()
	found := false
	for i := 0; i < 5; i++ {
		if strings.Contains(next(), "sync beyond body timeout") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("healthy sync failed after ingress deadline")
	}
}
func TestIngressProductionDefaults(t *testing.T) {
	srv := (&Server{}).httpServer("")
	if srv.ReadHeaderTimeout != 5*time.Second || srv.IdleTimeout != 60*time.Second || srv.ReadTimeout != 0 || srv.WriteTimeout != 0 {
		t.Fatalf("unsafe server defaults: %+v", srv)
	}
	limits := (&Server{}).ingressLimits()
	if limits.apiBody != 15*time.Second || limits.uploadBody != 60*time.Second {
		t.Fatal("unexpected body defaults")
	}
}
