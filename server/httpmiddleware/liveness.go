package httpmiddleware

import (
	"net/http"
)

func Liveness(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// liveness response
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
			return
		}
		h.ServeHTTP(w, r)
	})
}
