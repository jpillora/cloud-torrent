package eventsource

import (
	"mime"
	"net/http"
	"strings"
)

// Handler is an adapter for ordinary functions to act as an HTTP handler for
// event sources. It receives the ID of the last event processed by the client,
// and Encoder to deliver messages, and a channel to be notified if the client
// connection is closed.
type Handler func(lastId string, encoder *Encoder, stop <-chan bool)

func (h Handler) acceptable(accept string) bool {
	if accept == "" {
		// The absense of an Accept header is equivalent to "*/*".
		// https://tools.ietf.org/html/rfc2296#section-4.2.2
		return true
	}

	for _, a := range strings.Split(accept, ",") {
		mediatype, _, err := mime.ParseMediaType(a)
		if err != nil {
			continue
		}

		if mediatype == "text/event-stream" || mediatype == "text/*" || mediatype == "*/*" {
			return true
		}
	}

	return false
}

// ServeHTTP calls h with an Encoder and a close notification channel. It
// performs Content-Type negotiation.
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Vary", "Accept")

	if !h.acceptable(r.Header.Get("Accept")) {
		w.WriteHeader(http.StatusNotAcceptable)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)

	var stop <-chan bool

	if notifier, ok := w.(http.CloseNotifier); ok {
		stop = notifier.CloseNotify()
	}

	h(r.Header.Get("Last-Event-Id"), NewEncoder(w), stop)
}
