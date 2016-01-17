//go:generate go-bindata -pkg realtime -o realtime_embed.go realtime.js

package realtime

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var proto = "v1"

type Config struct {
	Throttle    time.Duration
	CheckOrigin bool
}

type Handler struct {
	config   Config
	upgrader websocket.Upgrader
	ws       http.Handler
	mut      sync.Mutex //protects object and user maps
	objs     map[key]*Object
	users    map[string]*User

	watchingUsers bool
	userEvents    chan *User
}

func NewHandler() *Handler {
	return NewHandlerConfig(Config{})
}

func NewHandlerConfig(c Config) *Handler {
	if c.Throttle < 15*time.Millisecond {
		//15ms is approximately highest resolution on the JS eventloop
		c.Throttle = 200 * time.Millisecond
	}
	r := &Handler{config: c}

	r.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	if !c.CheckOrigin {
		r.upgrader.CheckOrigin = func(r *http.Request) bool {
			return true
		}
	}
	r.objs = map[key]*Object{}
	r.users = map[string]*User{}
	r.userEvents = make(chan *User)
	//continually batches and sends updates
	go r.flusher()
	return r
}

func (r *Handler) UserEvents() <-chan *User {
	if r.watchingUsers {
		panic("Already watching user changes")
	}
	r.watchingUsers = true
	return r.userEvents
}

func (r *Handler) flusher() {
	//loops at Throttle speed
	for {
		//compute updates for each object for each subscriber
		//and append each update to the users pending updates
		for _, o := range r.objs {
			o.computeUpdate()
		}
		//send all pending updates
		r.mut.Lock()
		for _, u := range r.users {
			u.sendPending()
		}
		r.mut.Unlock()
		time.Sleep(r.config.Throttle)
	}
}

type addable interface {
	add(key string, val interface{}) (*Object, error)
}

func (r *Handler) MustAdd(k string, v interface{}) {
	if err := r.Add(k, v); err != nil {
		panic(err)
	}
}

func (r *Handler) Add(k string, v interface{}) error {
	r.mut.Lock()
	defer r.mut.Unlock()

	t := reflect.TypeOf(v)
	if t.Kind() != reflect.Ptr {
		return fmt.Errorf("Cannot add '%s' - it is not a pointer type", k)
	}

	if _, ok := r.objs[key(k)]; ok {
		return fmt.Errorf("Cannot add '%s' - already exists", k)
	}
	//access v.object via interfaces:
	a, ok := v.(addable)
	if !ok {
		return fmt.Errorf("Cannot add '%s' - does not embed realtime.Object", k)
	}
	//pass v into v.object and get v.object back out
	o, err := a.add(k, v)
	if err != nil {
		return fmt.Errorf("Cannot add '%s' %s", k, err)
	}
	r.objs[key(k)] = o
	return nil
}

func (r *Handler) UpdateAll() {
	r.mut.Lock()
	for _, obj := range r.objs {
		obj.checked = false
	}
	r.mut.Unlock()
}

func (r *Handler) Update(k string) {
	r.mut.Lock()
	if obj, ok := r.objs[key(k)]; ok {
		obj.checked = false
	}
	r.mut.Unlock()
}

func (r *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Upgrade") == "websocket" {
		conn, err := r.upgrader.Upgrade(w, req, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.serveWS(conn)
	} else if strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		JS.ServeHTTP(w, req) //gziped realtime.js script file
	} else {
		http.NotFound(w, req)
	}
}

func (r *Handler) serveWS(conn *websocket.Conn) {
	handshake := struct {
		Protocol       string
		ObjectVersions objectVersions
	}{}

	//first message is the rt handshake
	if err := conn.ReadJSON(&handshake); err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Invalid rt handshake"))
		conn.Close()
		return
	}
	if handshake.Protocol != proto {
		conn.WriteMessage(websocket.TextMessage, []byte("Invalid rt protocol version"))
		conn.Close()
		return
	}
	//ready
	u := &User{
		ID:        conn.RemoteAddr().String(),
		Connected: true,
		uptime:    time.Now(),
		conn:      conn,
		versions:  handshake.ObjectVersions,
		pending:   []*update{},
	}

	//add user and subscribe to each obj
	r.mut.Lock()
	for k := range u.versions {
		if _, ok := r.objs[k]; !ok {
			conn.WriteMessage(websocket.TextMessage, []byte("missing object: "+k))
			r.mut.Unlock()
			return
		}
	}
	r.users[u.ID] = u
	if r.watchingUsers {
		r.userEvents <- u
	}
	for k := range u.versions {
		obj := r.objs[k]
		obj.subscribers[u.ID] = u
		//create initial update
		u.pending = append(u.pending, &update{
			Key: k, Version: obj.version, Data: obj.bytes,
		})
		obj.Update()
	}
	r.mut.Unlock()

	//loop here during connection
	for {
		//msgType, msgBytes, err
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
		//TODO(jpillora): allow changing of subscriptions
	}
	conn.Close() //ensure closed
	u.Connected = false
	//remove user and unsubscribe to each obj
	r.mut.Lock()
	delete(r.users, u.ID)
	if r.watchingUsers {
		r.userEvents <- u
	}
	for k := range u.versions {
		obj := r.objs[k]
		delete(obj.subscribers, u.ID)
	}
	r.mut.Unlock()
	//disconnected
}

//embedded JS file
var JSBytes = _realtimeJs

type jsServe []byte

var JS = jsServe(JSBytes)

func (j jsServe) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Content-Type", "text/javascript")
	w.Header().Set("Content-Length", strconv.Itoa(len(JSBytes)))
	w.Write(JSBytes)
}
