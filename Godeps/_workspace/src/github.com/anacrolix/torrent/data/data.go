package data

import (
	"io"

	"github.com/anacrolix/torrent/metainfo"
)

type Store interface {
	OpenTorrent(*metainfo.Info) Data
}

// Represents data storage for a Torrent. Additional optional interfaces to
// implement are io.Closer, io.ReaderAt, StatefulData, and SectionOpener.
type Data interface {
	// OpenSection(off, n int64) (io.ReadCloser, error)
	// ReadAt(p []byte, off int64) (n int, err error)
	// Close()
	WriteAt(p []byte, off int64) (n int, err error)
	WriteSectionTo(w io.Writer, off, n int64) (written int64, err error)
}
