package server

import (
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/boypt/scraper/scraper"
	"github.com/jpillora/cloud-torrent/engine"
	ctstatic "github.com/jpillora/cloud-torrent/static"
	"github.com/jpillora/cookieauth"
	"github.com/jpillora/requestlog"
	"github.com/jpillora/velox"
	"github.com/radovskyb/watcher"
	"github.com/skratchdot/open-golang/open"
)

const (
	cacheSavedPrefix = "_CLDAUTOSAVED_"
	scraperUA        = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/57.0.2987.133 Safari/537.36"
)

//Server is the "State" portion of the diagram
type Server struct {
	//config
	Title          string `help:"Title of this instance" env:"TITLE"`
	Port           int    `help:"Listening port" env:"PORT"`
	Host           string `help:"Listening interface (default all)"`
	Auth           string `help:"Optional basic auth in form 'user:password'" env:"AUTH"`
	ConfigPath     string `help:"Configuration file path"`
	KeyPath        string `help:"TLS Key file path"`
	CertPath       string `help:"TLS Certicate file path" short:"r"`
	Log            bool   `help:"Enable request logging"`
	Open           bool   `help:"Open now with your default browser"`
	DisableLogTime bool   `help:"Don't print timestamp in log"`
	DebugTorrent   bool   `help:"Debug torrent engine"`
	//http handlers
	files, static http.Handler
	scraper       *scraper.Handler
	scraperh      http.Handler

	//file watcher
	watcher *watcher.Watcher

	//torrent engine
	engine *engine.Engine
	state  struct {
		velox.State
		sync.Mutex
		Config          engine.Config
		SearchProviders scraper.Config
		Downloads       *fsNode
		Torrents        map[string]*engine.Torrent
		Users           map[string]string
		Stats           struct {
			Title   string
			Version string
			Runtime string
			Uptime  time.Time
			System  stats
		}
	}
}

