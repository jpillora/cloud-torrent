package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jpillora/go-realtime"
)

type Foo struct {
	realtime.Object
	Lines []string
}

func main() {

	i := 0
	const size = 1000

	b, err := ioutil.ReadFile("data.txt")
	if err != nil {
		log.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	foo := &Foo{Lines: lines[i : size+i]}

	//create a go-realtime (websockets) http.Handler
	rt := realtime.NewHandler()
	//register foo
	rt.Add("foo", foo)

	go func() {
		for {
			i++
			if size+i == len(lines) {
				break
			}
			foo.Lines = lines[i : size+i]
			//mark updated
			foo.Update()
			//do other stuff...
			time.Sleep(10 * time.Millisecond)
		}
	}()

	//realtime handlers
	http.Handle("/realtime", rt)
	http.Handle("/realtime.js", realtime.JS)
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
<script src="realtime.js"></script>
<script>
	var foo = {};
	var rt = realtime("/realtime");
	//keep in sync with the server
	rt.add("foo", foo, function onupdate() {
		out.innerHTML = foo.Lines.join("\n");
	});
</script>
`)

//NOTE: deltas are not sent in the example since the target object is too small
