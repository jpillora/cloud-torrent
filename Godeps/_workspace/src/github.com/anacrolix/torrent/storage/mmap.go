package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/anacrolix/missinggo"
	"github.com/edsrzf/mmap-go"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/mmap_span"
)

type mmapStorage struct {
	baseDir string
}

func NewMMap(baseDir string) I {
	return &mmapStorage{
		baseDir: baseDir,
	}
}

func (me *mmapStorage) OpenTorrent(info *metainfo.InfoEx) (t Torrent, err error) {
	span, err := MMapTorrent(&info.Info, me.baseDir)
	t = &mmapTorrentStorage{
		span: span,
	}
	return
}

type mmapTorrentStorage struct {
	span      mmap_span.MMapSpan
	completed map[metainfo.Hash]bool
}

func (me *mmapTorrentStorage) Piece(p metainfo.Piece) Piece {
	return mmapStoragePiece{
		storage:  me,
		p:        p,
		ReaderAt: io.NewSectionReader(me.span, p.Offset(), p.Length()),
		WriterAt: missinggo.NewSectionWriter(me.span, p.Offset(), p.Length()),
	}
}

func (me *mmapTorrentStorage) Close() error {
	me.span.Close()
	return nil
}

type mmapStoragePiece struct {
	storage *mmapTorrentStorage
	p       metainfo.Piece
	io.ReaderAt
	io.WriterAt
}

func (me mmapStoragePiece) GetIsComplete() bool {
	return me.storage.completed[me.p.Hash()]
}

func (me mmapStoragePiece) MarkComplete() error {
	if me.storage.completed == nil {
		me.storage.completed = make(map[metainfo.Hash]bool)
	}
	me.storage.completed[me.p.Hash()] = true
	return nil
}

func MMapTorrent(md *metainfo.Info, location string) (mms mmap_span.MMapSpan, err error) {
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
	return
}
