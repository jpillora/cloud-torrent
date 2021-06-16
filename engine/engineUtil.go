package engine

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/fsnotify/fsnotify"
)

func (e *Engine) isTaskInList(ih string) bool {
	e.RLock()
	defer e.RUnlock()
	_, ok := e.ts[ih]
	return ok
}

func (e *Engine) upsertTorrent(ih, name string, isQueueing bool) (*Torrent, error) {
	defer func() {
		e.TsChanged <- struct{}{}
	}()

	e.RLock()
	torrent, ok := e.ts[ih]
	e.RUnlock()
	if !ok {
		torrent = &Torrent{
			Name:       name,
			InfoHash:   ih,
			IsQueueing: isQueueing,
			AddedAt:    time.Now(),
			cldServer:  e.cldServer,
			dropWait:   make(chan struct{}),
		}
		e.Lock()
		e.ts[ih] = torrent
		e.Unlock()
		return torrent, nil
	}
	torrent.IsQueueing = isQueueing
	return torrent, ErrTaskExists
}

func (e *Engine) getTorrent(infohash string) (*Torrent, error) {
	if t, ok := e.ts[infohash]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("Missing torrent %x", infohash)
}

func (e *Engine) deleteTorrent(infohash string) {
	delete(e.ts, infohash)
	e.TsChanged <- struct{}{}
}

func (e *Engine) UpdateTrackers() error {
	var txtlines []string
	url := e.config.TrackerListURL

	if !strings.HasPrefix(url, "https://") {
		err := fmt.Errorf("UpdateTrackers: trackers url invalid: %s (only https:// supported), extra trackers list now empty.", url)
		log.Println(err.Error())
		return err
	}

	log.Printf("UpdateTrackers: loading trackers from %s\n", url)
	client := http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		txtlines = append(txtlines, line)
	}

	e.bttracker = txtlines
	log.Printf("UpdateTrackers: loaded %d trackers \n", len(txtlines))
	return nil
}

func (e *Engine) WriteStauts(_w io.Writer) {
	e.RLock()
	defer e.RUnlock()
	if e.client != nil {
		e.client.WriteStatus(_w)
	}
}

func (e *Engine) ConnStat() torrent.ConnStats {
	e.RLock()
	defer e.RUnlock()
	if e.client != nil {
		return e.client.ConnStats()
	}
	return torrent.ConnStats{}
}

func (e *Engine) StartTorrentWatcher() error {

	if e.watcher != nil {
		log.Println("Torrent Watcher: close")
		e.watcher.Close()
		e.watcher = nil
	}

	if w, err := os.Stat(e.config.WatchDirectory); os.IsNotExist(err) || (err == nil && !w.IsDir()) {
		return fmt.Errorf("[Watcher] WatchDirectory [%s] is not a dir, will not watch", e.config.WatchDirectory)
	}

	log.Printf("Torrent Watcher: watching torrent file in %s", e.config.WatchDirectory)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	e.watcher = watcher

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// log.Println("event:", event)
				if event.Op&fsnotify.Write == fsnotify.Write {
					if !strings.HasSuffix(event.Name, ".torrent") {
						continue
					}
					if st, err := os.Stat(event.Name); err != nil {
						log.Println(err)
						continue
					} else if st.IsDir() {
						continue
					}

					if err := e.NewTorrentByFilePath(event.Name); err == nil {
						log.Printf("Torrent Watcher: added %s, file removed\n", event.Name)
						os.Remove(event.Name)
					} else {
						log.Printf("Torrent Watcher: fail to add %s, ERR:%#v\n", event.Name, err)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}()
	err = watcher.Add(e.config.WatchDirectory)
	if err != nil {
		log.Fatal(err)
	}

	return nil
}
