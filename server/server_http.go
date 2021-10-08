package server

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	ctstatic "github.com/boypt/simple-torrent/static"
	"github.com/jpillora/velox"
)

var (
	htmlTPL map[string]*template.Template
)

func (s *Server) webHandle(w http.ResponseWriter, r *http.Request) {

	switch r.URL.Path {

	case "/", "index.html":
		htmlTPL["index.html"].Execute(w, s.baseInfo)
		return
	case "/rss":
		s.rssh.ServeHTTP(w, r)
		return
	case "/sync":
		//handle realtime client connections,
		if r.Header.Get("Accept") == "text/event-stream" {
			// avoid gzip buffer
			w.Header().Set("Content-Encoding", "identity")
		}
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
	case "/js/velox.js":
		velox.JS.ServeHTTP(w, r)
		return
	}

	uptime := time.Unix(s.baseInfo.Uptime, 0)
	cacheSince := uptime.Format(http.TimeFormat)
	cacheExpire := uptime.AddDate(0, 1, 0).Format(http.TimeFormat) // 1 month expire date

	pathDir := strings.SplitN(r.URL.Path[1:], "/", 2)
	switch pathDir[0] {
	case "search":
		s.scraperh.ServeHTTP(w, r)
	case "api":
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		s.restAPIhandle(w, r)
	case "download":
		s.dlfilesh.ServeHTTP(w, r)
	case s.baseInfo.Version:
		w.Header().Set("Last-Modified", cacheSince)
		w.Header().Set("Expires", cacheExpire)
		w.Header().Set("Cache-Control", "max-age:290304000, public")
		s.verStatich.ServeHTTP(w, r)
	default:
		//no match, assume static file
		w.Header().Set("Last-Modified", cacheSince)
		w.Header().Set("Expires", cacheExpire)
		w.Header().Set("Cache-Control", "max-age:290304000, public")
		s.statich.ServeHTTP(w, r)
	}

}

// restAPIhandle is used both by main webserver and restapi server
func (s *Server) restAPIhandle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
		if err := s.apiPOST(r); err != nil {
			http.Error(w, fmt.Sprintf("%s:%s:%v", r.Method, r.URL, err.Error()), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	case "GET":
		if err := s.apiGET(w, r); err != nil {
			http.Error(w, fmt.Sprintf("%s:%s:%v", r.Method, r.URL, err.Error()), http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, fmt.Sprintf("%s:%s:Method Not Allowed", r.Method, r.URL), http.StatusBadRequest)
	}
}

type BaseInfo struct {
	Uptime                int64
	Title                 string
	Version               string
	Runtime               string
	AllowRuntimeConfigure bool
}

func (BaseInfo) GetTemplate(n string) (template.HTML, error) {
	b, err := ctstatic.ReadAll(n)
	if err != nil {
		return "", err
	}
	return template.HTML(b), nil
}

func init() {
	htmlTPL = make(map[string]*template.Template)
	for _, fsn := range []string{"index.html", "magadded.html"} {

		c, err := ctstatic.ReadAll(fsn)
		if err != nil {
			log.Fatalln(err)
		}

		htmlTPL[fsn] = template.Must(template.New(fsn).Delims("[[", "]]").Parse(string(c)))
	}
}
