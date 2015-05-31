package embed

import (
	"log"
	"net/http"
	"os"

	"github.com/elazarl/go-bindata-assetfs"
)

// all static/ files embedded as a Go library
func FileSystemHandler() http.Handler {
	if info, err := os.Stat("embed/"); err == nil && info.IsDir() {
		log.Printf("using local fs embed/ directory")
		return http.FileServer(http.Dir("embed/"))
	}
	return http.FileServer(&assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, Prefix: "embed"})
}
