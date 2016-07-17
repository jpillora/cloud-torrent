package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/jpillora/cloud-torrent/fs"
	"github.com/jpillora/cloud-torrent/fs/disk"
	"github.com/jpillora/cloud-torrent/fs/dropbox"
	"github.com/jpillora/cloud-torrent/fs/torrent"
	"github.com/jpillora/cloud-torrent/static"
	realtime "github.com/jpillora/go-realtime"
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
	//filesystems
	fileSystems map[string]fs.FS
	//velox state (sync'd with browser immediately)
	state struct {
		velox.State
		sync.Mutex
		// Config          Config
		SearchProviders scraper.Config
		FileSystems     map[string]fs.FS
		Configurations  map[string]interface{}
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
	//fs
	s.fileSystems = map[string]fs.FS{}
	for _, fs := range []fs.FS{
		torrent.New(),
		disk.New(),
		dropbox.New(),
	} {
		n := fs.Name()
		if _, ok := s.fileSystems[n]; ok {
			return errors.New("duplicate fs: " + n)
		}
		s.fileSystems[n] = fs
	}
	//stats
	s.state.Stats.Title = s.Title
	s.state.Stats.Version = version
	s.state.Stats.Runtime = strings.TrimPrefix(runtime.Version(), "go")
	s.state.Stats.Uptime = time.Now()
	s.state.FileSystems = s.fileSystems
	s.state.Configurations = map[string]interface{}{}
	s.state.Users = map[string]*realtime.User{}
	//will use a the local embed/ dir if it exists, otherwise will use the hardcoded embedded binaries
	// s.files = http.HandlerFunc(s.serveFiles)
	s.static = ctstatic.FileSystemHandler()
	s.scraper = &scraper.Handler{Log: false}
	if err := s.scraper.LoadConfig(ctstatic.MustAsset("files/misc/search.json")); err != nil {
		log.Fatal(err)
	}
	s.state.SearchProviders = s.scraper.Config //share scraper config
	s.scraperh = http.StripPrefix("/search", s.scraper)
	//configure
	if _, err := os.Stat(s.ConfigPath); err == nil {
		if b, err := ioutil.ReadFile(s.ConfigPath); err != nil {
			return fmt.Errorf("Read configurations error: %s", err)
		} else if len(b) == 0 {
			//ignore empty file
		} else if err := s.reconfigure(b); err != nil {
			return fmt.Errorf("initial configure failed: %s", err)
		}
	}
	//initial configure
	// if err := s.reconfigure(cfgs); err != nil {
	// 	return fmt.Errorf("initial configure failed: %s", err)
	// }

	log.Printf("poll...")
	//poll torrents
	go func() {
		for {
			s.state.Lock()
			// s.engine.Update()
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

func (s *Server) reconfigure(b []byte) error {
	cfgs := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &cfgs); err != nil {
		return nil
	}
	for name, raw := range cfgs {
		// if name == "Server" {
		//
		// } else if fs, ok := s.FileSystems[name]; ok {
		//
		// }
		v, err := s.configure(raw)
		if err != nil {
			continue
		}
		s.state.Configurations[name] = v
	}
	//write back to disk if changed
	b2, _ := json.MarshalIndent(&cfgs, "", "  ")
	if !bytes.Equal(b, b2) {
		ioutil.WriteFile(s.ConfigPath, b2, 0600)
		//update frontend
		s.state.Push()
	}
	log.Printf("reconfd")
	return nil
}

func (s *Server) configure(raw json.RawMessage) (interface{}, error) {
	return nil, nil
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
		// if err := s.handleAPI(w, r); err != nil {
		// 	w.WriteHeader(http.StatusBadRequest)
		// 	w.Write([]byte(err.Error()))
		// }
		return
	}
	//no match, assume static file
	s.files.ServeHTTP(w, r)
}
