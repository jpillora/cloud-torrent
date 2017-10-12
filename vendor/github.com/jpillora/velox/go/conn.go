package velox

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

//Conn represents a single live connection being synchronised.
//ID is current set to the connection's remote address.
type Conn interface {
	ID() string
	Connected() bool
	Wait()
	Push()
	Close() error
}

type conn struct {
	transport   transport
	state       *State
	connected   bool
	connectedCh chan struct{}
	waiter      sync.WaitGroup
	id          int64
	addr        string
	first       uint32
	pushing     uint32
	queued      uint32
	uptime      time.Time
	version     int64
	sendingMut  sync.Mutex //for msg/ping
}

func newConn(id int64, addr string, state *State, version int64) *conn {
	return &conn{
		connectedCh: make(chan struct{}),
		id:          id,
		addr:        addr,
		state:       state,
		version:     version,
	}
}

//ID of this connection
func (c *conn) ID() string {
	return strconv.FormatInt(c.id, 10)
}

//Status of this connection, should be true initially, then false after Wait().
func (c *conn) Connected() bool {
	return c.connected
}

//Wait will block until the connection is closed.
func (c *conn) Wait() {
	c.waiter.Wait()
}

//Push will the current state only to this client.
//Blocks until push is complete.
func (c *conn) Push() {
	c.push()
}

//Force close the connection.
func (c *conn) Close() error {
	return c.transport.close()
}

//connect using the provided transport
//and block until connection is ready
func (c *conn) connect(w http.ResponseWriter, r *http.Request) error {
	//choose transport
	if r.Header.Get("Accept") == "text/event-stream" {
		c.transport = &eventSourceTransport{writeTimeout: c.state.WriteTimeout}
	} else if r.Header.Get("Upgrade") == "websocket" {
		c.transport = &websocketsTransport{writeTimeout: c.state.WriteTimeout}
	} else {
		return fmt.Errorf("Invalid sync request")
	}
	//non-blocking connect to client over set transport
	if err := c.transport.connect(w, r); err != nil {
		return err
	}
	//initial ping
	if err := c.send(&update{Ping: true}); err != nil {
		return fmt.Errorf("Failed to send initial event")
	}
	//successfully connected
	c.connected = true
	c.waiter.Add(1)
	//while connected, ping loop (every 25s, browser timesout after 30s)
	go func() {
		for {
			select {
			case <-time.After(c.state.PingInterval):
				if err := c.send(&update{Ping: true}); err != nil {
					goto disconnected
				}
			case <-c.connectedCh:
				goto disconnected
			}
		}
	disconnected:
		c.connected = false
		c.Close()
		//unblock waiters
		c.waiter.Done()
	}()
	//non-blocking wait on connection
	go func() {
		if err := c.transport.wait(); err != nil {
			//log error?
		}
		close(c.connectedCh)
	}()
	//now connected, consumer can connection.Wait()
	return nil
}

func (c *conn) push() {
	//attempt to mark state as 'pushing'
	if !atomic.CompareAndSwapUint32(&c.pushing, 0, 1) {
		//if already pushing, mark queued
		atomic.StoreUint32(&c.queued, 1)
		return
	}
	defer func() {
		//no longer pushing
		atomic.StoreUint32(&c.pushing, 0)
		//queued? dequeue and push again
		if atomic.CompareAndSwapUint32(&c.queued, 1, 0) {
			c.push() //within same goroutine
		}
	}()
	//current state data
	d := &c.state.data
	d.mut.RLock()
	if c.version == d.version {
		d.mut.RUnlock()
		//already have this version
		return
	}
	update := &update{Version: d.version}
	//first push? include id
	if atomic.CompareAndSwapUint32(&c.first, 0, 1) {
		update.ID = d.id
	}
	//choose optimal update (send the smallest)
	if d.delta != nil &&
		c.version == (d.version-1) &&
		len(d.bytes) > 0 &&
		len(d.delta) < len(d.bytes) {
		update.Delta = true
		update.Body = d.delta
	} else {
		update.Delta = false
		update.Body = d.bytes
	}
	d.mut.RUnlock()
	//unlock data and send!
	if err := c.send(update); err != nil {
		c.Close()
		return
	}
	//on success, relock and update client version
	d.mut.RLock()
	c.version = d.version
	d.mut.RUnlock()
}

//send to connection, ensure only 1 concurrent sender
func (c *conn) send(upd *update) error {
	c.sendingMut.Lock()
	defer c.sendingMut.Unlock()
	//send (transports responsiblity to enforce timeouts)
	return c.transport.send(upd)
}
