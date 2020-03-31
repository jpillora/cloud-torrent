package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/jpillora/cloud-torrent/engine"
	ctstatic "github.com/jpillora/cloud-torrent/static"
)

var (
	errInvalidReq = errors.New("INVALID REQUEST")
	errUnknowAct  = errors.New("Unkown Action")
	errUnknowPath = errors.New("Unkown Path")
)

func (s *Server) apiGET(w http.ResponseWriter, r *http.Request) error {

	defer r.Body.Close()
	routeDirs := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	if len(routeDirs) == 0 {
		return errUnknowAct
	}

	action := routeDirs[0]
	switch action {
	case "magnet": // adds magnet by GET: /api/magnet?m=...
		c, err := ctstatic.ReadAll("template/magadded.html")
		if err != nil {
			log.Fatalln(err)
		}

		tmpl := template.Must(template.New("tpl").Parse(string(c)))
		tdata := struct {
			HasError bool
			Error    string
			Magnet   string
		}{}

		m := r.URL.Query().Get("m")
		if err := s.engine.NewMagnet(m); err != nil {
			tdata.HasError = true
			tdata.Error = err.Error()
		}

		tdata.Magnet = m
		tmpl.Execute(w, tdata)
	case "torrents":
		json.NewEncoder(w).Encode(s.engine.GetTorrents())
	case "files":
		s.state.Lock()
		json.NewEncoder(w).Encode(s.state.Downloads)
		s.state.Unlock()
	case "torrent":
		if len(routeDirs) != 2 {
			return errUnknowAct
		}
		hash := routeDirs[1]
		if len(hash) != 40 {
			return errUnknowPath
		}
		m := s.engine.GetTorrents()
		if t, ok := m[hash]; ok {
			json.NewEncoder(w).Encode(t)
		} else {
			return errUnknowPath
		}
	case "stat":
		s.state.Lock()
		c := s.engine.Config()
		s.state.Stats.System.loadStats(c.DownloadDirectory)
		json.NewEncoder(w).Encode(s.state.Stats)
		s.state.Unlock()
	case "enginedebug":
		w.Header().Set("Content-Type", "text/plain")
		s.engine.WriteStauts(w)
	default:
		return errUnknowAct
	}

	return nil
}

func (s *Server) apiPOST(r *http.Request) error {
	defer r.Body.Close()

	action := strings.TrimPrefix(r.URL.Path, "/api/")
	if r.Method != "POST" {
		return fmt.Errorf("Invalid request method (expecting POST)")
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("Failed to download request body: %w", err)
	}

	//convert url into torrent bytes
	if action == "url" {
		url := string(data)
		remote, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("Invalid remote torrent URL: %s %w", url, err)
		}
		//TODO enforce max body size (32k?)
		data, err = ioutil.ReadAll(remote.Body)
		defer remote.Body.Close()
		if err != nil {
			return fmt.Errorf("Failed to download remote torrent: %w", err)
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
		if err := s.engine.NewTorrentBySpec(spec); err != nil {
			return fmt.Errorf("Torrent error: %w", err)
		}
		return nil
	}

	//update after action completes
	defer s.state.Push()

	//interface with engine
	switch action {
	case "configure":
		return s.apiConfigure(data)
	case "magnet":
		if err := s.engine.NewMagnet(string(data)); err != nil {
			return fmt.Errorf("Magnet error: %w", err)
		}
	case "torrent":
		cmd := strings.SplitN(string(data), ":", 2)
		if len(cmd) != 2 {
			return errInvalidReq
		}
		state := cmd[0]
		infohash := cmd[1]
		switch state {
		case "start":
			if err := s.engine.StartTorrent(infohash); err != nil {
				return err
			}
		case "stop":
			if err := s.engine.StopTorrent(infohash); err != nil {
				return err
			}
		case "delete":
			if err := s.engine.DeleteTorrent(infohash); err != nil {
				return err
			}
		default:
			return fmt.Errorf("Invalid state: %s", state)
		}
	case "file":
		cmd := strings.SplitN(string(data), ":", 3)
		if len(cmd) != 3 {
			return errInvalidReq
		}
		state := cmd[0]
		infohash := cmd[1]
		filepath := cmd[2]
		switch state {
		case "start":
			if err := s.engine.StartFile(infohash, filepath); err != nil {
				return err
			}
		case "stop":
			if err := s.engine.StopFile(infohash, filepath); err != nil {
				return err
			}
		default:
			return fmt.Errorf("Invalid state: %s", state)
		}
	default:
		return fmt.Errorf("Invalid action: %s", action)
	}
	return nil
}

func (s *Server) apiConfigure(data []byte) error {

	c := engine.Config{}
	if err := json.Unmarshal(data, &c); err != nil {
		return err
	}
	if _, err := c.NormlizeConfigDir(); err != nil {
		return err
	}

	if !reflect.DeepEqual(s.state.Config, c) {
		status := s.state.Config.Validate(&c)

		if status&engine.ForbidRuntimeChange > 0 {
			log.Printf("[api] warnning! someone tried to change DoneCmd config")
			return errors.New("Nice Try! But this is NOT allowed being changed on runtime")
		}
		if status&engine.NeedRestartWatch > 0 {
			s.torrentWatcher()
			log.Printf("[api] file watcher restartd")
		}
		if status&engine.NeedUpdateTracker > 0 {
			go s.engine.UpdateTrackers()
		}

		// now it's safe to save the configure
		if err := s.state.Config.SyncViper(c); err != nil {
			return err
		}
		s.state.Config = c
		log.Printf("[api] config saved")

		// finally to reconfigure the engine
		if status&engine.NeedEngineReConfig > 0 {
			if err := s.engine.Configure(&s.state.Config); err != nil {
				if !s.engine.IsConfigred() {
					go func() {
						log.Println("[apiConfigure] serious error occured while reconfigured, will exit in 10s")
						time.Sleep(time.Second * 10)
						log.Fatalln(err)
					}()
				}
				return err
			}
			if err := s.RestoreTorrent(); err != nil {
				return err
			}
			log.Printf("[api] torrent engine reconfigred")
		} else {
			s.engine.SetConfig(s.state.Config)
		}
		s.state.Push()
	} else {
		log.Printf("[api] configure unchanged")
	}

	// update search config anyway
	go s.fetchSearchConfig(s.state.Config.ScraperURL)
	return nil
}
