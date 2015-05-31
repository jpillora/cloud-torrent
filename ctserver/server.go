package ctserver

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jpillora/cloud-torrent/ctserver/embed"
	"github.com/jpillora/go-realtime"
)

//Server is the "State" portion of the diagram
type Server struct {
	//config
	Port     int    `help:"Listening port"`
	Host     string `help:"Listening interface (default all)"`
	AuthUser string `help:"Optional HTTP Auth"`
	AuthPass string `help:"Optional HTTP Auth"`
	//state
	fs      http.Handler
	engines map[string]Engine
	rt      *realtime.Realtime
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
	s.rt = realtime.New(realtime.Config{})
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
	//handle realtime client connections
	if p := r.Header.Get("Sec-Websocket-Protocol"); p != "" {
		s.rt.ServeHTTP(w, r)
		return
	}
	//handle realtime javascript
	if strings.HasSuffix(r.URL.Path, "realtime.js") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "text/javascript")
		w.Write(realtime.JS)
		return
	}
	//no match, assume static file
	s.fs.ServeHTTP(w, r)
}
