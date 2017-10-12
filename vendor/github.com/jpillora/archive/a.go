package archive

import (
	"io"
	"os"
	"time"
)

//a common interface to tar/zip
type archive interface {
	addBytes(path string, contents []byte, mtime time.Time) error
	addFile(path string, info os.FileInfo, f *os.File) error
	addReader(path string, info os.FileInfo, r io.Reader) error
	close() error
}
