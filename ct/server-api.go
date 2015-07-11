package ct

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/jpillora/cloud-torrent/ct/engines"
	"github.com/jpillora/cloud-torrent/ct/shared"
)

type torrents map[string]*shared.Torrent

type request struct {
	Engine string
	URL    string
}

//path matcher ->                        engine id/action type
var pathRe = regexp.MustCompile(`^\/api\/([a-z]+)\/([a-z]+)$`)

func (s *Server) api(r *http.Request) error {

	defer r.Body.Close()
	if r.Method != "POST" {
		return fmt.Errorf("Invalid request method (expecting POST)")
	}

	m := pathRe.FindStringSubmatch(r.URL.Path)
	if len(m) == 0 {
		return fmt.Errorf("Invalid request URL")
	}

	eid := engine.ID(m[1])
	e, ok := s.engines[eid]
	if !ok {
		return fmt.Errorf("Invalid engine ID: %s", eid)
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("Failed to download request body")
	}

	action := m[2]

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
		action = "torrent"
	}

	//convert torrent bytes into magnet
	if action == "torrentfile" {
		reader := bytes.NewBuffer(data)
		info, err := metainfo.Load(reader)
		if err != nil {
			return err
		}
		ts := torrent.TorrentSpecFromMetaInfo(info)
		trackers := []string{}
		for _, tier := range ts.Trackers {
			for _, t := range tier {
				trackers = append(trackers, t)
			}
		}
		m := torrent.Magnet{
			InfoHash:    ts.InfoHash,
			Trackers:    trackers,
			DisplayName: ts.DisplayName,
		}
		data = []byte(m.String())
		action = "magnet"
	}

	//interface with engine
	switch action {
	case "configure":
		if err := s.loadConfig(data); err != nil {
			return fmt.Errorf("Configure error: %s", err)
		}
	case "magnet":
		uri := string(data)
		if err := e.NewTorrent(uri); err != nil {
			return fmt.Errorf("Magnet error: %s", err)
		}
	case "torrent":
		cmd := strings.SplitN(string(data), ":", 2)
		if len(cmd) != 2 {
			return fmt.Errorf("Invalid request")
		}
		state := cmd[0]
		infohash := cmd[1]
		log.Printf("torrent api: %s -> %s", state, infohash)
		if state == "start" {
			if err := e.StartTorrent(infohash); err != nil {
				return err
			}
		} else if state == "stop" {
			if err := e.StopTorrent(infohash); err != nil {
				return err
			}
		} else if state == "delete" {
			if err := e.DeleteTorrent(infohash); err != nil {
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
			if err := e.StartFile(infohash, filepath); err != nil {
				return err
			}
		} else if state == "stop" {
			if err := e.StopFile(infohash, filepath); err != nil {
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
