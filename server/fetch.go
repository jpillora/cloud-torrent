package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Timeout includes connection setup, redirects, headers and body consumption.
var defaultFetchClient = &http.Client{Timeout: 15 * time.Second}

func (s *Server) fetchBytes(ctx context.Context, url string, limit int64) ([]byte, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := s.fetchClient
	if client == nil {
		client = defaultFetchClient
	}
	resp, err := client.Do(r)
	if err != nil {
		return nil, fmt.Errorf("request %q: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request %q: HTTP %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", url, err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("response from %q exceeds %d bytes", url, limit)
	}
	return b, nil
}
