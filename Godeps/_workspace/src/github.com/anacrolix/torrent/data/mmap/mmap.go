package mmap

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/edsrzf/mmap-go"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/mmap_span"
)

type torrentData struct {
	// Supports non-torrent specific data operations for the torrent.Data
	// interface.
	mmap_span.MMapSpan

	completed []bool
}

func (me *torrentData) PieceComplete(piece int) bool {
	return me.completed[piece]
}

func (me *torrentData) PieceCompleted(piece int) error {
	me.completed[piece] = true
	return nil
}

func TorrentData(md *metainfo.Info, location string) (ret *torrentData, err error) {
	var mms mmap_span.MMapSpan
	defer func() {
		if err != nil {
			mms.Close()
		}
	}()
	for _, miFile := range md.UpvertedFiles() {
		fileName := filepath.Join(append([]string{location, md.Name}, miFile.Path...)...)
		err = os.MkdirAll(filepath.Dir(fileName), 0777)
		if err != nil {
			err = fmt.Errorf("error creating data directory %q: %s", filepath.Dir(fileName), err)
			return
		}
		var file *os.File
		file, err = os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0666)
		if err != nil {
			return
		}
		func() {
			defer file.Close()
			var fi os.FileInfo
			fi, err = file.Stat()
			if err != nil {
				return
			}
			if fi.Size() < miFile.Length {
				err = file.Truncate(miFile.Length)
				if err != nil {
					return
				}
			}
			if miFile.Length == 0 {
				// Can't mmap() regions with length 0.
				return
			}
			var mMap mmap.MMap
			mMap, err = mmap.MapRegion(file,
				int(miFile.Length), // Probably not great on <64 bit systems.
				mmap.RDWR, 0, 0)
			if err != nil {
				err = fmt.Errorf("error mapping file %q, length %d: %s", file.Name(), miFile.Length, err)
				return
			}
			if int64(len(mMap)) != miFile.Length {
				panic("mmap has wrong length")
			}
			mms.Append(mMap)
		}()
		if err != nil {
			return
		}
	}
	ret = &torrentData{
		MMapSpan:  mms,
		completed: make([]bool, md.NumPieces()),
	}
	return
}
