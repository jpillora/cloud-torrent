//go:generate go-bindata -pkg ctstatic -ignore .../.DS_Store -o files.go files/...

package ctstatic

import (
	"net/http"
	"os"

	"github.com/elazarl/go-bindata-assetfs"
)

// all static/ files embedded as a Go library
func FileSystemHandler() http.Handler {
	var h http.Handler
	if info, err := os.Stat("static/files/"); err == nil && info.IsDir() {
		h = http.FileServer(http.Dir("static/files/"))
	} else {
		h = http.FileServer(&assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, AssetInfo: AssetInfo, Prefix: "files"})
	}
	return h
}
