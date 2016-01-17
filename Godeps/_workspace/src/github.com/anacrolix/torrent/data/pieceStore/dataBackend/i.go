package dataBackend

import (
	"io"
	"os"
)

// All functions must return ErrNotFound as required.
type I interface {
	GetLength(path string) (int64, error)
	Open(path string, flags int) (File, error)
	OpenSection(path string, off, n int64) (io.ReadCloser, error)
	Delete(path string) error
}

var ErrNotFound = os.ErrNotExist

type File interface {
	io.Closer
	io.Seeker
	io.Writer
	io.Reader
}
