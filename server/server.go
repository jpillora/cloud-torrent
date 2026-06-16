package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/jpillora/cloud-torrent/engine"
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

	engine *engine.Engine
	state  State
}

// Run wires the torrent engine and realtime state, starts the poll/stats loops,
// and serves the API-less HTTP surface (sync + embedded SPA). The action API
// and search are added to the handler in later tasks.
func (s *Server) Run(version string) error {
	// static stats metadata
	s.state.Stats.Title = s.Title
	s.state.Stats.Version = version
	s.state.Stats.Runtime = strings.TrimPrefix(runtime.Version(), "go")
	s.state.Stats.Uptime = time.Now()
	s.state.Users = map[string]string{}

	// initialise the embedded velox.State (sets its marshaller + version
	// baseline) before any Push, so a fresh connection receives the full state
	// rather than a stale partial delta. Connections are driven via state.Handle
	// (in handleSync) so we can track per-connection users.
	_ = velox.SyncHandler(&s.state)

	// torrent engine + persisted config
	s.engine = engine.New()
	c := engine.Config{
		DownloadDirectory: "./downloads",
		EnableUpload:      true,
		AutoStart:         true,
	}
	if b, err := os.ReadFile(s.ConfigPath); err == nil && len(b) > 0 {
		if err := json.Unmarshal(b, &c); err != nil {
			return fmt.Errorf("malformed configuration: %w", err)
		}
	}
	if err := s.reconfigure(c); err != nil {
		return fmt.Errorf("initial configure failed: %w", err)
	}

	// poll torrents + downloads every second
	go func() {
		for {
			s.state.Lock()
			s.state.Torrents = s.engine.GetTorrents()
			s.state.Downloads = listFiles(s.engine.Config().DownloadDirectory)
			s.state.Unlock()
			s.state.Push()
			time.Sleep(time.Second)
		}
	}()
	// refresh system stats every five seconds
	go func() {
		for {
			dir := s.engine.Config().DownloadDirectory
			s.state.Lock()
			s.state.Stats.System.load(dir)
			s.state.Unlock()
			s.state.Push()
			time.Sleep(5 * time.Second)
		}
	}()

	host := s.Host
	if host == "" {
		host = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", host, s.Port)

	mux := http.NewServeMux()
	mux.HandleFunc("/sync", s.handleSync)
	mux.HandleFunc("/api/", s.handleAPI)
	mux.Handle("/js/velox.js", velox.JS)
	mux.Handle("/", gziphandler.GzipHandler(static.Handler()))

	log.Printf("Listening at http://%s", addr)
	return http.ListenAndServe(addr, mux)
}

// handleSync upgrades a velox connection on the embedded state and tracks the
// connected user for the lifetime of the connection.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	conn, err := s.state.Handle(w, r)
	if err != nil {
		log.Printf("sync failed: %s", err)
		return
	}
	id := conn.ID()
	s.state.Lock()
	s.state.Users[id] = r.RemoteAddr
	s.state.Unlock()
	s.state.Push()

	conn.Wait()

	s.state.Lock()
	delete(s.state.Users, id)
	s.state.Unlock()
	s.state.Push()
}

// reconfigure applies a new engine config, persists it, and updates the synced
// state.
func (s *Server) reconfigure(c engine.Config) error {
	dldir, err := filepath.Abs(c.DownloadDirectory)
	if err != nil {
		return fmt.Errorf("invalid download directory")
	}
	c.DownloadDirectory = dldir
	if c.IncomingPort <= 0 || c.IncomingPort >= 65535 {
		c.IncomingPort = 50007
	}
	if err := s.engine.Configure(c); err != nil {
		return err
	}
	if b, err := json.MarshalIndent(&c, "", "  "); err == nil {
		os.WriteFile(s.ConfigPath, b, 0644)
	}
	s.state.Lock()
	s.state.Config = c
	s.state.Unlock()
	s.state.Push()
	return nil
}
