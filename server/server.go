package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jpillora/cloud-torrent/engine"
	"github.com/jpillora/cloud-torrent/static"
	"github.com/jpillora/cloud-torrent/storage"
	"github.com/jpillora/go-realtime"
	"github.com/jpillora/requestlog"
	"github.com/jpillora/scraper/scraper"
	"github.com/jpillora/velox"
	"github.com/skratchdot/open-golang/open"
)

//Server is the "State" portion of the diagram
type Server struct {
	//config
	Title      string `help:"Title of this instance" env:"TITLE"`
	Port       int    `help:"Listening port" env:"PORT"`
	Host       string `help:"Listening interface (default all)"`
	Auth       string `help:"Optional basic auth in form 'user:password'" env:"AUTH"`
	ConfigPath string `help:"Configuration file path"`
	KeyPath    string `help:"TLS Key file path"`
	CertPath   string `help:"TLS Certicate file path" short:"r"`
	Log        bool   `help:"Enable request logging"`
	Open       bool   `help:"Open now with your default browser"`
	//http handlers
	files, static http.Handler
	scraper       *scraper.Handler
	scraperh      http.Handler
	//torrent storage
	storage *storage.Storage
	//torrent engine
	engine *engine.Engine
	//velox state (sync'd with browser immediately)
	state struct {
		velox.State
		sync.Mutex
		Config          Config
		SearchProviders scraper.Config
		Downloads       map[string]*storage.Node
		Torrents        map[string]*engine.Torrent
		Users           map[string]*realtime.User
		Stats           struct {
			Title   string
			Version string
			Runtime string
			Uptime  time.Time
		}
	}
}

func (s *Server) Run(version string) error {
	log.Printf("run...")
	tls := s.CertPath != "" || s.KeyPath != "" //poor man's XOR
	if tls && (s.CertPath == "" || s.KeyPath == "") {
		return fmt.Errorf("You must provide both key and cert paths")
	}
	//torrent storage
	s.storage = storage.New()
	//torrent engine
	s.engine = engine.New(s.storage)
	//stats
	s.state.Stats.Title = s.Title
	s.state.Stats.Version = version
	s.state.Stats.Runtime = strings.TrimPrefix(runtime.Version(), "go")
	s.state.Stats.Uptime = time.Now()
	//init maps
	s.state.Users = map[string]*realtime.User{}
	s.state.Downloads = map[string]*storage.Node{}
	s.state.Torrents = s.engine.Torrents
	//will use a the local embed/ dir if it exists, otherwise will use the hardcoded embedded binaries
	s.files = http.HandlerFunc(s.serveFiles)
	s.static = ctstatic.FileSystemHandler()
	s.scraper = &scraper.Handler{Log: false}
	if err := s.scraper.LoadConfig(defaultSearchConfig); err != nil {
		log.Fatal(err)
	}
	s.state.SearchProviders = s.scraper.Config //share scraper config
	s.scraperh = http.StripPrefix("/search", s.scraper)
	//configure engine
	c := Config{
		Torrent: engine.Config{
			DownloadDirectory: "./downloads",
			EnableUpload:      true,
			EnableEncryption:  true,
			AutoStart:         true,
		},
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
	log.Printf("reconf...")
	//initial configure
	if err := s.reconfigure(c); err != nil {
		return fmt.Errorf("initial configure failed: %s", err)
	}

	log.Printf("poll...")
	//poll torrents
	go func() {
		for {
			s.state.Lock()
			s.engine.Update()
			// log.Printf("torrents #%d files #%d", len(s.state.Torrents), len(s.state.Downloads.Children))
			s.state.Unlock()
			s.state.Push()
			time.Sleep(1 * time.Second)
		}
	}()
	//poll downloads
	// go func() {
	// 	for {
	// 		for name, fs := range s.storage.FileSystems {
	// 			if root, err := fs.List(""); err != nil {
	// 				s.state.Downloads[name] = root
	// 			}
	// 		}
	// 		time.Sleep(1 * time.Second)
	// 	}
	// }()

	host := s.Host
	if host == "" {
		host = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", host, s.Port)
	proto := "http"
	if tls {
		proto += "s"
	}
	log.Printf("Listening at %s://%s", proto, addr)

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

	h := http.Handler(http.HandlerFunc(s.handle))
	if s.Log {
		h = requestlog.Wrap(h)
	}

	if tls {
		return http.ListenAndServeTLS(addr, s.CertPath, s.KeyPath, h)
	} else {
		return http.ListenAndServe(addr, h)
	}
}

func (s *Server) reconfigure(c Config) error {
	if err := s.engine.Configure(&c.Torrent); err != nil {
		return err
	}
	if err := s.storage.Configure(c.Storage); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(&c, "", "  ")
	ioutil.WriteFile(s.ConfigPath, b, 0755)
	s.state.Config = c
	s.state.Push()
	log.Printf("reconfd")
	return nil
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	//handle realtime client connections
	if r.URL.Path == "/js/velox.js" {
		velox.JS.ServeHTTP(w, r)
		return
	} else if r.URL.Path == "/sync" {
		if conn, err := velox.Sync(&s.state, w, r); err == nil {
			conn.Wait()
		}
		return
	}
	//basic auth
	if s.Auth != "" {
		u, p, _ := r.BasicAuth()
		if s.Auth != u+":"+p {
			w.Header().Set("WWW-Authenticate", "Basic")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Access Denied"))
			return
		}
	}
	//search
	if strings.HasPrefix(r.URL.Path, "/search") {
		s.scraperh.ServeHTTP(w, r)
		return
	}
	//api call
	if strings.HasPrefix(r.URL.Path, "/api/") {
		//only pass request in, expect error out
		if err := s.handleAPI(w, r); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
		}
		return
	}
	//no match, assume static file
	s.files.ServeHTTP(w, r)
}
