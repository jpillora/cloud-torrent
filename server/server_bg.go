package server

import (
	"path"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
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

// stateRoutines watches the download dir / tasks / sys states
func (s *Server) stateRoutines() {

	// initial state
	s.state.Lock()
	s.state.Torrents = s.engine.GetTorrents()
	s.state.Downloads = s.listFiles()
	s.state.Stats.System.loadStats()
	s.state.Unlock()

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
				s.state.Downloads = s.listFiles()
			case <-s.connSyncState: // web user connected
				s.state.Stats.System.loadStats()
				s.state.Torrents = s.engine.GetTorrents()
				s.state.Stats.ConnStat = s.engine.ConnStat()
				s.state.Downloads = s.listFiles()
			case <-s.engine.TsChanged: // task added/deleted
				s.state.Torrents = s.engine.GetTorrents()
			}

			s.state.Push()
		}
	}()

	// download dir watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	// TODO: user may change download dir in run time?
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
					if strings.HasPrefix(path.Base(event.Name), ".") {
						// ignore hidden files
						continue
					}

					log.Println("Download dir watcher:", event)
					s.connSyncState <- struct{}{}
				}
			case err, ok := <-watcher.Errors:
				log.Println("Download dir watcher error:", err)
				if !ok {
					return
				}
			}
		}
	}()
}