// Run the server
func (s *Server) Run(version string) error {
	isTLS := s.CertPath != "" || s.KeyPath != "" //poor man's XOR
	if isTLS && (s.CertPath == "" || s.KeyPath == "") {
		return fmt.Errorf("You must provide both key and cert paths")
	}
	s.state.Stats.Title = s.Title
	s.state.Stats.Version = version
	s.state.Stats.Runtime = strings.TrimPrefix(runtime.Version(), "go")
	s.state.Stats.Uptime = time.Now()
	s.state.Stats.System.pusher = velox.Pusher(&s.state)
	//init maps
	s.state.Users = map[string]string{}
	//will use a the local embed/ dir if it exists, otherwise will use the hardcoded embedded binaries
	s.files = http.HandlerFunc(s.serveFiles)
	s.static = ctstatic.FileSystemHandler()
	s.scraper = &scraper.Handler{
		Log: false, Debug: false,
		Headers: map[string]string{
			//we're a trusty browser :)
			"User-Agent": scraperUA,
		},
	}
	if err := s.scraper.LoadConfig(defaultSearchConfig); err != nil {
		log.Fatal(err)
	}
	//scraper
	s.state.SearchProviders = s.scraper.Config //share scraper config
	go s.fetchSearchConfigLoop()

	s.scraperh = http.StripPrefix("/search", s.scraper)
	//torrent engine
	s.engine = engine.New()

	//configure engine
	c := engine.Config{
		DownloadDirectory:    "./downloads",
		WatchDirectory:       "./torrents",
		EnableUpload:         true,
		AutoStart:            true,
		DoneCmd:              "",
		SeedRatio:            0,
		ObfsPreferred:        true,
		ObfsRequirePreferred: false,
	}
	if _, err := os.Stat(s.ConfigPath); err == nil {
		if b, err := ioutil.ReadFile(s.ConfigPath); err != nil {
			return fmt.Errorf("Read configuration error: %s", err)
		} else if len(b) == 0 {
			//ignore empty file
		} else if err := json.Unmarshal(b, &c); err != nil {
			return fmt.Errorf("Malformed configuration: %s", err)
		}
	}
	if c.IncomingPort <= 0 || c.IncomingPort >= 65535 {
		c.IncomingPort = 50007
	}
	if err := s.reconfigure(c); err != nil {
		return fmt.Errorf("initial configure failed: %s", err)
	}
	log.Printf("Read Config: %#v\n", c)
	//poll torrents and files
	go func() {
		for {
			s.state.Lock()
			s.state.Torrents = s.engine.GetTorrents()
			s.state.Downloads = s.listFiles()
			s.state.Unlock()
			s.state.Push()
			time.Sleep(3 * time.Second)
		}
	}()
	//start collecting stats
	go func() {
		for {
			c := s.engine.Config()
			s.state.Stats.System.loadStats(c.DownloadDirectory)
			time.Sleep(5 * time.Second)
		}
	}()

	go s.engine.UpdateTrackers()

	// if torrent file exists in WatchDirectory, add them as task
	tors, _ := filepath.Glob(filepath.Join(c.WatchDirectory, "*.torrent"))
	for _, t := range tors {
		if err := s.engine.NewFileTorrent(t); err == nil {
			if strings.HasPrefix(filepath.Base(t), cacheSavedPrefix) {
				log.Printf("Inital Task Restored: %s \n", t)
			} else {
				log.Printf("Inital Task: added %s, file removed\n", t)
				os.Remove(t)
			}
		} else {
			log.Printf("Inital Task: fail to add %s, ERR:%#v\n", t, err)
		}
	}

	host := s.Host
	if host == "" {
		host = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", host, s.Port)
	proto := "http"
	if isTLS {
		proto += "s"
	}
	if s.Open {
		openhost := host
		if openhost == "0.0.0.0" {
			openhost = "localhost"
		}
		go func() {
			time.Sleep(1 * time.Second)
			open.Run(fmt.Sprintf("%s://%s:%d", proto, openhost, s.Port))
		}()
	}
	//define handler chain, from last to first
	h := http.Handler(http.HandlerFunc(s.handle))
	//gzip
	gzipWrap, _ := gziphandler.NewGzipLevelAndMinSize(gzip.DefaultCompression, 0)
	h = gzipWrap(h)
	//auth
	if s.Auth != "" {
		user := s.Auth
		pass := ""
		if s := strings.SplitN(s.Auth, ":", 2); len(s) == 2 {
			user = s[0]
			pass = s[1]
		}
		h = cookieauth.New().SetUserPass(user, pass).Wrap(h)
		log.Printf("Enabled HTTP authentication")
	}
	h = livenessWrap(h)
	if s.Log {
		h = requestlog.Wrap(h)
	}
	log.Printf("Listening at %s://%s", proto, addr)
	//serve!
	server := http.Server{
		//disable http2 due to velox bug
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		//address
		Addr: addr,
		//handler stack
		Handler: h,
	}
	if isTLS {
		return server.ListenAndServeTLS(s.CertPath, s.KeyPath)
	}
	return server.ListenAndServe()
}

func (s *Server) reconfigure(c engine.Config) error {
	dldir, err := filepath.Abs(c.DownloadDirectory)
	if err != nil {
		return fmt.Errorf("Invalid path")
	}
	c.DownloadDirectory = dldir

	wdir, err := filepath.Abs(c.WatchDirectory)
	if err != nil {
		return fmt.Errorf("Invalid path")
	}
	c.WatchDirectory = wdir

	b, _ := json.MarshalIndent(&c, "", "  ")
	if err := ioutil.WriteFile(s.ConfigPath, b, 0644); err != nil {
		return err
	}

	// torrent watcher
	if s.watcher != nil {
		log.Print("Torrent Watcher: close")
		s.watcher.Close()
		s.watcher = nil
	}

	if w, err := os.Stat(c.WatchDirectory); err == nil {
		if w.IsDir() {
			s.TorrentWatcher(c)
		}
	}

	if err := s.engine.Configure(c); err != nil {
		return err
	}
	s.state.Config = c
	s.state.Push()
	return nil
}

func (s *Server) TorrentWatcher(c engine.Config) error {

	log.Printf("Torrent Watcher: watching torrent file in %s", c.WatchDirectory)
	w := watcher.New()
	w.SetMaxEvents(10)
	w.FilterOps(watcher.Create)

	go func() {
		for {
			select {
			case event := <-w.Event:
				if event.IsDir() {
					continue
				}
				// skip auto saved torrent
				if strings.HasPrefix(event.Name(), cacheSavedPrefix) {
					continue
				}
				if strings.HasSuffix(event.Name(), ".torrent") {
					if err := s.engine.NewFileTorrent(event.Path); err == nil {
						log.Printf("Torrent Watcher: added %s, file removed\n", event.Name())
						os.Remove(event.Path)
					} else {
						log.Printf("Torrent Watcher: fail to add %s, ERR:%#v\n", event.Name(), err)
					}
				}
			case err := <-w.Error:
				log.Print(err)
			case <-w.Closed:
				return
			}
		}
	}()

	// Watch this folder for changes.
	if err := w.Add(c.WatchDirectory); err != nil {
		return err
	}

	s.watcher = w
	go w.Start(time.Second * 5)
	return nil
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	//handle realtime client library
	if strings.Compare(r.URL.Path, "/js/velox.js") == 0 {
		velox.JS.ServeHTTP(w, r)
		return
	}
	//handle realtime client connections
	if strings.Compare(r.URL.Path, "/sync") == 0 {
		conn, err := velox.Sync(&s.state, w, r)
		if err != nil {
			log.Printf("sync failed: %s", err)
			return
		}
		s.state.Users[conn.ID()] = r.RemoteAddr
		s.state.Push()
		conn.Wait()
		delete(s.state.Users, conn.ID())
		s.state.Push()
		return
	}
	//search
	if strings.HasPrefix(r.URL.Path, "/search") {
		s.scraperh.ServeHTTP(w, r)
		return
	}
	//api call
	if strings.HasPrefix(r.URL.Path, "/api/") {
		//only pass request in, expect error out
		if err := s.api(r); err == nil {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			switch err {
			case errRedirect:
				http.Redirect(w, r, "/", http.StatusFound)
			default:
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(err.Error()))
			}
		}
		return
	}
	//no match, assume static file
	s.files.ServeHTTP(w, r)
}

func livenessWrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// liveness response
		if strings.Compare(r.URL.Path, "/healthz") == 0 {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}
		h.ServeHTTP(w, r)
	})
}
