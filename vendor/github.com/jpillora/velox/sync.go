package velox

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync/atomic"

	"github.com/jpillora/velox/assets"
)

//NOTE(@jpillora): always assume v1, include v2 in checks when we get there...
const proto = "v1"

var JS = assets.VeloxJS

type syncer interface {
	sync(gostruct interface{}) (*State, error)
}

//SyncHandler is a small wrapper around Sync which simply synchronises
//all incoming connections. Use Sync if you wish to implement user authentication
//or any other request-time checks.
func SyncHandler(gostruct interface{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if conn, err := Sync(gostruct, w, r); err != nil {
			log.Printf("[velox] sync handler error: %s", err)
		} else {
			conn.Wait()
		}
	})
}

var connectionID int64

//Sync upgrades a given HTTP connection into a WebSocket connection and synchronises
//the provided struct with the client. velox takes responsibility for writing the response
//in the event of failure. Default handlers close the TCP connection on return so when
//manually using this method, you'll most likely want to block using Conn.Wait().
func Sync(gostruct interface{}, w http.ResponseWriter, r *http.Request) (Conn, error) {
	//access gostruct.State via interfaces:
	gosyncable, ok := gostruct.(syncer)
	if !ok {
		return nil, fmt.Errorf("velox sync failed: struct does not embed velox.State")
	}
	//extract internal state from gostruct
	state, err := gosyncable.sync(gostruct)
	if err != nil {
		return nil, fmt.Errorf("velox sync failed: %s", err)
	}
	version := int64(0)
	//matching id, allow user to pick version
	if id := r.URL.Query().Get("id"); id != "" && id == state.data.id {
		if v, err := strconv.ParseInt(r.URL.Query().Get("v"), 10, 64); err == nil && v > 0 {
			version = v
		}
	}
	//set initial connection state
	conn := newConn(atomic.AddInt64(&connectionID, 1), r.RemoteAddr, state, version)
	//attempt connection over transport
	//(negotiate websockets / start eventsource emitter)
	//return when connected
	if err := conn.connect(w, r); err != nil {
		return nil, fmt.Errorf("velox connection failed: %s", err)
	}
	//hand over to state to keep in sync
	state.subscribe(conn)
	//do an initial push only to this client
	conn.Push()
	//pass connection to user
	return conn, nil
}
