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
	pc      PieceCompletion
}

func NewMMap(baseDir string) ClientImpl {
	return NewMMapWithCompletion(baseDir, pieceCompletionForDir(baseDir))
}

func NewMMapWithCompletion(baseDir string, completion PieceCompletion) ClientImpl {
	return &mmapStorage{
		baseDir: baseDir,
		pc:      completion,
	}
}

func (s *mmapStorage) OpenTorrent(info *metainfo.Info, infoHash metainfo.Hash) (t TorrentImpl, err error) {
	span, err := mMapTorrent(info, s.baseDir)
	t = &mmapTorrentStorage{
		infoHash: infoHash,
		span:     span,
		pc:       s.pc,
	}
	return
}

func (s *mmapStorage) Close() error {
	return s.pc.Close()
}

type mmapTorrentStorage struct {
	infoHash metainfo.Hash
	span     mmap_span.MMapSpan
	pc       PieceCompletion
}

func (ts *mmapTorrentStorage) Piece(p metainfo.Piece) PieceImpl {
	return mmapStoragePiece{
		pc:       ts.pc,
		p:        p,
		ih:       ts.infoHash,
		ReaderAt: io.NewSectionReader(ts.span, p.Offset(), p.Length()),
		WriterAt: missinggo.NewSectionWriter(ts.span, p.Offset(), p.Length()),
	}
}

func (ts *mmapTorrentStorage) Close() error {
	ts.pc.Close()
	return ts.span.Close()
}

type mmapStoragePiece struct {
	pc PieceCompletion
	p  metainfo.Piece
	ih metainfo.Hash
	io.ReaderAt
	io.WriterAt
}

func (me mmapStoragePiece) pieceKey() metainfo.PieceKey {
	return metainfo.PieceKey{me.ih, me.p.Index()}
}

func (sp mmapStoragePiece) GetIsComplete() (ret bool) {
	ret, _ = sp.pc.Get(sp.pieceKey())
	return
}

func (sp mmapStoragePiece) MarkComplete() error {
	sp.pc.Set(sp.pieceKey(), true)
	return nil
}

func (sp mmapStoragePiece) MarkNotComplete() error {
	sp.pc.Set(sp.pieceKey(), false)
	return nil
}

func mMapTorrent(md *metainfo.Info, location string) (mms mmap_span.MMapSpan, err error) {
	defer func() {
		if err != nil {
			mms.Close()
		}
	}()
	for _, miFile := range md.UpvertedFiles() {
		fileName := filepath.Join(append([]string{location, md.Name}, miFile.Path...)...)
		var mm mmap.MMap
		mm, err = mmapFile(fileName, miFile.Length)
		if err != nil {
			err = fmt.Errorf("file %q: %s", miFile.DisplayPath(md), err)
			return
		}
		if mm != nil {
			mms.Append(mm)
		}
	}
	return
}

func mmapFile(name string, size int64) (ret mmap.MMap, err error) {
	dir := filepath.Dir(name)
	err = os.MkdirAll(dir, 0777)
	if err != nil {
		err = fmt.Errorf("making directory %q: %s", dir, err)
		return
	}
	var file *os.File
	file, err = os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return
	}
	defer file.Close()
	var fi os.FileInfo
	fi, err = file.Stat()
	if err != nil {
		return
	}
	if fi.Size() < size {
		// I think this is necessary on HFS+. Maybe Linux will SIGBUS too if
		// you overmap a file but I'm not sure.
		err = file.Truncate(size)
		if err != nil {
			return
		}
	}
	if size == 0 {
		// Can't mmap() regions with length 0.
		return
	}
	ret, err = mmap.MapRegion(file,
		int(size), // Probably not great on <64 bit systems.
		mmap.RDWR, 0, 0)
	if err != nil {
		err = fmt.Errorf("mapping file %q, length %d: %s", file.Name(), size, err)
		return
	}
	if int64(len(ret)) != size {
		panic("mmap has wrong length")
	}
	return
}
