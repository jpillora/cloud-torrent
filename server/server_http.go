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
		//only pass request in, expect error out
		err := s.api(r)
		if err == nil {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}
		switch err {
		case errTaskAdded:
			// internal rewrite to show status page
			r.URL.Path = "/sub/magadded.html"
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
	}
	//no match, assume static file
	s.files.ServeHTTP(w, r)
}

func (s *Server) resstAPIhandle(w http.ResponseWriter, r *http.Request) {
	ret := "Bad Request"
	if strings.HasPrefix(r.URL.Path, "/api/") {
		err := s.api(r)
		if err == nil {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}
		if err == errTaskAdded {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}
		ret = err.Error()
	}
	w.WriteHeader(http.StatusBadRequest)
	w.Write([]byte(ret))
}

func livenessWrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// liveness response
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}
		h.ServeHTTP(w, r)
	})
}
