# velox

[![GoDoc](https://godoc.org/github.com/jpillora/velox?status.svg)](https://godoc.org/github.com/jpillora/velox)

Real-time Go struct to JS object synchronisation over SSE and WebSockets

:warning: *This is beta software. Be wary of using this in production. Please report any [issues](https://github.com/jpillora/velox/issues) you encounter.*

### Features

* Simple API
* Synchronise any JSON marshallable struct
* Delta updates using [JSONPatch (RFC6902)](https://tools.ietf.org/html/rfc6902)
* Supports [Server-Sent Events (EventSource)](https://en.wikipedia.org/wiki/Server-sent_events) and [WebSockets](https://en.wikipedia.org/wiki/WebSocket)
* SSE [client-side poly-fill](https://github.com/remy/polyfills/blob/master/EventSource.js) to fallback to long-polling in older browsers (IE8+).
* Implement delta queries (return all results, then incrementally return changes)

### Quick Usage

Server

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

Client

``` js
// load script /velox.js
var foo = {};
var v = velox("/sync", foo); //uses websockets if supported, otherwise, use sse
v.onupdate = function() {
	//foo.A === 42 and foo.B === 21
};
```

### API

Server API

[![GoDoc](https://godoc.org/github.com/jpillora/velox?status.svg)](https://godoc.org/github.com/jpillora/velox)

Client API

* `velox(url, object)` *function* returns `v` - Creates a new auto-detect velox connection
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
* Object diff has not been optimized. It is a simple property-by-property comparison. :warning: Performance testing has not been done yet.

### TODO

* WebRTC support

#### MIT License

Copyright © 2016 Jaime Pillora &lt;dev@jpillora.com&gt;

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
