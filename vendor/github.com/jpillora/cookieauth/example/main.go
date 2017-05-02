package main

import (
	"log"
	"net/http"
	"os"

	"github.com/jpillora/cookieauth"
)

func main() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html>
			<link rel="shortcut icon" href="data:image/x-icon;," type="image/x-icon">
			<body>hello world</body>
			</html>`))
	})

	//custom usage
	ca := cookieauth.New()
	ca.SetUserPass("foo", "bar")
	ca.SetLogger(log.New(os.Stdout, "", log.LstdFlags))
	protected := ca.Wrap(handler)

	log.Print("listening on 8000...")
	log.Fatal(http.ListenAndServe(":8000", protected))
}
