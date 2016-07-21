package dropbox

import (
	"errors"
	"os"

	"github.com/jpillora/cloud-torrent/cloudtorrent/fs"
	dropbox "github.com/tj/go-dropbox"
)

func newFile(m *dropbox.Metadata) *file {
	return &file{
		children: map[string]*file{},
		meta:     m,
	}
}

func newFolder(name string) *file {
	return &file{
		children: map[string]*file{},
		meta: &dropbox.Metadata{
			Name: name,
			Tag:  "folder",
		},
	}
}

type file struct {
	children map[string]*file
	meta     *dropbox.Metadata
}

func (f *file) Children() []fs.Node {
	nodes := make([]fs.Node, len(f.children))
	i := 0
	for _, node := range f.children {
		nodes[i] = node
		i++
	}
	return nodes
}

func (f *file) update(m *dropbox.Metadata) bool {
	changed := false
	if *f.meta == *m {
		changed = true
		f.meta = m
	}
	return changed
}

func (f *file) Close() error {
	return errors.New("Not implemented")
}

func (f *file) Name() string {
	return f.meta.Name
}

func (f *file) Stat() (os.FileInfo, error) {
	return &fileInfo{meta: f.meta}, nil
}

func (f *file) Sync() error {
	return errors.New("Not implemented")
}

func (f *file) Truncate(size int64) error {
	return errors.New("Not implemented")
}

func (f *file) Read(b []byte) (n int, err error) {
	return 0, errors.New("Not implemented")
}

func (f *file) ReadAt(b []byte, off int64) (n int, err error) {
	return 0, errors.New("Not implemented")
}

func (f *file) Readdir(count int) (res []os.FileInfo, err error) {
	return nil, errors.New("Not implemented")
}

func (f *file) Readdirnames(n int) (names []string, err error) {
	return nil, errors.New("Not implemented")
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	return 0, errors.New("Not implemented")
}

func (f *file) Write(b []byte) (n int, err error) {
	return 0, errors.New("Not implemented")
}

func (f *file) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, errors.New("Not implemented")
}

func (f *file) WriteString(s string) (ret int, err error) {
	return 0, errors.New("Not implemented")
}
