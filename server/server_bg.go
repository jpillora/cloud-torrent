package server

import (
	"strings"
	"sync/atomic"
	"time"
)

func (s *Server) backgroundRoutines() {

	go s.fetchSearchConfig(s.engineConfig.ScraperURL) // nolint: errcheck

	// initial state
	s.state.Stats.System.loadStats()
	//collecting sys stats
	go func() {
		for {
			select {
			case <-s.syncConnected:
				if atomic.CompareAndSwapInt32(&(s.syncSemphor), 0, 1) {
					go s.tickerRoutine()
				}
			case <-s.engine.TsChanged: // task added/deleted
				s.engine.RLock()
				s.state.Push()
				s.engine.RUnlock()
			}
		}
	}()

	// rss updater
	go func() {
		// skip if not configured
		if strings.TrimSpace(s.engineConfig.RssURL) == "" {
			return
		}

		s.updateRSS()
		tk := time.NewTicker(30 * time.Minute)
		defer tk.Stop()
		for range tk.C {
			s.updateRSS()
		}
	}()

	go s.engine.RestoreCacheDir()
	if err := s.engine.StartTorrentWatcher(); err != nil {
		log.Println(err)
	}
}

// stateRoutines watches the tasks / sys states
func (s *Server) tickerRoutine() {
	defer atomic.StoreInt32(&(s.syncSemphor), 0)

	tick := time.Duration(s.IntevalSec) * time.Second
	log.Println("[tickerRoutine] sync connected, ticking for", tick)
	tk := time.NewTicker(tick)
	defer tk.Stop()

	done := make(chan struct{})
	go func() {
		s.syncWg.Wait()
		close(done)
	}()

	for {
		select {
		case <-tk.C:
			s.state.Stats.System.loadStats()
			s.state.Stats.ConnStat = s.engine.ConnStat()
			s.engine.RLock()
			s.state.Push()
			s.engine.RUnlock()
		case <-done:
			log.Println("[tickerRoutine] sync exit")
			return
		}
	}
}
