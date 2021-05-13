package server

import (
	"log"
	"os"
	"time"
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
				s.state.Lock()
				c := s.engine.Config()
				s.state.Stats.System.loadStats(c.DownloadDirectory)
				s.state.Stats.ConnStat = s.engine.ConnStat()
				s.state.Unlock()
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
	s.engine.StartTorrentWatcher()
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
