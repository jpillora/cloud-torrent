package ct

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jpillora/cloud-torrent/ct/embed"
	"github.com/jpillora/go-realtime"
)

//Server is the "State" portion of the diagram
type Server struct {
	//config
	Port int    `help:"Listening port"`
	Host string `help:"Listening interface (default all)"`
	Auth string `help:"Optional basic auth in form user:password"`
	//state
	fs      http.Handler
	engines map[string]Engine
	rt      *realtime.Realtime
	state   struct {
		A, B int
	}
}

func (s *Server) init() error {
	//will use a the local embed/ dir if it exists, otherwise will use the hardcoded embedded binaries
	s.fs = embed.FileSystemHandler()
	//prepare all torrent engines
	s.engines = map[string]Engine{}
	for _, e := range bundledEngines {
		if err := s.AddEngine(e); err != nil {
			return err
		}
	}
	//realtime
	s.rt = realtime.Sync(&s.state)
	//ready
	return nil
}

func (s *Server) AddEngine(e Engine) error {
	name := strings.ToLower(e.Name())
	if _, ok := s.engines[name]; ok {
		return fmt.Errorf("Engine %s already exists", name)
	}
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
	//handle realtime javascript
	if strings.HasSuffix(r.URL.Path, "realtime.js") {
		realtime.JS.ServeHTTP(w, r)
		return
	}
	//no match, assume static file
	s.fs.ServeHTTP(w, r)
}
