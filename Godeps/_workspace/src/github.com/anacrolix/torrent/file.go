package torrent

import (
	"strings"

	"github.com/anacrolix/torrent/metainfo"
)

// Provides access to regions of torrent data that correspond to its files.
type File struct {
	t      Torrent
	path   string
	offset int64
	length int64
	fi     metainfo.FileInfo
}

func (f *File) Torrent() Torrent {
	return f.t
}

// Data for this file begins this far into the torrent.
func (f *File) Offset() int64 {
	return f.offset
}

func (f File) FileInfo() metainfo.FileInfo {
	return f.fi
}

func (f File) Path() string {
	return f.path
}

func (f *File) Length() int64 {
	return f.length
}

// The relative file path for a multi-file torrent, and the torrent name for a
// single-file torrent.
func (f *File) DisplayPath() string {
	fip := f.FileInfo().Path
	if len(fip) == 0 {
		return f.t.Info().Name
	}
	return strings.Join(fip, "/")

}

type FilePieceState struct {
	Bytes int64 // Bytes within the piece that are part of this File.
	PieceState
}

// Returns the state of pieces in this file.
func (f *File) State() (ret []FilePieceState) {
	pieceSize := int64(f.t.torrent.usualPieceSize())
	off := f.offset % pieceSize
	remaining := f.length
	for i := int(f.offset / pieceSize); ; i++ {
		if remaining == 0 {
			break
		}
		len1 := pieceSize - off
		if len1 > remaining {
			len1 = remaining
		}
		f.t.cl.mu.RLock()
		ps := f.t.torrent.pieceState(i)
		f.t.cl.mu.RUnlock()
		ret = append(ret, FilePieceState{len1, ps})
		off = 0
		remaining -= len1
	}
	return
}

// Marks pieces in the region of the file for download. This is a helper
// wrapping Torrent.SetRegionPriority.
func (f *File) PrioritizeRegion(off, len int64) {
	if off < 0 || off >= f.length {
		return
	}
	if off+len > f.length {
		len = f.length - off
	}
	off += f.offset
	f.t.SetRegionPriority(off, len)
}
