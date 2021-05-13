package server

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/radovskyb/watcher"
)

func (s *Server) backgroundRoutines() {

	go s.fetchSearchConfig(s.state.Config.ScraperURL)

	//poll torrents and files
	go func() {
		// initial state
		s.state.Lock()
		s.state.Torrents = s.engine.GetTorrents()
		s.state.Downloads = s.listFiles()
		s.state.Unlock()

		for range time.Tick(time.Second) {
			if s.state.NumConnections() > 0 {
				// only update the state object if user connected
				s.state.Lock()
				s.state.Torrents = s.engine.GetTorrents()
				s.state.Downloads = s.listFiles()
				s.state.Unlock()
				s.state.Push()
			}
		}
	}()

	//start collecting stats
	go func() {
		for range time.Tick(5 * time.Second) {
			if s.state.NumConnections() > 0 {
				c := s.engine.Config()
				s.state.Stats.System.loadStats(c.DownloadDirectory)
				s.state.Stats.ConnStat = s.engine.ConnStat()
				s.state.Push()
			}
		}
	}()

	// rss updater
	go func() {
		for range time.Tick(30 * time.Minute) {
			s.updateRSS()
		}
	}()

	go s.engine.UpdateTrackers()
	go s.RestoreTorrent()
}

func (s *Server) RestoreTorrent() error {
	if w, err := os.Stat(s.state.Config.WatchDirectory); os.IsNotExist(err) || (err == nil && !w.IsDir()) {
		log.Printf("[Watcher] %s is not dir", s.state.Config.WatchDirectory)
		return err
	}

	s.engine.RestoreTorrent("*.torrent")
	s.engine.RestoreMagnet("*.info")
	return nil
}

func (s *Server) torrentWatcher() error {

	if s.watcher != nil {
		log.Print("Torrent Watcher: close")
		s.watcher.Close()
		s.watcher = nil
	}

	if w, err := os.Stat(s.state.Config.WatchDirectory); os.IsNotExist(err) || (err == nil && !w.IsDir()) {
		return fmt.Errorf("[Watcher] %s is not dir", s.state.Config.WatchDirectory)
	}

	log.Printf("Torrent Watcher: watching torrent file in %s", s.state.Config.WatchDirectory)
	w := watcher.New()
	w.SetMaxEvents(10)
	w.FilterOps(watcher.Create)

	go func() {
		for {
			select {
			case event := <-w.Event:
				if event.IsDir() {
					continue
				}
				// skip auto saved torrent
				if strings.HasPrefix(event.Name(), cacheSavedPrefix) {
					continue
				}
				if strings.HasSuffix(event.Name(), ".torrent") {
					if err := s.engine.NewTorrentByFilePath(event.Path); err == nil {
						log.Printf("Torrent Watcher: added %s, file removed\n", event.Name())
						os.Remove(event.Path)
					} else {
						log.Printf("Torrent Watcher: fail to add %s, ERR:%#v\n", event.Name(), err)
					}
				}
			case err := <-w.Error:
				log.Print(err)
			case <-w.Closed:
				return
			}
		}
	}()

	// Watch this folder for changes.
	if err := w.Add(s.state.Config.WatchDirectory); err != nil {
		return err
	}

	s.watcher = w
	go w.Start(time.Second * 5)
	return nil
}
