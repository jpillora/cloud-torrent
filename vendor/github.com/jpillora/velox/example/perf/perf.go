package main

import (
	"bytes"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/jpillora/gziphandler"
	"github.com/jpillora/velox"
)

type Book struct {
	//required velox state, adds sync state and a Push() method
	velox.State
	//optional mutex, prevents race conditions (foo.Push will make use of the sync.Locker interface)
	sync.Mutex
	Lines map[int]string
}

func main() {
	//state we wish to sync
	b := &Book{Lines: map[int]string{}}
	txt, err := ioutil.ReadFile("sample.txt")
	if err != nil {
		log.Fatal(err)
	}
	lines := bytes.Split(txt, []byte{'\n'})
	i := 0
	for n := 0; n < 10; n++ {
		for _, line := range lines {
			if len(line) == 0 || string(line) == "\r" {
				continue
			}
			b.Lines[i] = string(line)
			i++
		}
	}

	//swap lines
	go func() {
		for {
			x, y := rand.Intn(i), rand.Intn(i)
			b.Lock()
			b.Lines[x], b.Lines[y] = b.Lines[y], b.Lines[x]
			b.Unlock()
			b.Push()
			time.Sleep(100 * time.Millisecond)
		}
	}()

	//show goroutine/memory usage
	// go func() {
	// 	mem := runtime.MemStats{}
	// 	for {
	// 		g := runtime.NumGoroutine()
	// 		runtime.ReadMemStats(&mem)
	// 		time.Sleep(1000 * time.Millisecond)
	// 		log.Printf("goroutines: %03d, allocated %s", g, sizestr.ToString(int64(mem.Alloc)))
	// 	}
	// }()

	b.State.WriteTimeout = 3 * time.Second

	//sync handlers
	router := http.NewServeMux()
	router.Handle("/velox.js", velox.JS)
	router.Handle("/sync", velox.SyncHandler(b))
	//index handler
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(indexhtml)
	})

	//jpillora/gziphandler ignores websocket/eventsource connections
	//and gzips the rest
	gzippedRouter := gziphandler.GzipHandler(router)

	//listen!
	log.Printf("Listening on 7070...")
	log.Fatal(http.ListenAndServe(":7070", gzippedRouter))
}

var indexhtml = []byte(`
<div>Status: <b id="status">disconnected</b></div>
<pre id="example"></pre>
<script src="/velox.js?dev=1"></script>
<script>
var foo = {};
var v = velox("/sync", foo);
v.onchange = function(isConnected) {
	document.querySelector("#status").innerHTML = isConnected ? "connected" : "disconnected";
};
v.onupdate = function() {
	document.querySelector("#example").innerHTML = JSON.stringify(foo, null, 2).length;
};
</script>
`)
