package archive

import (
	"compress/gzip"
	"errors"
	"io"
	"sync"
	"time"

	"os"
	"path/filepath"
)

//Archive** combines Go's archive/zip,tar into a simpler, higher-level API
type Archive struct {
	//config
	DirMaxSize  int64 //defaults to no limit (-1)
	DirMaxFiles int   //defaults to no limit (-1)
	//state
	dst     io.Writer
	lock    sync.Mutex
	archive archive
}

//NewWriter is useful when you'd like a dynamic archive type using a filename with extension
func NewWriter(filename string, dst io.Writer) (*Archive, error) {

	a := &Archive{
		DirMaxSize:  -1,
		DirMaxFiles: -1,
		dst:         dst,
	}

	switch Extension(filename) {
	case ".tar":
		a.archive = newTarArchive(dst)
	case ".tar.gz":
		gz := gzip.NewWriter(dst)
		a.dst = gz
		a.archive = newTarArchive(gz)
	case ".zip":
		a.archive = newZipArchive(dst)
	default:
		return nil, errors.New("Invalid extension: " + filename)
	}

	return a, nil

}

func NewTarWriter(dst io.Writer) *Archive {
	a, _ := NewWriter(".tar", dst)
	return a
}

func NewTarGzWriter(dst io.Writer) *Archive {
	a, _ := NewWriter(".tar.gz", dst)
	return a
}

func NewZipWriter(dst io.Writer) *Archive {
	a, _ := NewWriter(".zip", dst)
	return a
}

func (a *Archive) AddBytes(path string, contents []byte) error {
	return a.AddBytesMTime(path, contents, time.Now())
}

func (a *Archive) AddBytesMTime(path string, contents []byte, mtime time.Time) error {
	if err := checkPath(path); err != nil {
		return err
	}
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.archive.addBytes(path, contents, mtime)
}

func (a *Archive) AddInfoReader(path string, info os.FileInfo, r io.Reader) error {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.archive.addReader(path, info, r)
}

//You can prevent archive from performing an extra Stat by using AddInfoFile
//instead of AddFile
func (a *Archive) AddInfoFile(path string, info os.FileInfo, f *os.File) error {
	if err := checkPath(path); err != nil {
		return err
	}
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.archive.addFile(path, info, f)
}

func (a *Archive) AddFile(path string, f *os.File) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	return a.AddInfoFile(path, info, f)
}

func (a *Archive) AddDir(path string) error {
	size := int64(0)
	num := 0
	return filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if a.DirMaxSize >= 0 {
			size += info.Size()
			if size > a.DirMaxSize {
				return errors.New("Surpassed maximum archive size")
			}
		}
		if a.DirMaxFiles >= 0 {
			num++
			if num == a.DirMaxFiles+1 {
				return errors.New("Surpassed maximum number of files in archive")
			}
		}
		// log.Printf("#%d %09d add file %s", num, size, p)
		rel, err := filepath.Rel(path, p)
		if err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		return a.archive.addFile(rel, info, f)
	})
}

func (a *Archive) Close() error {
	if err := a.archive.close(); err != nil {
		return err
	}
	if gz, ok := a.dst.(*gzip.Writer); ok {
		if err := gz.Close(); err != nil {
			return err
		}
	}
	return nil
}
