package engine

import (
	"errors"
	"io"

	"github.com/anacrolix/torrent"
	"github.com/jpillora/cloud-torrent/storage"
	"github.com/jpillora/media-sort/search"
	"github.com/jpillora/media-sort/sort"
)

func init() {
	mediasearch.Info = false
}

func NewFile(path string, size int64) *File {
	return &File{Path: path, Size: size}
}

type StorageInfo struct {
	ID   string
	Path string
}

type File struct {
	//static
	Path string
	Size int64
	//storage state
	Sorted    bool
	NewPath   string
	StorageID string
	//client state
	Started   bool
	Chunks    int
	Completed int
	Percent   float32
	//server state
	f  torrent.File
	fs storage.Fs
}

func (f *File) Start(fs storage.Fs) error {
	f.fs = fs
	return nil
}

func (f *File) Stop() error {
	return nil
}

func (f *File) sort(sortConfig *MediaSortConfig) {
	result, err := mediasort.Sort(f.Path)
	if err != nil {
		return
	}
	newPath, err := result.PrettyPath(mediasort.PathConfig{})
	if err != nil {
		return
	}
	f.NewPath = newPath
	f.Sorted = true
}

func (f *File) ReadAt(p []byte, off int64) (n int, err error) {
	// time.Sleep(1 * time.Second)
	return 0, nil //errors.New("not implemented")
}
func (f *File) WriteAt(p []byte, off int64) (n int, err error) {
	return 0, errors.New("not implemented")
}
func (f *File) WriteSectionTo(w io.Writer, off, n int64) (written int64, err error) {
	return 0, errors.New("not implemented")
}
