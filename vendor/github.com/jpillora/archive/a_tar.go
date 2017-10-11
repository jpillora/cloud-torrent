package archive

import (
	"archive/tar"
	"errors"
	"io"
	"time"

	"os"
)

var TarFileMode = os.FileMode(0755)

type tarArchive struct {
	writer   *tar.Writer
	filemode os.FileMode
}

func newTarArchive(dst io.Writer) *tarArchive {
	return &tarArchive{
		writer:   tar.NewWriter(dst),
		filemode: TarFileMode,
	}
}

func (a *tarArchive) addReader(path string, info os.FileInfo, r io.Reader) error {
	if !info.Mode().IsRegular() {
		return errors.New("Only regular files supported: " + path)
	}

	h, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}

	h.Name = path
	if err := a.writer.WriteHeader(h); err != nil {
		return err
	}

	_, err = io.Copy(a.writer, r)
	return err
}

func (a *tarArchive) addBytes(path string, contents []byte, mtime time.Time) error {
	h := &tar.Header{
		Name:    path,
		Size:    int64(len(contents)),
		ModTime: mtime,
		Mode:    int64(a.filemode),
	}
	if err := a.writer.WriteHeader(h); err != nil {
		return err
	}
	_, err := a.writer.Write(contents)
	return err
}

func (a *tarArchive) addFile(path string, info os.FileInfo, f *os.File) error {
	if !info.Mode().IsRegular() {
		return errors.New("Only regular files supported: " + path)
	}
	h, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	h.Name = path
	if err := a.writer.WriteHeader(h); err != nil {
		return err
	}
	n, err := io.Copy(a.writer, f)
	if info.Size() != n {
		return errors.New("Size mismatch: " + path)
	}
	return err
}

func (a *tarArchive) close() error {
	return a.writer.Close()
}
