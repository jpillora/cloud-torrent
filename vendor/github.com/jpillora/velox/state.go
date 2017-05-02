package velox

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jpillora/jsonpatch"
)

//Pusher implements a push method,
//similar to Flush
type Pusher interface {
	Push() bool
}

var (
	//15ms is approximately highest resolution on the JS eventloop
	MinThrottle         = 15 * time.Millisecond
	DefaultThrottle     = 200 * time.Millisecond
	DefaultWriteTimeout = 30 * time.Second
	DefaultPingInterval = 25 * time.Second
)

//State must be embedded into a struct to make it syncable.
type State struct {
	//configuration
	Throttle     time.Duration `json:"-"`
	WriteTimeout time.Duration `json:"-"`
	PingInterval time.Duration `json:"-"`
	//internal state
	initMut  sync.Mutex
	initd    bool
	gostruct interface{}
	connMut  sync.Mutex
	conns    map[int64]*conn
	data     struct {
		mut     sync.RWMutex
		id      string //data id != conn id
		bytes   []byte
		delta   []byte
		version int64
	}
	push struct {
		mut    sync.Mutex
		ing    uint32
		queued uint32
	}
}

func (s *State) init(gostruct interface{}) error {
	if s.Throttle < MinThrottle {
		s.Throttle = DefaultThrottle
	}
	if s.WriteTimeout == 0 {
		s.WriteTimeout = DefaultWriteTimeout
	}
	if s.PingInterval == 0 {
		s.PingInterval = DefaultPingInterval
	}
	//get initial JSON bytes and confirm gostruct is marshallable
	l, ok := gostruct.(sync.Locker)
	if ok {
		l.Lock()
		defer l.Unlock()
	}
	if b, err := json.Marshal(gostruct); err != nil {
		return fmt.Errorf("JSON marshalling failed: %s", err)
	} else {
		s.data.bytes = b
	}
	id := make([]byte, 4)
	if n, _ := rand.Read(id); n > 0 {
		s.data.id = hex.EncodeToString(id)
	}
	s.gostruct = gostruct
	s.data.version = 1
	s.conns = map[int64]*conn{}
	s.initd = true
	return nil
}

//ID uniquely identifies this state object
func (s *State) ID() string {
	return s.data.id
}

//Version of this state object (when the underlying struct is
//and a Push is performed, this version number is incremented).
func (s *State) Version() int64 {
	return s.data.version
}

func (s *State) sync(gostruct interface{}) (*State, error) {
	s.initMut.Lock()
	defer s.initMut.Unlock()
	if !s.initd {
		if err := s.init(gostruct); err != nil {
			return nil, err
		}
	} else if s.gostruct != gostruct {
		return nil, errors.New("A different struct is already synced")
	}
	return s, nil
}

func (s *State) subscribe(conn *conn) {
	//subscribe
	conn.waiter.Add(1)
	s.connMut.Lock()
	s.conns[conn.id] = conn
	s.connMut.Unlock()
	//and then unsubscribe on close
	go func() {
		<-conn.connectedCh //this unblocks before wait
		s.connMut.Lock()
		delete(s.conns, conn.id)
		s.connMut.Unlock()
		conn.waiter.Done()
	}()
}

func (s *State) NumConnections() int {
	s.connMut.Lock()
	n := len(s.conns)
	s.connMut.Unlock()
	return n
}

//Send the changes from this object to all connected clients.
//Push is thread-safe and is throttled so it can be called
//with abandon. Returns false if a Push is already in progress.
func (s *State) Push() bool {
	//attempt to mark state as 'pushing'
	if atomic.CompareAndSwapUint32(&s.push.ing, 0, 1) {
		go s.gopush()
		return true
	}
	//if already pushing, mark queued
	atomic.StoreUint32(&s.push.queued, 1)
	return false
}

//non-blocking push
func (s *State) gopush() {
	s.push.mut.Lock()
	t0 := time.Now()
	//queue cleanup
	defer func() {
		//measure time passed, ensure we wait at least Throttle time
		tdelta := time.Now().Sub(t0)
		if t := s.Throttle - tdelta; t > 0 {
			time.Sleep(t)
		}
		//push complete
		s.push.mut.Unlock()
		atomic.StoreUint32(&s.push.ing, 0)
		//if queued, auto-push again
		if atomic.CompareAndSwapUint32(&s.push.queued, 1, 0) {
			s.Push()
		}
	}()
	//calculate new json state
	l, hasLock := s.gostruct.(sync.Locker)
	if hasLock {
		l.Lock()
	}
	newBytes, err := json.Marshal(s.gostruct)
	if hasLock {
		l.Unlock()
	}
	if err != nil {
		log.Printf("velox: marshal failed: %s", err)
		return
	}
	//if changed, then calculate change set
	if !bytes.Equal(s.data.bytes, newBytes) {
		//calculate change set from last version
		ops, _ := jsonpatch.CreatePatch(s.data.bytes, newBytes)
		if len(s.data.bytes) > 0 && len(ops) > 0 {
			//changes! bump version
			s.data.mut.Lock()
			s.data.delta, _ = json.Marshal(ops)
			s.data.bytes = newBytes
			s.data.version++
			s.data.mut.Unlock()
		}
	}
	//send this new change to each subscriber
	s.connMut.Lock()
	for _, c := range s.conns {
		if c.version != s.data.version {
			go c.push()
		}
	}
	s.connMut.Unlock()
	//defered cleanup()
}
