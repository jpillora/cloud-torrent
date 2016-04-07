package storage

import (
	"io"
	"os"
	"path/filepath"

	"github.com/anacrolix/missinggo"

	"github.com/anacrolix/torrent/metainfo"
)

type fileStorage struct {
	baseDir   string
	completed map[[20]byte]bool
}

func NewFile(baseDir string) I {
	return &fileStorage{
		baseDir: baseDir,
	}
}

func (me *fileStorage) OpenTorrent(info *metainfo.InfoEx) (Torrent, error) {
	return fileTorrentStorage{me}, nil
}

type fileTorrentStorage struct {
	*fileStorage
}

func (me *fileStorage) Piece(p metainfo.Piece) Piece {
	_io := &fileStorageTorrent{
		p.Info,
		me.baseDir,
	}
	return &fileStoragePiece{
		me,
		p,
		missinggo.NewSectionWriter(_io, p.Offset(), p.Length()),
		io.NewSectionReader(_io, p.Offset(), p.Length()),
	}
}

func (me *fileStorage) Close() error {
	return nil
}

type fileStoragePiece struct {
	*fileStorage
	p metainfo.Piece
	io.WriterAt
	io.ReaderAt
}

func (me *fileStoragePiece) GetIsComplete() bool {
	return me.completed[me.p.Hash()]
}

func (me *fileStoragePiece) MarkComplete() error {
	if me.completed == nil {
		me.completed = make(map[[20]byte]bool)
	}
	me.completed[me.p.Hash()] = true
	return nil
}

type fileStorageTorrent struct {
	info    *metainfo.InfoEx
	baseDir string
}

// Returns EOF on short or missing file.
func (me *fileStorageTorrent) readFileAt(fi metainfo.FileInfo, b []byte, off int64) (n int, err error) {
	f, err := os.Open(me.fileInfoName(fi))
	if os.IsNotExist(err) {
		// File missing is treated the same as a short file.
		err = io.EOF
		return
	}
	if err != nil {
		return
	}
	defer f.Close()
	// Limit the read to within the expected bounds of this file.
	if int64(len(b)) > fi.Length-off {
		b = b[:fi.Length-off]
	}
	for off < fi.Length && len(b) != 0 {
		n1, err1 := f.ReadAt(b, off)
		b = b[n1:]
		n += n1
		off += int64(n1)
		if n1 == 0 {
			err = err1
			break
		}
	}
	return
}

// Only returns EOF at the end of the torrent. Premature EOF is ErrUnexpectedEOF.
func (me *fileStorageTorrent) ReadAt(b []byte, off int64) (n int, err error) {
	for _, fi := range me.info.UpvertedFiles() {
		for off < fi.Length {
			n1, err1 := me.readFileAt(fi, b, off)
			n += n1
			off += int64(n1)
			b = b[n1:]
			if len(b) == 0 {
				// Got what we need.
				return
			}
			if n1 != 0 {
				// Made progress.
				continue
			}
			err = err1
			if err == io.EOF {
				// Lies.
				err = io.ErrUnexpectedEOF
			}
			return
		}
		off -= fi.Length
	}
	err = io.EOF
	return
}

func (me *fileStorageTorrent) WriteAt(p []byte, off int64) (n int, err error) {
	for _, fi := range me.info.UpvertedFiles() {
		if off >= fi.Length {
			off -= fi.Length
			continue
		}
		n1 := len(p)
		if int64(n1) > fi.Length-off {
			n1 = int(fi.Length - off)
		}
		name := me.fileInfoName(fi)
		os.MkdirAll(filepath.Dir(name), 0770)
		var f *os.File
		f, err = os.OpenFile(name, os.O_WRONLY|os.O_CREATE, 0660)
		if err != nil {
			return
		}
		n1, err = f.WriteAt(p[:n1], off)
		f.Close()
		if err != nil {
			return
		}
		n += n1
		off = 0
		p = p[n1:]
		if len(p) == 0 {
			break
		}
	}
	return
}

func (me *fileStorageTorrent) fileInfoName(fi metainfo.FileInfo) string {
	return filepath.Join(append([]string{me.baseDir, me.info.Name}, fi.Path...)...)
}
