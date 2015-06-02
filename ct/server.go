package ct

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jpillora/cloud-torrent/ct/embed"
	"github.com/jpillora/go-realtime"
	"github.com/jpillora/scraper/lib"
)

//Server is the "State" portion of the diagram
type Server struct {
	//config
	Port   int    `help:"Listening port"`
	Host   string `help:"Listening interface (default all)"`
	Auth   string `help:"Optional basic auth (in form user:password)"`
	Config string `help:"Configuration file path"`
	//state
	fs       http.Handler
	scraper  *scraper.Handler
	scraperh http.Handler
	engines  map[engineID]Engine
	rt       *realtime.Realtime
	state    struct {
		Configs  map[engineID]interface{}
		Torrents map[engineID]torrents
	}
}

func (s *Server) init() error {
	//init maps
	s.state.Configs = map[engineID]interface{}{}
	//will use a the local embed/ dir if it exists, otherwise will use the hardcoded embedded binaries
	s.fs = embed.FileSystemHandler()
	s.scraper = &scraper.Handler{}
	s.scraperh = http.StripPrefix("/search", s.scraper)
	//prepare all torrent engines
	s.engines = map[engineID]Engine{}
	for _, e := range bundledEngines {
		if err := s.AddEngine(e); err != nil {
			return err
		}
	}
	//realtime
	if rt, err := realtime.Sync(&s.state); err != nil {
		log.Fatalf("State not syncable: %s", err)
	} else {
		s.rt = rt
	}
	//initial config provided
	var cfg []byte = nil
	if s.Config != "" {
		var err error
		if cfg, err = ioutil.ReadFile(s.Config); err != nil {
			return err
		}
	}
	//load default or provided
	if err := s.loadConfig(cfg); err != nil {
		return err
	}
	//ready
	return nil
}

func (s *Server) AddEngine(e Engine) error {
	name := e.Name()
	id := engineID(strings.ToLower(name))
	if _, ok := s.engines[id]; ok {
		return fmt.Errorf("Engine %s already exists", id)
	}
	s.engines[id] = e
	s.state.Configs[id] = e.GetConfig()
	return nil
}

func (s *Server) Run() error {
	if err := s.init(); err != nil {
		return err
	}
	log.Printf("Listening on %d...", s.Port)
	http.ListenAndServe(s.Host+":"+strconv.Itoa(s.Port), http.HandlerFunc(s.handle))
	return nil
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	//basic auth
	if s.Auth != "" {
		u, p, _ := r.BasicAuth()
		if s.Auth != u+":"+p {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Access Denied"))
			return
		}
	}
	//handle realtime client connections
	if p := r.Header.Get("Sec-Websocket-Protocol"); p != "" {
		s.rt.ServeHTTP(w, r)
		return
	}
	//embedded realtime js lib
	if strings.HasSuffix(r.URL.Path, "realtime.js") {
		realtime.JS.ServeHTTP(w, r)
		return
	}
	//search
	if strings.HasPrefix(r.URL.Path, "/search/") {
		s.scraperh.ServeHTTP(w, r)
		return
	}
	//api call
	if strings.HasPrefix(r.URL.Path, "/api/") {
		//only pass request in, expect error out
		if err := s.api(r); err == nil {
			w.WriteHeader(200)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(400)
			w.Write([]byte(err.Error()))
		}
		return
	}
	//no match, assume static file
	s.fs.ServeHTTP(w, r)
}

func (s *Server) loadConfig(b []byte) error {
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.state.Configs); err != nil {
			return err
		}
	}
	for _, e := range s.engines {
		if err := e.SetConfig(); err != nil {
			return err
		}
	}
	s.rt.Update()
	return nil
}
