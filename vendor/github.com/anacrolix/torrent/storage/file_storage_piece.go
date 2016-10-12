package storage

import (
	"io"
	"os"

	"github.com/anacrolix/torrent/metainfo"
)

type fileStoragePiece struct {
	*fileTorrentImpl
	p metainfo.Piece
	io.WriterAt
	io.ReaderAt
}

func (me *fileStoragePiece) pieceKey() metainfo.PieceKey {
	return metainfo.PieceKey{me.infoHash, me.p.Index()}
}

func (fs *fileStoragePiece) GetIsComplete() bool {
	ret, err := fs.completion.Get(fs.pieceKey())
	if err != nil || !ret {
		return false
	}
	// If it's allegedly complete, check that its constituent files have the
	// necessary length.
	for _, fi := range extentCompleteRequiredLengths(fs.p.Info, fs.p.Offset(), fs.p.Length()) {
		s, err := os.Stat(fs.fileInfoName(fi))
		if err != nil || s.Size() < fi.Length {
			ret = false
			break
		}
	}
	if ret {
		return true
	}
	// The completion was wrong, fix it.
	fs.completion.Set(fs.pieceKey(), false)
	return false
}

func (fs *fileStoragePiece) MarkComplete() error {
	fs.completion.Set(fs.pieceKey(), true)
	return nil
}

func (fs *fileStoragePiece) MarkNotComplete() error {
	fs.completion.Set(fs.pieceKey(), false)
	return nil
}
