package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/boypt/simple-torrent/common"
	"github.com/boypt/simple-torrent/engine"
)

var (
	errInvalidReq = errors.New("INVALID REQUEST")
	errUnknowAct  = errors.New("UNKOWN ACTION")
	errUnknowPath = errors.New("UNKOWN PATH")
)

func (s *Server) apiGET(w http.ResponseWriter, r *http.Request) error {

	defer r.Body.Close()
	routeDirs := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
	if len(routeDirs) == 0 {
		return errUnknowAct
	}

	w.Header().Set("Content-Type", "application/json")
	action := routeDirs[0]
	switch action {
	case "magnet": // adds magnet by GET: /api/magnet?m=...
		tdata := struct {
			HasError bool
			Error    string
			Magnet   string
		}{}

		m := r.URL.Query().Get("m")
		if err := s.engine.NewMagnet(m); err != nil {
			if !errors.Is(err, engine.ErrMaxConnTasks) {
				tdata.HasError = true
				tdata.Error = err.Error()
			}
		}
		tdata.Magnet = m
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		common.HandleError(htmlTPL["magadded.html"].Execute(w, tdata))
	case "configure":
		common.HandleError(json.NewEncoder(w).Encode(*(s.engineConfig)))
	case "torrents":
		common.HandleError(json.NewEncoder(w).Encode(s.engine.GetTorrents()))
	case "files":
		common.HandleError(json.NewEncoder(w).Encode(s.listFiles()))
	case "torrent":
		if len(routeDirs) != 2 {
			return errUnknowAct
		}
		hash := routeDirs[1]
		if len(hash) != 40 {
			return errUnknowPath
		}
		m := s.engine.GetTorrents()
		if t, ok := (*m)[hash]; ok {
			common.HandleError(json.NewEncoder(w).Encode(t))
		} else {
			return errUnknowPath
		}
	case "stat":
		s.state.Stats.System.loadStats()
		s.state.Stats.ConnStat = s.engine.ConnStat()
		common.HandleError(json.NewEncoder(w).Encode(s.state.Stats))
	case "searchproviders":
		common.HandleError(json.NewEncoder(w).Encode(s.searchProviders))
	case "enginedebug":
		w.Header().Set("Content-Type", "application/json")
		var buf bytes.Buffer
		s.engine.WriteStauts(bufio.NewWriter(&buf))
		common.HandleError(json.NewEncoder(w).Encode(struct {
			EngineStatus string
			Trackers     []string
		}{buf.String(), s.engine.Trackers}))
	default:
		return errUnknowAct
	}

	return nil
}

func (s *Server) apiPOST(r *http.Request) error {
	defer r.Body.Close()

	action := strings.TrimPrefix(r.URL.Path, "/api/")
	if r.Method != "POST" {
		return fmt.Errorf("ERROR: Invalid request method (expecting POST)")
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("ERROR: Failed to download request body: %w", err)
	}

	//convert url into torrent bytes
	if action == "url" {
		url := string(data)
		remote, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("ERROR: Invalid remote torrent URL: %s %w", url, err)
		}
		defer remote.Body.Close()
		if remote.ContentLength > 512*1024 {
			//enforce max body size (512k)
			return fmt.Errorf("ERROR: Remote torrent too large")
		}
		data, err = ioutil.ReadAll(remote.Body)
		if err != nil {
			return fmt.Errorf("ERROR: Failed to download remote torrent: %w", err)
		}
		action = "torrentfile"
	}

	//convert torrent bytes into magnet
	if action == "torrentfile" {
		if err := s.engine.NewTorrentByReader(bytes.NewBuffer(data)); err != nil {
			if !errors.Is(err, engine.ErrMaxConnTasks) {
				return err
			}
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
			if errors.Is(err, engine.ErrMaxConnTasks) {
				return nil
			}
			return fmt.Errorf("ERROR: Magnet error: %w", err)
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
			if err := s.engine.ManualStartTorrent(infohash); err != nil {
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
			s.engine.RemoveCache(infohash)
		case "move2wait":
			if err := s.engine.DeleteTorrent(infohash); err != nil {
				return err
			}
			if err := s.engine.PushWaitTask(infohash); err != nil {
				return err
			}
		default:
			return fmt.Errorf("ERROR: Invalid state: %s", state)
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
			return fmt.Errorf("ERROR: Invalid state: %s", state)
		}
	default:
		return fmt.Errorf("ERROR: Invalid action: %s", action)
	}
	return nil
}

func (s *Server) apiConfigure(data []byte) error {

	if !s.engineConfig.AllowRuntimeConfigure {
		return errors.New("AllowRuntimeConfigure is set to false")
	}

	c := engine.Config{}
	if err := json.Unmarshal(data, &c); err != nil {
		return err
	}
	if _, err := c.NormlizeConfigDir(); err != nil {
		return err
	}

	if !reflect.DeepEqual(s.engineConfig, c) {
		status := s.engineConfig.Validate(&c)

		if status&engine.ForbidRuntimeChange > 0 {
			log.Printf("[api] warnning! someone tried to change DoneCmd config")
			return errors.New("ERROR: This item is NOT allowed being changed on runtime")
		}
		if status&engine.NeedRestartWatch > 0 {
			common.FancyHandleError(s.engine.StartTorrentWatcher())
			log.Printf("[api] file watcher restartd")
		}

		// now it's safe to save the configure
		s.engineConfig.SyncViper(c)
		s.engineConfig = &c
		if err := s.engineConfig.WriteDefault(); err != nil {
			return err
		}
		log.Printf("[api] config saved")

		// finally to reconfigure the engine
		if status&engine.NeedEngineReConfig > 0 {
			if err := s.engine.Configure(s.engineConfig); err != nil {
				if !s.engine.IsConfigred() {
					go func() {
						log.Println("[apiConfigure] serious error occured while reconfigured, will exit in 10s")
						time.Sleep(time.Second * 10)
						log.Fatalln(err)
					}()
				}
				return err
			}
			s.engine.RestoreCacheDir()
			log.Printf("[api] torrent engine reconfigred")
		} else {
			s.engine.SetConfig(s.engineConfig)
		}

		if status&engine.NeedUpdateTracker > 0 {
			go s.engine.ParseTrackerList() // nolint: errcheck
		}
		if status&engine.NeedUpdateRSS > 0 {
			go s.updateRSS()
		}
		s.state.Push()

		// do after config synced
		s.state.UseQueue = (s.engineConfig.MaxConcurrentTask > 0)
		if status&engine.NeedLoadWaitList > 0 {
			go func() {
				for {
					if err := s.engine.NextWaitTask(); err != nil {
						if errors.Is(err, engine.ErrMaxConnTasks) || errors.Is(err, engine.ErrWaitListEmpty) {
							break
						}
					}
				}
			}()
		}
	} else {
		log.Printf("[api] configure unchanged")
	}

	// update search config anyway
	go s.fetchSearchConfig(s.engineConfig.ScraperURL) // nolint: errcheck
	return nil
}

func (s *Server) GetStrAttribute(name string) string {
	cval := reflect.Indirect(reflect.ValueOf(s)).FieldByName(name)
	return cval.String()
}

func (s *Server) GetBoolAttribute(name string) bool {
	cval := reflect.Indirect(reflect.ValueOf(s)).FieldByName(name)
	return cval.Bool()
}
