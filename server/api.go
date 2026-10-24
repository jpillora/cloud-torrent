package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jpillora/cloud-torrent/engine"
)

// maxRequestBytes caps API request bodies (a .torrent file is the largest
// payload expected).
const maxRequestBytes = 10 << 20 // 10 MiB

// API request bodies. These are exported so tygo generates matching TypeScript
// for the typed frontend client (the configure action uses engine.Config).

// URLRequest fetches a remote .torrent file by URL.
type URLRequest struct {
	URL string `json:"url"`
}

// MagnetRequest adds a torrent from a magnet URI.
type MagnetRequest struct {
	URI string `json:"uri"`
}

// TorrentRequest controls a torrent; Action is "start", "stop" or "delete".
type TorrentRequest struct {
	Action   string `json:"action"`
	InfoHash string `json:"infohash"`
}

// FileRequest controls a single file; Action is "start" or "stop".
type FileRequest struct {
	Action   string `json:"action"`
	InfoHash string `json:"infohash"`
	Path     string `json:"path"`
}

// handleAPI dispatches POST /api/<action>. On success the new state is pushed to
// clients via velox and the response body is a plain "OK"; failures return 400.
func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	if err := s.api(r); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRequestTooLarge) {
			status = http.StatusRequestEntityTooLarge
			w.Header().Set("Connection", "close")
		}
		if errors.Is(err, errRequestTimedOut) {
			status = http.StatusRequestTimeout
			w.Header().Set("Connection", "close")
		}
		http.Error(w, err.Error(), status)
		return
	}
	// Engine snapshots are detached; explicitly publish a fresh one after an
	// action rather than relying on shared pointers or waiting for the poll.
	torrents := s.engine.GetTorrents()
	s.state.Lock()
	s.state.Torrents = torrents
	s.state.Unlock()
	s.state.Push()
	w.Write([]byte("OK"))
}

func (s *Server) api(r *http.Request) error {
	if r.Method != http.MethodPost {
		return fmt.Errorf("expecting POST")
	}
	// net/http closes incoming bodies after the response. Closing an unread
	// body here can block error delivery while it drains a stalled upload.
	action := strings.TrimPrefix(r.URL.Path, "/api/")
	body, err := readAPIRequest(r)
	if err != nil {
		return err
	}

	switch action {
	case "configure":
		var c engine.Config
		if err := json.Unmarshal(body, &c); err != nil {
			return err
		}
		return s.reconfigure(c)
	case "url":
		var req URLRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		return s.addTorrentFromURL(r.Context(), req.URL)
	case "torrentfile":
		return s.addTorrentFromBytes(body)
	case "magnet":
		var req MagnetRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		return s.engine.NewMagnet(req.URI)
	case "torrent":
		var req TorrentRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		switch req.Action {
		case "start":
			return s.engine.StartTorrent(req.InfoHash)
		case "resume":
			return s.engine.ResumeTorrent(req.InfoHash)
		case "resume-seeding":
			return s.engine.ResumeSeeding(req.InfoHash)
		case "stop":
			return s.engine.StopTorrent(req.InfoHash)
		case "delete":
			return s.engine.DeleteTorrent(req.InfoHash)
		default:
			return fmt.Errorf("invalid torrent action: %q", req.Action)
		}
	case "file":
		var req FileRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return err
		}
		switch req.Action {
		case "start":
			return s.engine.StartFile(req.InfoHash, req.Path)
		case "stop":
			return s.engine.StopFile(req.InfoHash, req.Path)
		default:
			return fmt.Errorf("invalid file action: %q", req.Action)
		}
	default:
		return fmt.Errorf("unknown action: %q", action)
	}
}

func (s *Server) addTorrentFromURL(ctx context.Context, url string) error {
	body, err := s.fetchBytes(ctx, url, maxRequestBytes)
	if err != nil {
		return fmt.Errorf("fetch torrent: %w", err)
	}
	return s.addTorrentFromBytes(body)
}

func (s *Server) addTorrentFromBytes(b []byte) error {
	spec, err := parseTorrentBytes(b)
	if err != nil {
		return err
	}
	return s.engine.NewTorrent(spec)
}
