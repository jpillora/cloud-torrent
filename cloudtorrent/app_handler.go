package cloudtorrent

import (
	"log"
	"net/http"
	"time"

	goji "goji.io"
	"goji.io/pat"
	"golang.org/x/net/context"

	"github.com/jpillora/gziphandler"
	"github.com/jpillora/requestlog"
	"github.com/jpillora/velox"
	"github.com/zenazn/goji/web/middleware"
)

func (a *App) routes() http.Handler {
	mux := goji.NewMux()
	//middleware
	mux.Use(middleware.RealIP)
	if a.Log {
		mux.Use(func(next http.Handler) http.Handler {
			return requestlog.WrapWith(next, requestlog.Options{
				Format: `{{ if .Timestamp }}{{ .Timestamp }} [Web] {{end}}` +
					`{{ .Method }} {{ .Path }} {{ .CodeColor }}{{ .Code }}{{ .Reset }} ` +
					`{{ .Duration }}{{ if .Size }} {{ .Size }}{{end}}` +
					`{{ if .IP }} ({{ .IP }}){{end}}` + "\n",
				TimeFormat: "2006/01/02 15:04:05",
			})
		})
	}
	mux.Use(a.auth.Wrap)
	mux.Use(gziphandler.GzipHandler)
	//handlers
	mux.Handle(pat.Get("/js/velox.js"), velox.JS)
	mux.HandleFunc(pat.Get("/sync"), a.veloxSync)
	mux.Handle(pat.Get("/search/*"), a.scraperh)
	mux.Handle(pat.Post("/api/configure"), errhand(a.handleConfigure))
	mux.HandleFunc(pat.Get("/*"), func(w http.ResponseWriter, r *http.Request) {
		log.Printf("fallback handle: %s %s", r.Method, r.URL)
		a.static.ServeHTTP(w, r)
	})
	return mux
}

func (a *App) veloxSync(w http.ResponseWriter, r *http.Request) {
	if conn, err := velox.Sync(&a.state, w, r); err != nil {
		log.Print(err)
	} else {
		//add user
		a.state.Lock()
		a.state.Users[r.RemoteAddr] = time.Now().UTC()
		a.state.Unlock()
		a.state.Push()
		//block
		log.Printf("[Web] User (%s) connected", r.RemoteAddr)
		conn.Wait()
		log.Printf("[Web] User (%s) disconnected", r.RemoteAddr)
		//remove user
		a.state.Lock()
		delete(a.state.Users, r.RemoteAddr)
		a.state.Unlock()
		a.state.Push()
	}
}

func errhand(fn func(ctx context.Context, w http.ResponseWriter, r *http.Request) error) goji.HandlerFunc {
	return goji.HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		if err := fn(ctx, w, r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	})
}
