package main

import (
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/jpillora/velox"
)

//debug enables goroutine and memory counters
const debug = false

type Foo struct {
	//required velox state, adds sync state and a Push() method
	velox.State
	//optional mutex, prevents race conditions (foo.Push will make use of the sync.Locker interface)
	sync.Mutex
	NumConnections int
	NumGoRoutines  int     `json:",omitempty"`
	AllocMem       float64 `json:",omitempty"`
	A, B           int
	C              map[string]int
	D              Bar
}

type Bar struct {
	X, Y int
}

func main() {
	//state we wish to sync
	foo := &Foo{A: 21, B: 42, C: map[string]int{}}
	go func() {
		i := 0
		for {
			//change foo
			foo.Lock()
			foo.A++
			if i%2 == 0 {
				foo.B--
			}
			i++
			foo.C[string('A'+rand.Intn(26))] = i
			if i%2 == 0 {
				j := 0
				rmj := rand.Intn(len(foo.C))
				for k, _ := range foo.C {
					if j == rmj {
						delete(foo.C, k)
						break
					}
					j++
				}
			}
			if i%5 == 0 {
				foo.D.X--
				foo.D.Y++
			}
			foo.NumConnections = foo.State.NumConnections() //show number of connections 'foo' is currently handling
			foo.Unlock()
			//push to all connections
			foo.Push()
			//do other stuff...
			time.Sleep(250 * time.Millisecond)
		}
	}()
	//show memory/goroutine stats
	if debug {
		go func() {
			mem := &runtime.MemStats{}
			i := 0
			for {
				foo.NumGoRoutines = runtime.NumGoroutine()
				runtime.ReadMemStats(mem)
				foo.AllocMem = float64(mem.Alloc)
				time.Sleep(100 * time.Millisecond)
				i++
				// if i%10 == 0 { runtime.GC() }
				foo.Push()
			}
		}()
	}
	//sync handlers
	http.Handle("/velox.js", velox.JS)
	http.Handle("/sync", velox.SyncHandler(foo))
	//index handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(indexhtml)
	})
	//listen!
	port := os.Getenv("PORT")
	if port == "" {
		port = "7070"
	}
	log.Printf("Listening on :%s...", port)
	s := http.Server{
		Addr: ":" + port,
		//these will be ignored by velox.SyncHandler, see State.WriteTimeout
		ReadTimeout:  42 * time.Millisecond,
		WriteTimeout: 42 * time.Millisecond,
	}
	log.Fatal(s.ListenAndServe())
}

var indexhtml = []byte(`
<!-- documentation -->
Client:<br>
<pre id="code">Status: &lt;div>&lt;b id="status">disconnected&lt;/b>&lt;/div>
&lt;pre id="example">&lt;/pre>
&lt;script src="/velox.js">&lt;/script>
&lt;script>
var foo = {};
var v = velox("/sync", foo);
v.onchange = function(isConnected) {
	document.querySelector("#status").innerHTML = isConnected ? "connected" : "disconnected";
};
v.onupdate = function() {
	document.querySelector("#example").innerHTML = JSON.stringify(foo, null, 2);
};
&lt;/script>
</pre>
<a href="https://github.com/jpillora/velox"><img style="position: absolute; z-index: 2; top: 0; right: 0; border: 0;" src="https://s3.amazonaws.com/github/ribbons/forkme_right_darkblue_121621.png" alt="Fork me on GitHub"></a>
<hr>

Server:<br>
<a href="https://github.com/jpillora/velox/blob/master/example/demo/main.go" target="_blank">
	https://github.com/jpillora/velox/blob/master/example/demo/main.go
</a>
<hr>

<!-- example -->
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
