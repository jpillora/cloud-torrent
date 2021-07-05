package server

import (
	"strings"
	"time"
)

func (s *Server) backgroundRoutines() {

	go s.fetchSearchConfig(s.state.Config.ScraperURL)
	s.stateRoutines()

	// rss updater
	go func() {
		// skip if not configured
		if !strings.HasPrefix(s.state.Config.RssURL, "http") {
			return
		}

		for range time.Tick(30 * time.Minute) {
			s.updateRSS()
		}
	}()

	go s.engine.RestoreCacheDir()

	if err := s.engine.StartTorrentWatcher(); err != nil {
		log.Println(err)
	}
}

// stateRoutines watches the tasks / sys states
func (s *Server) stateRoutines() {

	// initial state
	s.state.Torrents = s.engine.GetTorrents()
	s.state.Stats.System.loadStats()

	//collecting sys stats
	go func() {
		tk := time.NewTicker(10 * time.Second)
		defer tk.Stop()

		for {

		LRESELECT:
			select {
			case <-tk.C:
				if s.state.NumConnections() == 0 {
					goto LRESELECT
				}

				s.state.Stats.System.loadStats()
				s.state.Torrents = s.engine.GetTorrents()
				s.state.Stats.ConnStat = s.engine.ConnStat()
			case <-s.engine.TsChanged: // task added/deleted
				s.state.Torrents = s.engine.GetTorrents()
			}

			s.state.Push()
		}
	}()
}
