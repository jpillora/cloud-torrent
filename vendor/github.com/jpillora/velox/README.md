# velox

[![GoDoc](https://godoc.org/github.com/jpillora/velox?status.svg)](https://godoc.org/github.com/jpillora/velox)

Real-time JS object synchronisation over SSE and WebSockets in Go and JavaScript (Node.js and browser)

### Features

* Simple API
* Synchronise any JSON marshallable struct in Go
* Synchronise any JSON stringifiable struct in Node
* Delta updates using [JSONPatch (RFC6902)](https://tools.ietf.org/html/rfc6902)
* Supports [Server-Sent Events (EventSource)](https://en.wikipedia.org/wiki/Server-sent_events) and [WebSockets](https://en.wikipedia.org/wiki/WebSocket)
* SSE [client-side poly-fill](https://github.com/remy/polyfills/blob/master/EventSource.js) to fallback to long-polling in older browsers (IE8+).
* Implement delta queries (return all results, then incrementally return changes)

### Quick Usage

Server (Go)

``` go
//syncable struct
type Foo struct {
	velox.State
	A, B int
}
foo := &Foo{}
//serve velox.js client library (assets/dist/velox.min.js)
http.Handle("/velox.js", velox.JS)
//serve velox sync endpoint for foo
http.Handle("/sync", velox.SyncHandler(foo))
//make changes
foo.A = 42
foo.B = 21
//push to client
foo.Push()
```

Server (Node)

``` js
//syncable object
foo := &Foo{
	a: 1,
	b: 2
}
//express server
let app = express();
//serve velox.js client library (assets/dist/velox.min.js)
app.get("/velox.js", velox.JS)
//serve velox sync endpoint for foo
app.get("/sync", velox.handle(foo))
//make changes
foo.a = 42
foo.b = 21
//push to client
foo.$push()
```

Client (Node and Browser)

``` js
// load script /velox.js
var foo = {};
var v = velox("/sync", foo);
v.onupdate = function() {
	//foo.A === 42 and foo.B === 21
};
```

### API

Server API (Go)

[![GoDoc](https://godoc.org/github.com/jpillora/velox?status.svg)](https://godoc.org/github.com/jpillora/velox)

Server API (Node)

* `velox.handle(object)` *function* returns `v` - Creates a new route handler for use with express
* `velox.state(object)` *function* returns `state` - Creates or restores a velox state from a given object
* `state.handle(req, res)` *function* returns `Promise` - Handle the provided `express` request/response. Resolves on connection close. Rejects on any error.

Client API (Node and Browser)

* `velox(url, object)` *function* returns `v` - Creates a new SSE velox connection
* `velox.sse(url, object)` *function* returns `v` - Creates a new SSE velox connection
* `velox.ws(url, object)` *function* returns `v` - Creates a new WS velox connection
* `v.onupdate(object)` *function* - Called when a server push is received
* `v.onerror(err)` *function* - Called when a connection error occurs
* `v.onconnect()` *function* - Called when the connection is opened
* `v.ondisconnect()` *function* - Called when the connection is closed
* `v.onchange(bool)` *function* - Called when the connection is opened or closed
* `v.connected` *bool* - Denotes whether the connection is currently open
* `v.ws` *bool* - Denotes whether the connection is in web sockets mode
* `v.sse` *bool* - Denotes whether the connection is in server-sent events mode

### Example

See this [simple `example/`](example/) and view it live here: https://velox.jpillora.com

![screenshot](https://cloud.githubusercontent.com/assets/633843/13481947/8eea1804-e13d-11e5-80c8-be9317c54fbc.png)

*Here is a screenshot from this example page, showing the messages arriving as either a full replacement of the object or just a delta. The server will send which ever is smaller.*

### Notes

* JS object properties beginning with `$` will be ignored to play nice with Angular.
* JS object with an `$apply` function will automatically be called on each update to play nice with Angular.
* `velox.SyncHandler` is just a small wrapper around `velox.Sync`:

	```go
	func SyncHandler(gostruct interface{}) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if conn, err := Sync(gostruct, w, r); err == nil {
				conn.Wait()
			}
		})
	}
	```

### Known issues

* Object synchronization is currently one way (server to client) only.
* Object diff has not been optimized. It is a simple property-by-property comparison.

### TODO

* WebRTC support
* Plain [`http`](https://nodejs.org/api/http.html#http_http_createserver_requestlistener) server support in Node
* WebSockets support in Node

#### MIT License

Copyright © 2017 Jaime Pillora &lt;dev@jpillora.com&gt;

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
'Software'), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED 'AS IS', WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
