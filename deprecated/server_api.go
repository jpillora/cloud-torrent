package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/anacrolix/torrent/metainfo"
)

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) error {
	defer r.Body.Close()
	if r.Method != "POST" {
		return fmt.Errorf("Invalid request method (expecting POST)")
	}
	action := strings.TrimPrefix(r.URL.Path, "/api/")
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("Failed to download request body")
	}
	//convert url into torrent bytes
	if action == "url" {
		url := string(data)
		remote, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("Invalid remote torrent URL: %s (%s)", err, url)
		}
		data, err = ioutil.ReadAll(remote.Body)
		if err != nil {
			return fmt.Errorf("Failed to download remote torrent: %s", err)
		}
		action = "torrentfile"
	}
	//convert torrent bytes into magnet
	if action == "torrentfile" {
		reader := bytes.NewBuffer(data)
		info, err := metainfo.Load(reader)
		if err != nil {
			return err
		}
		data = []byte(info.Magnet().String())
		action = "magnet"
	}
	//update after action completes
	defer s.state.Push()
	//interface with engine
	switch action {
	case "configure":
		c := Config{}
		if err := json.Unmarshal(data, &c); err != nil {
			return err
		}
		if err := s.reconfigure(c); err != nil {
			return err
		}
	case "magnet":
		uri := string(data)
		if err := s.engine.NewByMagnet(uri); err != nil {
			return fmt.Errorf("Magnet error: %s", err)
		}
	case "torrentfile":
		if len(data) > 1*1024*1024 /*1MB*/ {
			return fmt.Errorf("Invalid torrent: too large")
		}
		if err := s.engine.NewByFile(bytes.NewBuffer(data)); err != nil {
			return fmt.Errorf("File error: %s", err)
		}
	case "torrent":
		command := torrentCommand{}
		if err := json.Unmarshal(data, &command); err != nil {
			return fmt.Errorf("Invalid command: %s", err)
		}
		return s.handleTorrentAPI(w, r, command)
	default:
		return fmt.Errorf("Invalid action: %s", action)
	}
	return nil
}

type torrentCommand struct {
	State    string `json:"state"`
	InfoHash string `json:"infohash"`
	File     *struct {
		Path      string `json:"path"`
		StorageID string `json:"storageId"`
		NewPath   string `json:"newPath"`
	} `json:"file"`
}

func (s *Server) handleTorrentAPI(w http.ResponseWriter, r *http.Request, command torrentCommand) error {
	if command.InfoHash == "" {
		return fmt.Errorf("Invalid command: missing infohash")
	}
	torrent, ok := s.engine.Get(command.InfoHash)
	if !ok {
		return fmt.Errorf("Invalid torrent")
	}
	if command.File == nil {
		switch command.State {
		case "start":
			return torrent.Start()
		case "stop":
			return torrent.Stop()
		case "remove":
			return s.engine.Remove(torrent)
		default:
			return fmt.Errorf("Invalid torrent state: %s", command.State)
		}
	}
	if command.File.Path == "" {
		return fmt.Errorf("Invalid file command: missing path")
	}
	file, ok := torrent.Get(command.File.Path)
	if !ok {
		return fmt.Errorf("Invalid file")
	}
	//override new path?
	if command.File.NewPath != "" {
		file.NewPath = command.File.NewPath
	}
	switch command.State {
	case "start":
		fs, ok := s.storage.Get(file.StorageID)
		if !ok {
			return fmt.Errorf("Invalid storage ID")
		}
		return file.Start(fs)
	case "stop":
		return file.Stop()
	default:
		return fmt.Errorf("Invalid file command: %s", command.State)
	}
}
