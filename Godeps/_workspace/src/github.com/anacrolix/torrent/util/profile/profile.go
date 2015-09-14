package profile

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
)

func init() {
	if httpAddr := os.Getenv("GOPROF"); httpAddr != "" {
		go func() {
			err := http.ListenAndServe(httpAddr, nil)
			if err != nil {
				log.Print(err)
			}
		}()
	}
}
