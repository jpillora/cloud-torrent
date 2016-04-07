
:warning: This project has been rewritten, simplified and now been deprecated.

## See https://github.com/jpillora/velox

---

### go-realtime

Keep your Go structs in sync with your JS objects

:warning: This project is in beta and the API may change

### Features

* Simple API
* Works with any JSON marshallable struct
* Delta updates using JSONPatch

### Quick Usage

Server

``` go
type Foo struct {
	realtime.Object
	A, B int
}
foo := &Foo{}

//create handler and add foo
rt := realtime.NewHandler()
rt.Add("foo", foo)

//serve websockets and realtime.js client library 
http.Handle("/realtime", rt)

//...later...

//make changes
foo.A = 42
//push to client
foo.Update()
```

Client

``` js
var foo = {};

var rt = realtime("/realtime");

rt.add("foo", foo, function onupdate() {
	//do stuff with foo...
});
```

### Example

See [example](example/) which is running live here https://go-realtime-demo.herokuapp.com/

### Notes

* Object synchronization is currently one way (server to client) only.
* Client object properties beginning with `$` will be ignored.

#### MIT License

Copyright © 2015 Jaime Pillora &lt;dev@jpillora.com&gt;

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
