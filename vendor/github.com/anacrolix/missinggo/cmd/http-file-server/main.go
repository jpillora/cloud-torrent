package main

import (
	"log"
	"net"
	"net/http"
	"os"

	"github.com/anacrolix/tagflag"
)

func setIfGetHeader(w http.ResponseWriter, r *http.Request, set, get string) {
	h := r.Header.Get(get)
	if h == "" {
		return
	}
	w.Header().Set(set, h)
}

func allowCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			setIfGetHeader(w, r, "Access-Control-Allow-Methods", "Access-Control-Request-Method")
			setIfGetHeader(w, r, "Access-Control-Allow-Headers", "Access-Control-Request-Headers")
			w.Header().Set("Access-Control-Expose-Headers", "Content-Type")
		}
		h.ServeHTTP(w, r)
	})
}

func main() {
	var flags = struct {
		Addr string
	}{
		Addr: "localhost:8080",
	}
	tagflag.Parse(&flags)
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	l, err := net.Listen("tcp", flags.Addr)
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
	addr := l.Addr()
	log.Printf("serving %q at %s", dir, addr)
	log.Fatal(http.Serve(l, allowCORS(http.FileServer(http.Dir(dir)))))
}
