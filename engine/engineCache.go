package engine

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/boypt/simple-torrent/common"
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
			if err == nil {
				defer cf.Close()
				_, err := cf.WriteString(magnetURI)
				common.HandleError(err)
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
			if err == nil {
				defer cf.Close()
				common.FancyHandleError(meta.Write(cf))
				log.Println("created torrent cache file", infohash)
			} else {
				log.Println("failed to create torrent file", err)
			}
		}
	}
}

func (e *Engine) removeMagnetCache(infohash string) {
	// remove both magnet and torrent cache if exists.
	cacheInfoPath := filepath.Join(e.cacheDir,
		fmt.Sprintf("%s%s.info", cacheSavedPrefix, infohash))
	if err := os.Remove(cacheInfoPath); err == nil {
		log.Printf("removed magnet info file %s", cacheInfoPath)
	} else if !os.IsNotExist(err) { // it's fine if the cache is not exists
		log.Printf("fail to removed cache file [%s], %s", infohash, err)
	}
}

func (e *Engine) removeTorrentCache(infohash string, toTrash bool) {
	fileName := fmt.Sprintf("%s%s.torrent", cacheSavedPrefix, infohash)
	cacheFilePath := filepath.Join(e.cacheDir, fileName)

	if toTrash {
		trashFilePath := filepath.Join(e.trashDir, fileName)
		if err := os.Rename(cacheFilePath, trashFilePath); err == nil {
			log.Printf("move torrent file to trash [%s]", trashFilePath)
		} else {
			log.Println("fail to move to trash", err)
		}
	} else {
		if err := os.Remove(cacheFilePath); err == nil {
			log.Printf("removed torrent file [%s]", cacheFilePath)
		} else if !os.IsNotExist(err) { // it's fine if the cache is not exists
			log.Printf("fail to removed cache file [%s] %s", infohash, err)
		}
	}
}

func (e *Engine) TorrentCacheFileName(infohash string) string {
	cacheFilePath := filepath.Join(e.cacheDir,
		fmt.Sprintf("%s%s.torrent", cacheSavedPrefix, infohash))
	return cacheFilePath
}

func (e *Engine) PushWaitTask(ih string) error {
	log.Println("Pushed task to wait", ih)
	e.pushWaitTask(ih, taskTorrent)
	info, err := metainfo.LoadFromFile(e.TorrentCacheFileName(ih))
	if err != nil {
		return err
	}
	spec := torrent.TorrentSpecFromMetaInfo(info)
	_, err = e.upsertTorrent(ih, spec.DisplayName, true)
	return err
}

func (e *Engine) RestoreTask(fn string) error {

	isCachedFile := strings.HasPrefix(filepath.Base(fn), cacheSavedPrefix)
	if strings.HasSuffix(fn, ".torrent") {
		if err := e.NewTorrentByFilePath(fn); err != nil {
			return err
		}
		if isCachedFile {
			log.Printf("[RestoreTask] Restored Torrent: %s \n", fn)
		} else {
			log.Printf("Task: added %s, file removed\n", fn)
			os.Remove(fn)
		}
	} else if strings.HasSuffix(fn, ".info") && isCachedFile {
		mag, err := ioutil.ReadFile(fn)
		if err != nil {
			log.Printf("Task: fail to read %s\n", fn)
			return err
		}
		if err := e.NewMagnet(string(mag)); err != nil {
			return err
		}
		log.Printf("[RestoreMagnet] Restored: %s \n", fn)
	} else {
		log.Println("Cache file doesn't match", fn)
	}

	return nil
}

func (e *Engine) RestoreCacheDir() {

	files, err := ioutil.ReadDir(e.cacheDir)
	if err != nil {
		log.Println("RestoreCacheDir failed read cachedir", err)
		return
	}

	// sort by modtime
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().Before(files[j].ModTime())
	})

	for _, i := range files {
		if i.IsDir() {
			continue
		}
		common.FancyHandleError(e.RestoreTask(path.Join(e.cacheDir, i.Name())))
	}
}

func (e *Engine) NextWaitTask() error {
	if !e.isReadyAddTask() {
		log.Println("NextWaitTask: engine tasks max")
		return ErrMaxConnTasks
	}

	for {
		if elm := e.waitList.Pop(); elm != nil {
			var res string
			te := elm.(taskElem)
			switch te.tp {
			case taskTorrent:
				res = fmt.Sprintf("%s%s.torrent", cacheSavedPrefix, te.ih)
			case taskMagnet:
				res = fmt.Sprintf("%s%s.info", cacheSavedPrefix, te.ih)
			}

			fn := path.Join(e.cacheDir, res)
			if _, err := os.Stat(fn); err != nil {
				log.Println("NextWaitTask RestoreTask err:", fn, err)
				continue
			}
			return e.RestoreTask(fn)
		} else {
			log.Println("NextWaitTask: wait list empty")
			return ErrWaitListEmpty
		}
	}
}

func (e *Engine) pushWaitTask(ih string, tp taskType) {
	e.waitList.Push(taskElem{ih: ih, tp: tp})
	log.Println("waitqueue len", e.waitList.Len())
}
