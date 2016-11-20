package server

import (
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

	"github.com/jpillora/cloud-torrent/engine"
	"github.com/jpillora/cloud-torrent/static"
	"github.com/jpillora/go-realtime"
	"github.com/jpillora/requestlog"
	"github.com/jpillora/scraper/scraper"
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
	//torrent engine
	engine *engine.Engine
	//realtime state (sync'd with browser immediately)
	rt    *realtime.Handler
	state struct {
		realtime.Object
		sync.Mutex
		Config          engine.Config
		SearchProviders scraper.Config
		Downloads       *fsNode
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

	tls := s.CertPath != "" || s.KeyPath != "" //poor man's XOR
	if tls && (s.CertPath == "" || s.KeyPath == "") {
		return fmt.Errorf("You must provide both key and cert paths")
	}

	s.state.Stats.Title = s.Title
	s.state.Stats.Version = version
	s.state.Stats.Runtime = strings.TrimPrefix(runtime.Version(), "go")
	s.state.Stats.Uptime = time.Now()

	//init maps
	s.state.Users = map[string]*realtime.User{}
	//will use a the local embed/ dir if it exists, otherwise will use the hardcoded embedded binaries
	s.files = http.HandlerFunc(s.serveFiles)
	s.static = ctstatic.FileSystemHandler()
	s.scraper = &scraper.Handler{
		Log: false, Debug: false,
		Headers: map[string]string{
			"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/54.0.2840.71 Safari/537.36",
		},
	}
	if err := s.scraper.LoadConfig(defaultSearchConfig); err != nil {
		log.Fatal(err)
	}

	s.state.SearchProviders = s.scraper.Config //share scraper config
	go s.fetchSearchConfigLoop()
	s.scraperh = http.StripPrefix("/search", s.scraper)

	s.engine = engine.New()

	//realtime
	s.rt = realtime.NewHandler()
	if err := s.rt.Add("state", &s.state); err != nil {
		log.Fatalf("State object not syncable: %s", err)
	}
	//realtime user events
	go func() {
		for user := range s.rt.UserEvents() {
			if user.Connected {
				s.state.Users[user.ID] = user
			} else {
				delete(s.state.Users, user.ID)
			}
			s.state.Update()
		}
	}()

	//configure engine
	c := engine.Config{
		DownloadDirectory: "./downloads",
		EnableUpload:      true,
		AutoStart:         true,
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

	//poll torrents and files
	go func() {
		for {
			s.state.Lock()
			s.state.Torrents = s.engine.GetTorrents()
			s.state.Downloads = s.listFiles()
			// log.Printf("torrents #%d files #%d", len(s.state.Torrents), len(s.state.Downloads.Children))
			s.state.Unlock()
			s.state.Update()
			time.Sleep(1 * time.Second)
		}
	}()

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

func (s *Server) reconfigure(c engine.Config) error {
	dldir, err := filepath.Abs(c.DownloadDirectory)
	if err != nil {
		return fmt.Errorf("Invalid path")
	}
	c.DownloadDirectory = dldir
	if err := s.engine.Configure(c); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(&c, "", "  ")
	ioutil.WriteFile(s.ConfigPath, b, 0755)
	s.state.Config = c
	s.state.Update()
	return nil
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {

	//handle realtime client connections
	if r.URL.Path == "/realtime.js" {
		realtime.JS.ServeHTTP(w, r)
		return
	} else if r.URL.Path == "/realtime" {
		s.rt.ServeHTTP(w, r)
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
		if err := s.api(r); err == nil {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
		}
		return
	}
	//no match, assume static file
	s.files.ServeHTTP(w, r)
}
