package server

import (
	"log"
	"time"

	"github.com/fsnotify/fsnotify"
)

func (s *Server) backgroundRoutines() {

	go s.fetchSearchConfig(s.state.Config.ScraperURL)
	s.stateRoutines()

	// rss updater
	go func() {
		for range time.Tick(30 * time.Minute) {
			s.updateRSS()
		}
	}()

	go s.engine.RestoreCacheDir()

	if err := s.engine.StartTorrentWatcher(); err != nil {
		log.Println("Bg", err)
	}
}

// stateRoutines watches the download dir / tasks / sys states
func (s *Server) stateRoutines() {
	dir := s.engine.Config().DownloadDirectory

	// initial state
	s.state.Lock()
	s.state.Torrents = s.engine.GetTorrents()
	s.state.Downloads = s.listFiles()
	s.state.Stats.System.loadStats(dir)
	s.state.Unlock()

	//collecting sys stats
	go func() {
		for range time.Tick(10 * time.Second) {
			if s.state.NumConnections() > 0 {
				s.state.Lock()
				s.state.Stats.System.loadStats(dir)
				s.state.Stats.ConnStat = s.engine.ConnStat()
				s.state.Unlock()
				s.state.Push()
			}
		}
	}()

	// download dir watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	if err := watcher.Add(s.state.Config.DownloadDirectory); err != nil {
		log.Fatal(err)
	}
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Create|fsnotify.Remove) > 0 {
					log.Println("Download dir watcher:", event)
					s.state.Lock()
					s.state.Downloads = s.listFiles()
					s.state.Unlock()
					if s.state.NumConnections() > 0 {
						s.state.Push()
					}
				}
			case err, ok := <-watcher.Errors:
				log.Println("Download dir watcher error:", err)
				if !ok {
					return
				}
			}
		}
	}()

	//torrents
	go func() {
		for range s.engine.TsChanged {
			log.Println("Torrents Updated")
			s.state.Lock()
			s.state.Torrents = s.engine.GetTorrents()
			s.state.Unlock()
			s.state.Push()
		}
	}()

}
