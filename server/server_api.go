package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"strings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/jpillora/cloud-torrent/engine"
)

var errTaskAdded = errors.New("REDIRECT TORRENT HOME")

func (s *Server) api(r *http.Request) error {
	defer r.Body.Close()

	action := strings.TrimPrefix(r.URL.Path, "/api/")

	if r.Method == "GET" {
		// adds magnet by GET: /api/magnet?m=...
		if action == "magnet" {
			m := r.URL.Query().Get("m")
			if strings.HasPrefix(m, "magnet:?") {
				if err := s.engine.NewMagnet(m); err != nil {
					return fmt.Errorf("Magnet error: %s", err)
				}
			} else {
				return fmt.Errorf("Invalid Magnet link: %s", m)
			}
			return errTaskAdded
		}

		return errors.New("Invalid path")
	}

	if r.Method != "POST" {
		return fmt.Errorf("Invalid request method (expecting POST)")
	}

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
	defer s.state.Push()

	//interface with engine
	switch action {
	case "configure":
		s.apiConfigure(data)
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

func (s *Server) apiConfigure(data []byte) error {

	// update search config anyway
	go s.fetchSearchConfig()

	c := engine.Config{}
	if err := json.Unmarshal(data, &c); err != nil {
		return err
	}
	if err := s.normlizeConfigDir(&c); err != nil {
		return err
	}

	if !reflect.DeepEqual(s.state.Config, c) {
		status := s.state.Config.Validate(&c)

		if status&engine.ForbidRuntimeChange > 0 {
			log.Printf("[api] warnning! someone tried to change DoneCmd config")
			return errors.New("Nice Try! But this is NOT allowed being changed on runtime")
		}
		if status&engine.NeedRestartWatch > 0 {
			s.TorrentWatcher()
			log.Printf("[api] file watcher restartd")
		}
		if status&engine.NeedUpdateTracker > 0 {
			go s.engine.UpdateTrackers()
		}

		// all Torrent must be STOPPED to reconfigure engine
		if status&engine.NeedEngineReConfig > 0 {
			ts := s.engine.GetTorrents()
			for _, tt := range ts {
				if tt.Started {
					return errors.New("All Torrent must be STOPPED to reconfigure")
				}
			}
		}

		// now it's safe to save the configure
		log.Printf("[api] config saved")
		s.state.Config = c
		if err := s.state.Config.SaveConfigFile(s.ConfigPath); err != nil {
			return err
		}

		// finally to reconfigure the engine
		if status&engine.NeedEngineReConfig > 0 {
			if err := s.engine.Configure(s.state.Config); err != nil {
				return err
			}
			log.Printf("[api] torrent engine reconfigred")
		}

		s.state.Push()
	} else {
		log.Printf("[api] configure unchanged")
	}
	return nil
}
