//go:generate go-bindata -pkg velox -o files.go -prefix ../js/build/ ../js/build/bundle.js

package velox

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strings"
)

var JS = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
	filename := "bundle.js"
	b, _ := Asset(filename)
	info, _ := AssetInfo(filename)
	//requested compression and not already compressed?
	if strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") && w.Header().Get("Content-Encoding") != "gzip" {
		gb := bytes.Buffer{}
		g := gzip.NewWriter(&gb)
		g.Write(b)
		g.Close()
		b = gb.Bytes()
		w.Header().Set("Content-Encoding", "gzip")
	}
	buff := bytes.NewReader(b)
	//serve
	http.ServeContent(w, req, info.Name(), info.ModTime(), buff)
})
