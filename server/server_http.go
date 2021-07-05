package server

import (
	"html/template"
	"net/http"
	"strings"

	ctstatic "github.com/jpillora/cloud-torrent/static"
	"github.com/jpillora/velox"
)

var (
	htmlTPL map[string]*template.Template
)

func (s *Server) webHandle(w http.ResponseWriter, r *http.Request) {

	switch r.URL.Path {

	case "/", "index.html":
		htmlTPL["index.html"].Execute(w, struct{ CLDVER string }{s.state.Stats.Version})
		return
	case "/js/velox.js":
		//handle realtime client library
		velox.JS.ServeHTTP(w, r)
		return
	case "/rss":
		s.rssh.ServeHTTP(w, r)
		return
	case "/sync":
		//handle realtime client connections
		conn, err := velox.Sync(&s.state, w, r)
		if err != nil {
			log.Printf("sync failed: %s", err)
			return
		}
		s.syncConnected <- struct{}{}
		s.state.Users[conn.ID()] = r.RemoteAddr
		s.state.Push()
		conn.Wait()
		delete(s.state.Users, conn.ID())
		s.state.Push()
		return
	default:
		//search
		if strings.HasPrefix(r.URL.Path, "/search") {
			s.scraperh.ServeHTTP(w, r)
			return
		}
		//api call
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Access-Control-Allow-Headers", "authorization")
			s.restAPIhandle(w, r)
			return
		}
		//no match, assume static file
		s.files.ServeHTTP(w, r)
	}
}

// restAPIhandle is used both by main webserver and restapi server
func (s *Server) restAPIhandle(w http.ResponseWriter, r *http.Request) {
	ret := "Bad Request"
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		switch r.Method {
		case "POST":
			if err := s.apiPOST(r); err == nil {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("OK"))
			} else {
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
		case "GET":
			if err := s.apiGET(w, r); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	http.Error(w, ret, http.StatusBadRequest)
}

func init() {
	htmlTPL = make(map[string]*template.Template)
	for _, fsn := range []string{"index.html", "template/magadded.html"} {

		c, err := ctstatic.ReadAll(fsn)
		if err != nil {
			log.Fatalln(err)
		}

		htmlTPL[fsn] = template.Must(template.New(fsn).Delims("[[", "]]").Parse(string(c)))
	}
}
