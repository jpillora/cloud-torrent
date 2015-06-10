package embed

import (
	"log"
	"mime"
	"net/http"
	"os"

	"github.com/elazarl/go-bindata-assetfs"
)

// all static/ files embedded as a Go library
func FileSystemHandler() http.Handler {

	mime.AddExtensionType(".woff", "application/x-font-woff")
	mime.AddExtensionType(".woff2", "font/woff2")

	var h http.Handler
	if info, err := os.Stat("embed/"); err == nil && info.IsDir() {
		log.Printf("using local fs embed/ directory")
		h = http.FileServer(http.Dir("embed/"))
	} else {
		h = http.FileServer(&assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, Prefix: "embed"})
	}
	return h
}
