// Package static serves the built frontend single-page app, embedded into the
// binary so cloud-torrent ships as a single executable.
package static

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
)

// dist holds the built frontend (vite build -> static/dist). The `all:` prefix
// embeds the directory even when it only contains the .gitkeep placeholder
// (i.e. before the first `npm run build`), so `go build` always compiles.
//
//go:embed all:dist
var dist embed.FS

// Handler serves the embedded SPA. For a fast dev loop, set CT_STATIC_DIR to a
// directory of built assets on disk and it is served from there instead of the
// embedded copy (parity with master's serveFiles local-dir fallback).
func Handler() http.Handler {
	if dir := os.Getenv("CT_STATIC_DIR"); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return http.FileServer(http.Dir(dir))
		}
	}
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
