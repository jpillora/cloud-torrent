package ctstatic

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed files/*
var staticFiles embed.FS

// all static/ files embedded as a Go library
func FileSystemHandler() http.Handler {
	fs, err := fs.Sub(staticFiles, "files")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(fs))
}
