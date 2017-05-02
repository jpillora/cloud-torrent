package httptoo

import (
	"io"
	"net/http"
	"sync"

	"github.com/anacrolix/missinggo"
)

type responseWriter struct {
	mu            sync.Mutex
	r             http.Response
	headerWritten missinggo.Event
	bodyWriter    io.WriteCloser
	closed        missinggo.SynchronizedEvent
}

var _ interface {
	http.ResponseWriter
	http.CloseNotifier
} = &responseWriter{}

func (me *responseWriter) CloseNotify() <-chan bool {
	ret := make(chan bool, 1)
	go func() {
		<-me.closed.C()
		ret <- true
	}()
	return ret
}

func (me *responseWriter) Header() http.Header {
	if me.r.Header == nil {
		me.r.Header = make(http.Header)
	}
	return me.r.Header
}

func (me *responseWriter) Write(b []byte) (int, error) {
	me.mu.Lock()
	if !me.headerWritten.IsSet() {
		me.writeHeader(200)
	}
	me.mu.Unlock()
	return me.bodyWriter.Write(b)
}

func (me *responseWriter) WriteHeader(status int) {
	me.mu.Lock()
	me.writeHeader(status)
	me.mu.Unlock()
}

func (me *responseWriter) writeHeader(status int) {
	if me.headerWritten.IsSet() {
		return
	}
	me.r.StatusCode = status
	me.headerWritten.Set()
}

func (me *responseWriter) runHandler(h http.Handler, req *http.Request) {
	var pr *io.PipeReader
	pr, me.bodyWriter = io.Pipe()
	me.r.Body = struct {
		io.Reader
		io.Closer
	}{pr, eventCloser{pr, &me.closed}}
	defer me.bodyWriter.Close()
	defer me.WriteHeader(200)
	h.ServeHTTP(me, req)
}

type eventCloser struct {
	c      io.Closer
	closed *missinggo.SynchronizedEvent
}

func (me eventCloser) Close() (err error) {
	err = me.c.Close()
	me.closed.Set()
	return
}

func RoundTripHandler(req *http.Request, h http.Handler) (*http.Response, error) {
	rw := responseWriter{}
	go rw.runHandler(h, req)
	<-rw.headerWritten.LockedChan(&rw.mu)
	return &rw.r, nil
}

type InProcRoundTripper struct {
	Handler http.Handler
}

func (me *InProcRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return RoundTripHandler(req, me.Handler)
}
