package server

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/jpillora/velox"
)

func TestDownloadCompressionPolicy(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("download content\n"), 100)
	for _, name := range []string{"movie.mp4", "photo.jpg", "archive.zip", "notes.txt", "data.json", "script.js"} {
		must(t, os.WriteFile(filepath.Join(dir, name), data, 0o600))
	}
	must(t, os.Mkdir(filepath.Join(dir, "folder"), 0o755))
	mustWrite(t, filepath.Join(dir, "folder", "notes.txt"), "hello")
	h := compressResponses(mount(dir))
	for _, name := range []string{"movie.mp4", "photo.jpg", "archive.zip", "folder", "notes.txt", "data.json", "script.js"} {
		r := httptest.NewRequest(http.MethodGet, "/download/"+name, nil)
		r.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d", name, w.Code)
		}
		compressed := name == "notes.txt" || name == "data.json" || name == "script.js"
		if (w.Header().Get("Content-Encoding") == "gzip") != compressed {
			t.Fatalf("%s encoding=%s", name, w.Header().Get("Content-Encoding"))
		}
		body := w.Body.Bytes()
		if compressed {
			reader, err := gzip.NewReader(bytes.NewReader(body))
			must(t, err)
			body, err = io.ReadAll(reader)
			must(t, err)
			reader.Close()
		}
		if name != "folder" && !bytes.Equal(body, data) {
			t.Fatalf("%s content changed", name)
		}
	}
}

func TestGzippedRealtimeStreamFlushesImmediately(t *testing.T) {
	s := &Server{}
	s.state.Users = map[string]string{}
	_ = velox.SyncHandler(&s.state)
	srv := httptest.NewServer(compressResponses(http.HandlerFunc(s.handleSync)))
	defer srv.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	r, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	must(t, err)
	r.Header.Set("Accept-Encoding", "gzip")
	r.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(r)
	must(t, err)
	defer resp.Body.Close()
	if resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatal("SSE did not retain gzip")
	}
	reader, err := gzip.NewReader(resp.Body)
	must(t, err)
	defer reader.Close()
	lines := bufio.NewReader(reader)
	for {
		line, err := lines.ReadString('\n')
		must(t, err)
		if strings.HasPrefix(line, "data:") {
			break
		}
	}
}

type compressionCounter struct {
	header http.Header
	bytes  int64
}

func (w *compressionCounter) Header() http.Header { return w.header }
func (w *compressionCounter) WriteHeader(int)     {}
func (w *compressionCounter) Write(b []byte) (int, error) {
	w.bytes += int64(len(b))
	return len(b), nil
}

func BenchmarkMediaDownloadCompression(b *testing.B) {
	dir := b.TempDir()
	data := make([]byte, 8<<20)
	if _, err := rand.Read(data); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "movie.mp4"), data, 0o600); err != nil {
		b.Fatal(err)
	}
	oldWrap, _ := gziphandler.NewGzipLevelAndMinSize(gzip.DefaultCompression, 0)
	base := mount(dir)
	for name, h := range map[string]http.Handler{"previous": oldWrap(base), "mime-policy": compressResponses(base)} {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			for range b.N {
				r := httptest.NewRequest(http.MethodGet, "/download/movie.mp4", nil)
				r.Header.Set("Accept-Encoding", "gzip")
				w := &compressionCounter{header: make(http.Header)}
				h.ServeHTTP(w, r)
			}
		})
	}
}
