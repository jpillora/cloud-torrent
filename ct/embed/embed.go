package embed

import (
	"log"
	"mime"
	"net/http"
	"os"

	"github.com/elazarl/go-bindata-assetfs"
)

type wrapFS struct {
	FS http.Handler
}

func (wrap *wrapFS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// if strings.HasSuffix(r.URL.Path, ".woff") {
	// 	log.Printf("set font")
	// 	w.Header().Set("Content-Type", "font/woff")
	// }
	wrap.FS.ServeHTTP(w, r)
}

// all static/ files embedded as a Go library
func FileSystemHandler() http.Handler {

	mime.AddExtensionType(".woff", "application/x-font-woff")
	mime.AddExtensionType(".woff2", "font/woff2")

	wrapper := &wrapFS{}
	if info, err := os.Stat("embed/"); err == nil && info.IsDir() {
		log.Printf("using local fs embed/ directory")
		wrapper.FS = http.FileServer(http.Dir("embed/"))
	} else {
		wrapper.FS = http.FileServer(&assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, Prefix: "embed"})
	}
	return wrapper
}
