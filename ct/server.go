package ct

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jpillora/cloud-torrent/ct/engines"
	"github.com/jpillora/cloud-torrent/ct/shared"
	"github.com/jpillora/cloud-torrent/static"
	"github.com/jpillora/go-realtime"
	"github.com/jpillora/scraper/lib"
)

//Server is the "State" portion of the diagram
type Server struct {
	//config
	Port       int    `help:"Listening port"`
	Host       string `help:"Listening interface (default all)"`
	Auth       string `help:"Optional basic auth (in form user:password)"`
	ConfigPath string `help:"Configuration file path"`
	//http handlers
	fs       http.Handler
	scraper  *scraper.Handler
	scraperh http.Handler
	//enabled torrent engines
	engines map[engine.ID]engine.Engine
	//realtime state (sync'd with browser immediately)
	rt    *realtime.Realtime
	state struct {
		Engines         map[engine.ID]engineState
		SearchProviders scraper.Config
		Users           map[string]realtime.User
	}
}

//engine state shared with clients
type engineState struct {
	Name     string
	Config   interface{}
	Torrents torrents
}

func (s *Server) init() error {
	//init maps
	s.state.Engines = map[engine.ID]engineState{}
	s.state.Users = map[string]realtime.User{}
	//will use a the local embed/ dir if it exists, otherwise will use the hardcoded embedded binaries
	s.fs = ctstatic.FileSystemHandler()
	s.scraper = &scraper.Handler{Log: true}
	if err := s.scraper.LoadConfig(defaultSearchConfig); err != nil {
		log.Fatal(err)
	}
	s.state.SearchProviders = s.scraper.Config //share scraper config
	s.scraperh = http.StripPrefix("/search", s.scraper)

	//prepare all torrent engines
	s.engines = map[engine.ID]engine.Engine{}
	for _, e := range engine.Bundled {
		if err := s.AddEngine(e); err != nil {
			return err
		}
	}
	//realtime
	rt, err := realtime.Sync(&s.state)
	if err != nil {
		log.Fatalf("State object not syncable: %s", err)
	}
	s.rt = rt

	//check users every second
	go s.userWatch()

	//initial config provided
	var cfg []byte = nil
	if s.ConfigPath != "" {
		var err error
		if cfg, err = ioutil.ReadFile(s.ConfigPath); err != nil {
			return err
		}
	} else {
		cfg = s.defaultConfig()
	}

	//load default or provided
	if err := s.loadConfig(cfg); err != nil {
		return err
	}

	//ready
	return nil
}

func (s *Server) Run() error {
	if err := s.init(); err != nil {
		return err
	}
	// TODO if Open {
	// cross platform open - https://github.com/skratchdot/open-golang
	// }
	log.Printf("Listening on %d...", s.Port)
	http.ListenAndServe(s.Host+":"+strconv.Itoa(s.Port), http.HandlerFunc(s.handle))
	return nil
}

func (s *Server) AddEngine(e engine.Engine) error {
	name := e.Name()
	id := engine.ID(strings.ToLower(name))
	if _, ok := s.engines[id]; ok {
		return fmt.Errorf("engine.Engine %s already exists", id)
	}
	s.engines[id] = e
	torrents := torrents{}
	s.state.Engines[id] = engineState{
		Name:     name,
		Config:   e.NewConfig(),
		Torrents: torrents,
	}
	go s.torrentsWatch(torrents, e.GetTorrents())
	return nil
}

func (s *Server) torrentsWatch(torrents torrents, queue <-chan *shared.Torrent) {
	for t := range queue {
		torrents[t.InfoHash] = t
		s.rt.Update()
	}
}

//runs in a go routine
func (s *Server) userWatch() {
	//TODO poll engines when users connected
	for user := range s.rt.Changes() {
		log.Printf("user %+v", user)
		if user.Connected {
			s.state.Users[user.Address] = user
		} else {
			delete(s.state.Users, user.Address)
		}
		s.rt.Update()
	}
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
	s.fs.ServeHTTP(w, r)
}

//generates the default configuration for all engines
func (s *Server) defaultConfig() []byte {
	configs := map[engine.ID]interface{}{}
	for id, e := range s.state.Engines {
		configs[id] = e.Config
	}
	b, _ := json.Marshal(configs)
	return b
}

//load a json configuration
func (s *Server) loadConfig(b []byte) error {

	//batch alter configuration
	configs := map[engine.ID]json.RawMessage{}
	if err := json.Unmarshal(b, &configs); err != nil {
		return err
	}

	for id, msg := range configs {
		e, ok := s.engines[id]
		if !ok {
			return fmt.Errorf("engine: %s: missing", id)
		}

		c := s.state.Engines[id].Config
		if err := json.Unmarshal(msg, &c); err != nil {
			return fmt.Errorf("engine: %s: replace config failed: %s", id, err)
		}

		if err := e.SetConfig(c); err != nil {
			return fmt.Errorf("engine: %s: apply config failed: %s", id, err)
		}
	}
	s.rt.Update()
	return nil
}
