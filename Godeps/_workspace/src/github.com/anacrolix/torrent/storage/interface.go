package storage

import (
	"io"

	"github.com/anacrolix/torrent/metainfo"
)

// Represents data storage for an unspecified torrent.
type I interface {
	OpenTorrent(info *metainfo.InfoEx) (Torrent, error)
}

// Data storage bound to a torrent.
type Torrent interface {
	Piece(metainfo.Piece) Piece
	Close() error
}

// Interacts with torrent piece data.
type Piece interface {
	// Should return io.EOF only at end of torrent. Short reads due to missing
	// data should return io.ErrUnexpectedEOF.
	io.ReaderAt
	io.WriterAt
	// Called when the client believes the piece data will pass a hash check.
	// The storage can move or mark the piece data as read-only as it sees
	// fit.
	MarkComplete() error
	// Returns true if the piece is complete.
	GetIsComplete() bool
}
