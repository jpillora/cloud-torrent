package ct

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/jpillora/cloud-torrent/ct/shared"
)

type torrents map[string]*shared.Torrent

type request struct {
	Engine string
	URL    string
}

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

	eid := engineID(m[1])
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
		data, err = ioutil.ReadAll(remote.Body)
		if err != nil {
			return fmt.Errorf("Failed to download remote torrent: %s", err)
		}
		action = "torrent"
	}

	//convert torrent bytes into magnet
	if action == "torrent" {
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
	case "magnet":
		uri := string(data)
		if err := e.Magnet(uri); err != nil {
			return fmt.Errorf("Magnet error: %s", err)
		}
	case "list":
		torrents, err := e.List()
		if err != nil {
			return fmt.Errorf("List error: %s", err)
		}
		etorrents := s.state.Torrents[eid]
		for _, t := range torrents {
			etorrents[t.InfoHash] = t
		}
		s.rt.Update() //state change
	case "fetch":
		ih := string(data)
		t, ok := s.state.Torrents[eid][ih]
		if !ok {
			return fmt.Errorf("Invalid torrent: %s", ih)
		}
		if err := e.Fetch(t); err != nil {
			return fmt.Errorf("Fetch error: %s", err)
		}
	default:
		return fmt.Errorf("Invalid action: %s", action)
	}
	return nil
}
