// Package eventsource provides the building blocks for consuming and building
// EventSource services.
package eventsource

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"
)

var (
	// ErrClosed signals that the event source has been closed and will not be
	// reopened.
	ErrClosed = errors.New("closed")

	// ErrInvalidEncoding is returned by Encoder and Decoder when invalid UTF-8
	// event data is encountered.
	ErrInvalidEncoding = errors.New("invalid UTF-8 sequence")
)

// An Event is a message can be written to an event stream and read from an
// event source.
type Event struct {
	Type    string
	ID      string
	Retry   string
	Data    []byte
	ResetID bool
}

// An EventSource consumes server sent events over HTTP with automatic
// recovery.
type EventSource struct {
	retry       time.Duration
	request     *http.Request
	err         error
	r           io.ReadCloser
	dec         *Decoder
	lastEventID string
}

// New prepares an EventSource. The connection is automatically managed, using
// req to connect, and retrying from recoverable errors after waiting the
// provided retry duration.
func New(req *http.Request, retry time.Duration) *EventSource {
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	return &EventSource{
		retry:   retry,
		request: req,
	}
}

// Close the source. Any further calls to Read() will return ErrClosed.
func (es *EventSource) Close() {
	if es.r != nil {
		es.r.Close()
	}
	es.err = ErrClosed
}

// Connect to an event source, validate the response, and gracefully handle
// reconnects.
func (es *EventSource) connect() {
	for es.err == nil {
		if es.r != nil {
			es.r.Close()
			<-time.After(es.retry)
		}

		es.request.Header.Set("Last-Event-Id", es.lastEventID)

		resp, err := http.DefaultClient.Do(es.request)

		if err != nil {
			continue // reconnect
		}

		if resp.StatusCode >= 500 {
			// assumed to be temporary, try reconnecting
			resp.Body.Close()
		} else if resp.StatusCode == 204 {
			resp.Body.Close()
			es.err = ErrClosed
		} else if resp.StatusCode != 200 {
			resp.Body.Close()
			es.err = fmt.Errorf("endpoint returned unrecoverable status %q", resp.Status)
		} else {
			mediatype, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))

			if mediatype != "text/event-stream" {
				resp.Body.Close()
				es.err = fmt.Errorf("invalid content type %q", resp.Header.Get("Content-Type"))
			} else {
				es.r = resp.Body
				es.dec = NewDecoder(es.r)
				return
			}
		}
	}
}

// Read an event from EventSource. If an error is returned, the EventSource
// will not reconnect, and any further call to Read() will return the same
// error.
func (es *EventSource) Read() (Event, error) {
	if es.r == nil {
		es.connect()
	}

	for es.err == nil {
		var e Event

		err := es.dec.Decode(&e)

		if err == ErrInvalidEncoding {
			continue
		}

		if err != nil {
			es.connect()
			continue
		}

		if len(e.Data) == 0 {
			continue
		}

		if len(e.ID) > 0 || e.ResetID {
			es.lastEventID = e.ID
		}

		if len(e.Retry) > 0 {
			if retry, err := strconv.Atoi(e.Retry); err == nil {
				es.retry = time.Duration(retry) * time.Millisecond
			}
		}

		return e, nil
	}

	return Event{}, es.err
}
