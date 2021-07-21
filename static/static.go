package ctstatic

import (
	"embed"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

//go:embed files
var staticFS embed.FS

var curFS fs.FS

const resourcePath = "static/files"

func init() {
	if info, err := os.Stat(resourcePath); err == nil && info.IsDir() {
		log.Printf("[static] found %s, using external resources.", resourcePath)
		curFS = os.DirFS(resourcePath)
	} else {
		curFS, _ = fs.Sub(staticFS, "files")
	}
}

func FileSystemHandler() http.Handler {
	return http.FileServer(http.FS(curFS))
}

func ReadAll(name string) ([]byte, error) {
	f, err := curFS.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ioutil.ReadAll(f)
}
