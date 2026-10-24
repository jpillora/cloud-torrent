package server

import (
	"context"
	"fmt"
	"github.com/jpillora/cloud-torrent/internal/outbound"
	"io"
	"net/http"
)

// Timeout includes connection setup, redirects, headers and body consumption.
var defaultPolicy, _ = outbound.New("")
var defaultFetchClient = defaultPolicy.Client()

func (s *Server) fetchBytes(ctx context.Context, url string, limit int64) ([]byte, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client, err := s.remoteClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(r)
	if err != nil {
		return nil, outbound.Failure{Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("remote response: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, outbound.Failure{Cause: err}
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("remote response exceeds %d bytes", limit)
	}
	return b, nil
}

func (s *Server) remoteClient() (*http.Client, error) {
	if s.fetchClient != nil {
		return s.fetchClient, nil
	}
	if s.OutboundAllowCIDRs == "" {
		return defaultFetchClient, nil
	}
	policy, err := outbound.New(s.OutboundAllowCIDRs)
	if err != nil {
		return nil, err
	}
	return policy.Client(), nil
}
