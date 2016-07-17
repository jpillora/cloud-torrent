package dropbox

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"time"

	"github.com/jpillora/cloud-torrent/cloudtorrent/fs"
	"github.com/spf13/afero"
)

func New() fs.FS {
	return &dropboxFS{}
}

type dropboxFS struct {
	config struct {
		Token string
	}
}

func (d *dropboxFS) Name() string {
	return "Dropbox"
}

func (d *dropboxFS) Mode() fs.FSMode {
	return fs.RW
}

func (d *dropboxFS) Configure(raw json.RawMessage) (interface{}, error) {
	if err := json.Unmarshal(raw, &d.config); err != nil {
		return nil, err
	}
	return &d.config, nil
}

func (d *dropboxFS) Sync(chan fs.Node) error {
	return nil
}

func (d *dropboxFS) Create(name string) (afero.File, error) {
	return &file{}, nil
}

func (d *dropboxFS) Open(name string) (afero.File, error) {
	return &file{}, nil
}

func (d *dropboxFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return d.Open(name)
}

func (d *dropboxFS) Mkdir(name string, perm os.FileMode) error {
	return errors.New("not supported yet")
}

func (d *dropboxFS) MkdirAll(path string, perm os.FileMode) error {
	return errors.New("not supported yet")
}

func (d *dropboxFS) Remove(name string) error {
	return errors.New("not supported yet")
}

func (d *dropboxFS) RemoveAll(path string) error {
	return errors.New("not supported yet")
}

func (d *dropboxFS) Rename(oldname, newname string) error {
	return errors.New("not supported yet")
}

func (d *dropboxFS) Stat(name string) (os.FileInfo, error) {
	return nil, errors.New("not supported yet")
}

func (d *dropboxFS) Chmod(name string, mode os.FileMode) error {
	return errors.New("not supported yet")
}

func (d *dropboxFS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return errors.New("not supported yet")
}

func logf(format string, args ...interface{}) {
	log.Printf("[Dropbox] "+format, args...)
}
