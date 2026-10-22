package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTorrentURLFetchTimeoutAndCancellation(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		started, stopped := make(chan struct{}), make(chan struct{})
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			close(started)
			<-r.Context().Done()
			close(stopped)
		}))
		s := &Server{fetchClient: &http.Client{Timeout: 150 * time.Millisecond}}
		ctx, cancel := context.WithCancel(context.Background())
		body, _ := json.Marshal(URLRequest{URL: origin.URL})
		r := httptest.NewRequest(http.MethodPost, "/api/url", strings.NewReader(string(body))).WithContext(ctx)
		done := make(chan error, 1)
		go func() { done <- s.api(r) }()
		<-started
		if cancelRequest {
			cancel()
		}
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("stalled origin unexpectedly succeeded")
			}
			if cancelRequest && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("stalled fetch did not finish")
		}
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("upstream request retained after cancellation/timeout")
		}
		cancel()
		origin.Close()
	}
}

func TestFetchSizeLimitAndStatus(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		io.WriteString(w, strings.Repeat("x", 1025))
	}))
	defer origin.Close()
	s := &Server{}
	if _, err := s.fetchBytes(context.Background(), origin.URL, 1024); err == nil || !strings.Contains(err.Error(), "exceeds 1024") {
		t.Fatalf("oversized response: %v", err)
	}
	if _, err := s.fetchBytes(context.Background(), origin.URL+"/status", 1024); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("bad status: %v", err)
	}
}

func TestProviderConfigFetchIsBounded(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if oversized {
				io.WriteString(w, strings.Repeat("x", (1<<20)+1))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}))
		s := &Server{fetchClient: &http.Client{Timeout: 150 * time.Millisecond, Transport: searchRoundTrip(func(r *http.Request) (*http.Response, error) {
			redirect, err := http.NewRequestWithContext(r.Context(), http.MethodGet, origin.URL, nil)
			if err != nil {
				return nil, err
			}
			return http.DefaultTransport.RoundTrip(redirect)
		})}}
		err := s.fetchSearchConfig()
		origin.Close()
		if err == nil {
			t.Fatal("unbounded provider response succeeded")
		}
		if oversized && !strings.Contains(err.Error(), "exceeds 1048576") {
			t.Fatalf("provider oversize error: %v", err)
		}
	}
}
