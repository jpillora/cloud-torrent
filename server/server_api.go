package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/jpillora/cloud-torrent/engine"
)

func (s *Server) api(r *http.Request) error {
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
		//TODO enforce max body size (32k?)
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
		spec := torrent.TorrentSpecFromMetaInfo(info)
		if err := s.engine.NewTorrent(spec); err != nil {
			return fmt.Errorf("Torrent error: %s", err)
		}
		return nil
	}

	//update after action completes
	defer s.state.Update()

	//interface with engine
	switch action {
	case "configure":
		c := engine.Config{}
		if err := json.Unmarshal(data, &c); err != nil {
			return err
		}
		if err := s.reconfigure(c); err != nil {
			return err
		}
	case "magnet":
		uri := string(data)
		if err := s.engine.NewMagnet(uri); err != nil {
			return fmt.Errorf("Magnet error: %s", err)
		}
	case "torrent":
		cmd := strings.SplitN(string(data), ":", 2)
		if len(cmd) != 2 {
			return fmt.Errorf("Invalid request")
		}
		state := cmd[0]
		infohash := cmd[1]
		if state == "start" {
			if err := s.engine.StartTorrent(infohash); err != nil {
				return err
			}
		} else if state == "stop" {
			if err := s.engine.StopTorrent(infohash); err != nil {
				return err
			}
		} else if state == "delete" {
			if err := s.engine.DeleteTorrent(infohash); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("Invalid state: %s", state)
		}
	case "file":
		cmd := strings.SplitN(string(data), ":", 3)
		if len(cmd) != 3 {
			return fmt.Errorf("Invalid request")
		}
		state := cmd[0]
		infohash := cmd[1]
		filepath := cmd[2]
		if state == "start" {
			if err := s.engine.StartFile(infohash, filepath); err != nil {
				return err
			}
		} else if state == "stop" {
			if err := s.engine.StopFile(infohash, filepath); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("Invalid state: %s", state)
		}
	default:
		return fmt.Errorf("Invalid action: %s", action)
	}
	return nil
}
