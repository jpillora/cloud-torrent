package main

import (
	"log"
	"math/rand"
	"net/http"
	// _ "net/http/pprof"

	"sync"
	"time"

	"github.com/jpillora/velox"
)

type Foo struct {
	//required velox state, adds sync state and a Push() method
	velox.State
	//optional mutex, prevents race conditions (foo.Push will make use of the sync.Locker interface)
	sync.Mutex
	A, B int
	C    Bar
}

type Bar struct {
	X, Y int
}

func main() {
	//state we wish to sync
	foo := &Foo{A: 21, B: 42}
	go func() {
		for {
			foo.Lock()
			foo.C.X = rand.Intn(99)
			foo.C.Y = rand.Intn(99)
			foo.Unlock()
			//push to all connections
			foo.Push()
			//do other stuff...
			time.Sleep(2500 * time.Millisecond)
		}
	}()
	//sync handlers
	http.Handle("/velox.js", velox.JS)
	http.Handle("/sync", velox.SyncHandler(foo))
	//index handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(indexhtml)
	})
	//listen!
	log.Printf("Listening on 7070...")
	log.Fatal(http.ListenAndServe(":7070", nil))
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
	document.querySelector("#example").innerHTML = JSON.stringify(foo, null, 2);
};
</script>
`)
