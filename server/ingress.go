package server

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type ingressLimits struct {
	header, idle, apiBody, uploadBody time.Duration
}

func (s *Server) ingressLimits() ingressLimits {
	limits := s.ingress
	if limits.header <= 0 {
		limits.header = 5 * time.Second
	}
	if limits.idle <= 0 {
		limits.idle = 60 * time.Second
	}
	if limits.apiBody <= 0 {
		limits.apiBody = 15 * time.Second
	}
	if limits.uploadBody <= 0 {
		limits.uploadBody = 60 * time.Second
	}
	return limits
}
func (s *Server) httpServer(addr string) *http.Server {
	limits := s.ingressLimits()
	return &http.Server{
		Addr: addr, Handler: s.handler(), ReadHeaderTimeout: limits.header, IdleTimeout: limits.idle,
		// Body deadlines are installed at ingress, and cleared upon full receipt.
		// No global ReadTimeout or WriteTimeout may truncate sync/file/ZIP streams.
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
	}
}

// ingressBody releases the read deadline only after a complete body. Unread or
// failed bodies keep their deadline through net/http's bounded drain/close.
type ingressBody struct {
	io.ReadCloser
	controller *http.ResponseController
	writer     http.ResponseWriter
	finished   bool
}

func (b *ingressBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if errors.Is(err, io.EOF) && !b.finished {
		b.finished = true
		_ = b.controller.SetReadDeadline(time.Time{})
	}
	if err != nil && !errors.Is(err, io.EOF) {
		b.writer.Header().Set("Connection", "close")
	}
	return n, err
}
func (s *Server) protectIngress(next http.Handler) http.Handler {
	limits := s.ingressLimits()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody {
			timeout := limits.apiBody
			if strings.TrimSuffix(r.URL.Path, "/") == "/api/torrentfile" {
				timeout = limits.uploadBody
			}
			controller := http.NewResponseController(w)
			// This wrapper sits outside compression/logging/auth so it can reach the
			// real net/http writer, including for unauthenticated stalled requests.
			if err := controller.SetReadDeadline(time.Now().Add(timeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
				w.Header().Set("Connection", "close")
				http.Error(w, "cannot protect request body", 503)
				return
			}
			r.Body = &ingressBody{ReadCloser: http.MaxBytesReader(w, r.Body, maxRequestBytes), controller: controller, writer: w}
		}
		next.ServeHTTP(w, r)
	})
}

var errRequestTooLarge = errors.New("request body exceeds 10 MiB limit")
var errRequestTimedOut = errors.New("request body read timed out")

func readAPIRequest(r *http.Request) ([]byte, error) {
	if r.ContentLength > maxRequestBytes {
		return nil, errRequestTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			return nil, errRequestTooLarge
		}
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			return nil, errRequestTimedOut
		}
		return nil, errors.New("failed to read request body")
	}
	if len(body) > maxRequestBytes {
		return nil, errRequestTooLarge
	}
	return body, nil
}
