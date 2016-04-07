package storage

import (
	"errors"
	"io"
	"os"
	"path"

	"github.com/anacrolix/missinggo"

	"github.com/anacrolix/torrent/metainfo"
)

type pieceFileStorage struct {
	fs missinggo.FileStore
}

func NewPieceFileStorage(fs missinggo.FileStore) I {
	return &pieceFileStorage{
		fs: fs,
	}
}

type pieceFileTorrentStorage struct {
	s *pieceFileStorage
}

func (me *pieceFileStorage) OpenTorrent(info *metainfo.InfoEx) (Torrent, error) {
	return &pieceFileTorrentStorage{me}, nil
}

func (me *pieceFileTorrentStorage) Close() error {
	return nil
}

func (me *pieceFileTorrentStorage) Piece(p metainfo.Piece) Piece {
	return pieceFileTorrentStoragePiece{me, p, me.s.fs}
}

type pieceFileTorrentStoragePiece struct {
	ts *pieceFileTorrentStorage
	p  metainfo.Piece
	fs missinggo.FileStore
}

func (me pieceFileTorrentStoragePiece) completedPath() string {
	return path.Join("completed", me.p.Hash().HexString())
}

func (me pieceFileTorrentStoragePiece) incompletePath() string {
	return path.Join("incomplete", me.p.Hash().HexString())
}

func (me pieceFileTorrentStoragePiece) GetIsComplete() bool {
	fi, err := me.ts.s.fs.Stat(me.completedPath())
	return err == nil && fi.Size() == me.p.Length()
}

func (me pieceFileTorrentStoragePiece) MarkComplete() error {
	return me.fs.Rename(me.incompletePath(), me.completedPath())
}

func (me pieceFileTorrentStoragePiece) openFile() (f missinggo.File, err error) {
	f, err = me.fs.OpenFile(me.completedPath(), os.O_RDONLY)
	if err == nil {
		var fi os.FileInfo
		fi, err = f.Stat()
		if err == nil && fi.Size() == me.p.Length() {
			return
		}
		f.Close()
	} else if !os.IsNotExist(err) {
		return
	}
	f, err = me.fs.OpenFile(me.incompletePath(), os.O_RDONLY)
	if os.IsNotExist(err) {
		err = io.ErrUnexpectedEOF
	}
	return
}

func (me pieceFileTorrentStoragePiece) ReadAt(b []byte, off int64) (n int, err error) {
	f, err := me.openFile()
	if err != nil {
		return
	}
	defer f.Close()
	missinggo.LimitLen(&b, me.p.Length()-off)
	n, err = f.ReadAt(b, off)
	off += int64(n)
	if off >= me.p.Length() {
		err = io.EOF
	} else if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return
}

func (me pieceFileTorrentStoragePiece) WriteAt(b []byte, off int64) (n int, err error) {
	if me.GetIsComplete() {
		err = errors.New("piece completed")
		return
	}
	f, err := me.fs.OpenFile(me.incompletePath(), os.O_WRONLY|os.O_CREATE)
	if err != nil {
		return
	}
	defer f.Close()
	missinggo.LimitLen(&b, me.p.Length()-off)
	return f.WriteAt(b, off)
}
