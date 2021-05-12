package engine

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/anacrolix/torrent/metainfo"
)

const (
	cacheSavedPrefix = "_CLDAUTOSAVED_"
)

func (e *Engine) newMagnetCacheFile(magnetURI, infohash string) {
	// create .info file with hash as filename
	if w, err := os.Stat(e.cacheDir); err == nil && w.IsDir() {
		cacheInfoPath := filepath.Join(e.cacheDir,
			fmt.Sprintf("%s%s.info", cacheSavedPrefix, infohash))
		if _, err := os.Stat(cacheInfoPath); os.IsNotExist(err) {
			cf, err := os.Create(cacheInfoPath)
			defer cf.Close()
			if err == nil {
				cf.WriteString(magnetURI)
				log.Println("created magnet cache info file", infohash)
			}
		}
	}
}

func (e *Engine) newTorrentCacheFile(meta *metainfo.MetaInfo) {
	// create .torrent file
	infohash := meta.HashInfoBytes().HexString()
	if w, err := os.Stat(e.cacheDir); err == nil && w.IsDir() {
		cacheFilePath := filepath.Join(e.cacheDir,
			fmt.Sprintf("%s%s.torrent", cacheSavedPrefix, infohash))
		// only create the cache file if not exists
		// avoid recreating cache files during boot import
		if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
			cf, err := os.Create(cacheFilePath)
			defer cf.Close()
			if err == nil {
				meta.Write(cf)
				log.Println("created torrent cache file", infohash)
			} else {
				log.Println("failed to create torrent file ", err)
			}
		}
	}
}

func (e *Engine) removeMagnetCache(infohash string) {
	// remove both magnet and torrent cache if exists.
	cacheInfoPath := filepath.Join(e.cacheDir,
		fmt.Sprintf("%s%s.info", cacheSavedPrefix, infohash))
	if err := os.Remove(cacheInfoPath); err == nil {
		log.Printf("removed magnet info file %s", infohash)
	}
}

func (e *Engine) removeTorrentCache(infohash string) {
	cacheFilePath := filepath.Join(e.cacheDir,
		fmt.Sprintf("%s%s.torrent", cacheSavedPrefix, infohash))
	if err := os.Remove(cacheFilePath); err == nil {
		log.Printf("removed torrent file %s", infohash)
	} else {
		log.Printf("fail to removed torrent file %s, %s", infohash, err)
	}
}

func (e *Engine) RestoreTorrent(fnpattern string) {
	// restore saved torrent tasks
	tors, _ := filepath.Glob(filepath.Join(e.config.WatchDirectory, fnpattern))
	for _, t := range tors {
		if err := e.NewTorrentByFilePath(t); err == nil {
			if strings.HasPrefix(filepath.Base(t), cacheSavedPrefix) {
				log.Printf("Task Restored: %s \n", t)
			} else {
				log.Printf("Task: added %s, file removed\n", t)
				os.Remove(t)
			}
		} else {
			log.Printf("Inital Task: fail to add %s, ERR:%#v\n", t, err)
		}
	}
}

func (e *Engine) RestoreMagnet(fnpattern string) {
	// restore saved magnet tasks
	infos, _ := filepath.Glob(filepath.Join(e.config.WatchDirectory, fnpattern))
	for _, i := range infos {
		fn := filepath.Base(i)
		// only restore our cache file
		if strings.HasPrefix(fn, cacheSavedPrefix) && len(fn) == 59 {
			mag, err := ioutil.ReadFile(i)
			if err != nil {
				continue
			}
			if err := e.NewMagnet(string(mag)); err == nil {
				log.Printf("Task Restored: %s \n", fn)
			} else {
				log.Printf("Task: fail to add %s, ERR:%#v\n", fn, err)
			}
		}
	}
}

func (e *Engine) restoreFromElem(te *taskElem) {
	fn := te.Filename()
	switch te.tp {
	case taskMagnet:
		e.RestoreMagnet(fn)
	case taskTorrent:
		e.RestoreTorrent(fn)
	}
}

func (e *Engine) nextWaitTask() {
	if elm := e.waitList.Pop(); elm != nil {
		taskElm := elm.(taskElem)
		log.Println("nextWaitTask", taskElm.Filename())
		e.restoreFromElem(&taskElm)
	} else {
		log.Println("nextWaitTask: nil")
	}
}
