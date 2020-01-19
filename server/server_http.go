package server

import (
	"log"
	"net/http"
	"strings"

	"github.com/jpillora/velox"
)

func (s *Server) webHandle(w http.ResponseWriter, r *http.Request) {
	//handle realtime client library
	if r.URL.Path == "/js/velox.js" {
		velox.JS.ServeHTTP(w, r)
		return
	}
	if r.URL.Path == "/rss" {
		s.rssh.ServeHTTP(w, r)
		return
	}
	//handle realtime client connections
	if r.URL.Path == "/sync" {
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
		w.Header().Set("Access-Control-Allow-Headers", "authorization")
		s.restAPIhandle(w, r)
		return
	}
	//no match, assume static file
	s.files.ServeHTTP(w, r)
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
