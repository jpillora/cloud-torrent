package engine

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

func (e *Engine) isTaskInList(ih string) bool {
	e.RLock()
	defer e.RUnlock()
	_, ok := e.ts[ih]
	return ok
}

func (e *Engine) upsertTorrent(ih, name string) *Torrent {
	e.RLock()
	torrent, ok := e.ts[ih]
	e.RUnlock()
	if !ok {
		torrent = &Torrent{
			Name:     name,
			InfoHash: ih,
			AddedAt:  time.Now(),
			dropWait: make(chan struct{}),
		}
		e.Lock()
		e.ts[ih] = torrent
		e.Unlock()
	}
	//update torrent fields using underlying torrent
	// torrent.Update(tt)
	return torrent
}

func (e *Engine) getTorrent(infohash string) (*Torrent, error) {
	e.RLock()
	defer e.RUnlock()
	ih := metainfo.NewHashFromHex(infohash)
	t, ok := e.ts[ih.HexString()]
	if !ok {
		return t, fmt.Errorf("Missing torrent %x", ih)
	}
	return t, nil
}

func (e *Engine) callDoneCmd(env []string) {
	if e.config.DoneCmd == "" {
		return
	}

	cmd := exec.Command(e.config.DoneCmd)
	cmd.Env = env
	log.Printf("[DoneCmd] [%s] environ:%v", e.config.DoneCmd, cmd.Env)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("[DoneCmd] Err:", err)
		return
	}
	log.Println("[DoneCmd] Exit:", cmd.ProcessState.ExitCode(), "Output:", string(out))
}

func (e *Engine) UpdateTrackers() error {
	var txtlines []string
	url := e.config.TrackerListURL

	if !strings.HasPrefix(url, "https://") {
		err := fmt.Errorf("UpdateTrackers: trackers url invalid: %s (only https:// supported), extra trackers list now empty.", url)
		log.Print(err.Error())
		e.bttracker = txtlines
		return err
	}

	log.Printf("UpdateTrackers: loading trackers from %s\n", url)
	resp, err := http.Get(url)
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
