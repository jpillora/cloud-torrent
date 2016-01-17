package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jpillora/go-realtime"
)

type Foo struct {
	realtime.Object //adds sync state and an Update() method

	A, B int
	C    []int
	D    string
	E    Bar
}

type Bar struct {
	X, Y int
}

func main() {

	foo := &Foo{A: 21, B: 42, D: "0"}

	//create a go-realtime (websockets) http.Handler
	rt := realtime.NewHandler()
	//register foo
	rt.Add("foo", foo)

	go func() {
		i := 0
		for {
			//change foo
			foo.A++
			if i%2 == 0 {
				foo.B--
			}
			i++
			if i > 10 {
				foo.C = foo.C[1:]
			}
			foo.C = append(foo.C, i)
			if i%5 == 0 {
				foo.E.Y++
			}
			//mark updated
			foo.Update()
			//do other stuff...
			time.Sleep(250 * time.Millisecond)
		}
	}()

	//realtime handlers
	http.Handle("/realtime", rt)
	//index handler
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(indexhtml)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	log.Printf("Listening on localhost:%s...", port)
	http.ListenAndServe(":"+port, nil)
}

var indexhtml = []byte(`
<pre id="out"></pre>
<script src="/realtime"></script>
<script>
	var foo = {};

	var rt = realtime("/realtime");

	//keep in sync with the server
	rt.add("foo", foo, function onupdate() {
		out.innerHTML = JSON.stringify(foo, null, 2);
	});
</script>
`)

//NOTE: deltas are not sent in the example since the target object is too small
