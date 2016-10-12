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
	me.r.Body, me.bodyWriter = io.Pipe()
	defer me.bodyWriter.Close()
	defer me.WriteHeader(200)
	h.ServeHTTP(me, req)
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
