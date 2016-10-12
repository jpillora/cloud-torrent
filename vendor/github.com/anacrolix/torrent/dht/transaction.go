package dht

import (
	"sync"
	"time"

	"github.com/anacrolix/torrent/dht/krpc"
)

// Transaction keeps track of a message exchange between nodes, such as a
// query message and a response message.
type Transaction struct {
	mu             sync.Mutex
	remoteAddr     Addr
	t              string
	response       chan krpc.Msg
	onResponse     func(krpc.Msg) // Called with the server locked.
	done           chan struct{}
	queryPacket    []byte
	timer          *time.Timer
	s              *Server
	retries        int
	lastSend       time.Time
	userOnResponse func(krpc.Msg, bool)
}

// SetResponseHandler sets up a function to be called when the query response
// is available.
func (t *Transaction) SetResponseHandler(f func(krpc.Msg, bool)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.userOnResponse = f
	t.tryHandleResponse()
}

func (t *Transaction) tryHandleResponse() {
	if t.userOnResponse == nil {
		return
	}
	select {
	case r, ok := <-t.response:
		t.userOnResponse(r, ok)
		// Shouldn't be called more than once.
		t.userOnResponse = nil
	default:
	}
}

func (t *Transaction) key() transactionKey {
	return transactionKey{
		t.remoteAddr.String(),
		t.t,
	}
}

func (t *Transaction) startTimer() {
	t.timer = time.AfterFunc(jitterDuration(queryResendEvery, time.Second), t.timerCallback)
}

func (t *Transaction) timerCallback() {
	t.mu.Lock()
	defer t.mu.Unlock()
	select {
	case <-t.done:
		return
	default:
	}
	if t.retries == 2 {
		t.timeout()
		return
	}
	t.retries++
	t.sendQuery()
	if t.timer.Reset(jitterDuration(queryResendEvery, time.Second)) {
		panic("timer should have fired to get here")
	}
}

func (t *Transaction) sendQuery() error {
	err := t.s.writeToNode(t.queryPacket, t.remoteAddr)
	if err != nil {
		return err
	}
	t.lastSend = time.Now()
	return nil
}

func (t *Transaction) timeout() {
	go func() {
		t.s.mu.Lock()
		defer t.s.mu.Unlock()
		t.s.nodeTimedOut(t.remoteAddr)
	}()
	t.close()
}

func (t *Transaction) close() {
	if t.closing() {
		return
	}
	t.queryPacket = nil
	close(t.response)
	t.tryHandleResponse()
	close(t.done)
	t.timer.Stop()
	go func() {
		t.s.mu.Lock()
		defer t.s.mu.Unlock()
		t.s.deleteTransaction(t)
	}()
}

func (t *Transaction) closing() bool {
	select {
	case <-t.done:
		return true
	default:
		return false
	}
}

// Close (abandon) the transaction.
func (t *Transaction) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.close()
}

func (t *Transaction) handleResponse(m krpc.Msg) {
	t.mu.Lock()
	if t.closing() {
		t.mu.Unlock()
		return
	}
	close(t.done)
	t.mu.Unlock()
	if t.onResponse != nil {
		t.s.mu.Lock()
		t.onResponse(m)
		t.s.mu.Unlock()
	}
	t.queryPacket = nil
	select {
	case t.response <- m:
	default:
		panic("blocked handling response")
	}
	close(t.response)
	t.tryHandleResponse()
}
