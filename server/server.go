package server

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/jpillora/cloud-torrent/static"
	"github.com/jpillora/velox"
)

// Server is the cloud-torrent HTTP server. Its exported fields double as the
// command-line flags, parsed by github.com/jpillora/opts (see main.go).
type Server struct {
	Title      string `help:"Title of this instance" env:"TITLE"`
	Port       int    `help:"Listening port" env:"PORT"`
	Host       string `help:"Listening interface (default all)"`
	Auth       string `help:"Optional basic auth in form 'user:password'" env:"AUTH"`
	ConfigPath string `help:"Configuration file path"`
	KeyPath    string `help:"TLS Key file path"`
	CertPath   string `help:"TLS Certificate file path" short:"r"`
	Log        bool   `help:"Enable request logging"`
	Open       bool   `help:"Open now with your default browser"`

	state State
}

// Run starts the HTTP server: realtime state sync (velox) plus the embedded
// SPA. The torrent engine, action API and search are added to the handler in
// later tasks.
func (s *Server) Run(version string) error {
	host := s.Host
	if host == "" {
		host = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", host, s.Port)

	// demo: bump a counter every second so connected browsers observe live
	// updates. Replaced by the real poll/stats loops in the root-state task.
	go func() {
		for {
			time.Sleep(time.Second)
			s.state.Lock()
			s.state.Tick++
			s.state.Unlock()
			s.state.Push()
		}
	}()

	mux := http.NewServeMux()
	// realtime state sync — SyncHandler binds the EMBEDDED velox.State, so
	// s.state.Push() reaches these connections (velox.Sync would wrap a separate
	// State). Kept out of the gzip wrap so SSE streams unbuffered.
	mux.Handle("/sync", velox.SyncHandler(&s.state))
	mux.Handle("/js/velox.js", velox.JS)
	// everything else: the embedded SPA, gzipped
	mux.Handle("/", gziphandler.GzipHandler(static.Handler()))

	log.Printf("Listening at http://%s", addr)
	return http.ListenAndServe(addr, mux)
}
