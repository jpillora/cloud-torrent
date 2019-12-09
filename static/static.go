//go:generate go-bindata -pkg ctstatic -ignore .../.DS_Store -o files.go files/...

package ctstatic

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/elazarl/go-bindata-assetfs"
)

// FileSystemHandler all static/ files embedded as a Go library
func FileSystemHandler() http.Handler {
	if info, err := os.Stat("static/files/"); err == nil && info.IsDir() {
		log.Println("Using local static files")
		return http.FileServer(http.Dir("static/files/"))
	}
	return http.FileServer(&assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, AssetInfo: AssetInfo, Prefix: "files"})
}

// ReadAll return local file if exists
func ReadAll(name string) ([]byte, error) {
	if info, err := os.Stat("static/files/"); err == nil && info.IsDir() {
		diskPath := "static/files/" + name
		f, err := os.Open(diskPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return ioutil.ReadAll(f)
	}
	return Asset("files/" + name)
}
