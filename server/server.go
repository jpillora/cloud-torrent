package server

import (
	"compress/gzip"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/jpillora/cloud-torrent/engine"
	"github.com/jpillora/cloud-torrent/static"
	"github.com/jpillora/cookieauth"
	"github.com/jpillora/requestlog"
	"github.com/jpillora/scraper/scraper"
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

	engine       *engine.Engine
	state        State
	scraper      *scraper.Handler
	searchConfig []byte     // last-loaded (normalized) provider config, for change detection
	configMu     sync.Mutex // serializes engine changes and their persisted configuration
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

	c, err := readConfig(s.ConfigPath, engine.Config{
		DownloadDirectory: "./downloads",
		EnableUpload:      true,
		AutoStart:         true,
	})
	if err != nil {
		return err
	}

	// search providers: load the embedded config, publish into the state, and
	// start the background refresh of the remote config.
	if err := s.startSearch(); err != nil {
		return fmt.Errorf("search init failed: %w", err)
	}

	// torrent engine + persisted config
	s.engine = engine.New()
	if err := s.reconfigure(c); err != nil {
		return fmt.Errorf("initial configure failed: %w", err)
	}

	// poll torrents + downloads every second
	go func() {
		for {
			// compute outside the state lock so a slow GetTorrents/listFiles (e.g.
			// under active download) doesn't block action-triggered pushes
			torrents := s.engine.GetTorrents()
			downloads := listFiles(s.engine.Config().DownloadDirectory)
			s.state.Lock()
			s.state.Torrents = torrents
			s.state.Downloads = downloads
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

	// TLS is enabled when a cert/key pair is given; both are required.
	isTLS := s.CertPath != "" || s.KeyPath != ""
	if isTLS && (s.CertPath == "" || s.KeyPath == "") {
		return fmt.Errorf("both --cert-path and --key-path are required for TLS")
	}
	proto := "http"
	if isTLS {
		proto = "https"
	}
	srv := &http.Server{
		Addr:    addr,
		Handler: s.handler(),
		// disable HTTP/2: velox's streaming sync is incompatible with it.
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
	}
	log.Printf("Listening at %s://%s", proto, addr)
	if isTLS {
		return srv.ListenAndServeTLS(s.CertPath, s.KeyPath)
	}
	return srv.ListenAndServe()
}

// handler builds the HTTP handler chain: the route mux (realtime sync, action
// API, search, file downloads, velox client, embedded SPA) wrapped — innermost
// to outermost — by gzip, optional basic auth, and optional request logging,
// composed exactly as the original server.
func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/sync", s.handleSync)
	mux.HandleFunc("/api/", s.handleAPI)
	mux.Handle("/search/", http.StripPrefix("/search", http.HandlerFunc(s.handleSearch)))
	mux.Handle("/download/", http.StripPrefix("/download/", downloadHandler(func() string {
		return s.engine.Config().DownloadDirectory
	}, s.refreshDownloads)))
	mux.Handle("/js/velox.js", velox.JS)
	mux.Handle("/", static.Handler())

	// gzip everything. minSize 0 (gzip even small/streamed responses) keeps the
	// velox sync stream flushing in realtime instead of buffering to a threshold.
	h := http.Handler(mux)
	gzipWrap, _ := gziphandler.NewGzipLevelAndMinSize(gzip.DefaultCompression, 0)
	h = gzipWrap(h)

	// basic auth (cookie-backed) when --auth user:pass is provided
	if s.Auth != "" {
		user, pass := s.Auth, ""
		if parts := strings.SplitN(s.Auth, ":", 2); len(parts) == 2 {
			user, pass = parts[0], parts[1]
		}
		h = cookieauth.New().SetUserPass(user, pass).Wrap(h)
		log.Printf("Enabled HTTP authentication")
	}

	// request logging, outermost so it records every request
	if s.Log {
		h = requestlog.Wrap(h)
	}
	return h
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
	s.configMu.Lock()
	defer s.configMu.Unlock()
	dldir, err := filepath.Abs(c.DownloadDirectory)
	if err != nil {
		return fmt.Errorf("invalid download directory")
	}
	c.DownloadDirectory = dldir
	if c.IncomingPort <= 0 || c.IncomingPort >= 65535 {
		c.IncomingPort = 50007
	}
	pending, err := stageConfig(s.ConfigPath, c)
	if err != nil {
		return err
	}
	defer pending.discard()
	previous := s.engine.Config()
	if err := s.engine.Configure(c); err != nil {
		return err
	}
	if err := pending.commit(s.ConfigPath); err != nil {
		var rollback error
		if previous.DownloadDirectory == "" {
			rollback = s.engine.Close()
		} else {
			rollback = s.engine.Configure(previous)
			s.publishConfiguration()
		}
		if rollback != nil {
			return errors.Join(err, fmt.Errorf("restore previous configuration: %w", rollback))
		}
		return err
	}
	s.publishConfiguration()
	return nil
}

func (s *Server) publishConfiguration() {
	torrents := s.engine.GetTorrents()
	c := s.engine.Config()
	s.state.Lock()
	s.state.Config = c
	s.state.Torrents = torrents
	s.state.Unlock()
	s.state.Push()
}

// refreshDownloads re-lists the download directory into the synced state and
// pushes immediately, so a file deletion is reflected without waiting for the
// one-second poll.
func (s *Server) refreshDownloads() {
	downloads := listFiles(s.engine.Config().DownloadDirectory)
	s.state.Lock()
	s.state.Downloads = downloads
	s.state.Unlock()
	s.state.Push()
}
