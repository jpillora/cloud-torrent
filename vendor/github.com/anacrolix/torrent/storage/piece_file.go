package storage

import (
	"io"
	"os"
	"path"

	"github.com/anacrolix/missinggo"

	"github.com/anacrolix/torrent/metainfo"
)

type pieceFileStorage struct {
	fs missinggo.FileStore
}

func NewFileStorePieces(fs missinggo.FileStore) ClientImpl {
	return &pieceFileStorage{
		fs: fs,
	}
}

func (pieceFileStorage) Close() error { return nil }

type pieceFileTorrentStorage struct {
	s *pieceFileStorage
}

func (s *pieceFileStorage) OpenTorrent(info *metainfo.Info, infoHash metainfo.Hash) (TorrentImpl, error) {
	return &pieceFileTorrentStorage{s}, nil
}

func (s *pieceFileTorrentStorage) Close() error {
	return nil
}

func (s *pieceFileTorrentStorage) Piece(p metainfo.Piece) PieceImpl {
	return pieceFileTorrentStoragePiece{s, p, s.s.fs}
}

type pieceFileTorrentStoragePiece struct {
	ts *pieceFileTorrentStorage
	p  metainfo.Piece
	fs missinggo.FileStore
}

func (s pieceFileTorrentStoragePiece) completedPath() string {
	return path.Join("completed", s.p.Hash().HexString())
}

func (s pieceFileTorrentStoragePiece) incompletePath() string {
	return path.Join("incomplete", s.p.Hash().HexString())
}

func (s pieceFileTorrentStoragePiece) GetIsComplete() bool {
	fi, err := s.fs.Stat(s.completedPath())
	return err == nil && fi.Size() == s.p.Length()
}

func (s pieceFileTorrentStoragePiece) MarkComplete() error {
	return s.fs.Rename(s.incompletePath(), s.completedPath())
}

func (s pieceFileTorrentStoragePiece) MarkNotComplete() error {
	return s.fs.Remove(s.completedPath())
}

func (s pieceFileTorrentStoragePiece) openFile() (f missinggo.File, err error) {
	f, err = s.fs.OpenFile(s.completedPath(), os.O_RDONLY)
	if err == nil {
		var fi os.FileInfo
		fi, err = f.Stat()
		if err == nil && fi.Size() == s.p.Length() {
			return
		}
		f.Close()
	} else if !os.IsNotExist(err) {
		return
	}
	f, err = s.fs.OpenFile(s.incompletePath(), os.O_RDONLY)
	if os.IsNotExist(err) {
		err = io.ErrUnexpectedEOF
	}
	return
}

func (s pieceFileTorrentStoragePiece) ReadAt(b []byte, off int64) (n int, err error) {
	f, err := s.openFile()
	if err != nil {
		return
	}
	defer f.Close()
	return f.ReadAt(b, off)
}

func (s pieceFileTorrentStoragePiece) WriteAt(b []byte, off int64) (n int, err error) {
	f, err := s.fs.OpenFile(s.incompletePath(), os.O_WRONLY|os.O_CREATE)
	if err != nil {
		return
	}
	defer f.Close()
	return f.WriteAt(b, off)
}
