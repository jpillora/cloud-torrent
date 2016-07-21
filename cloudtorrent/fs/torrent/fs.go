package torrent

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
	return &torrentFS{}
}

type torrentFS struct {
	config struct {
		PeerID            string
		DownloadDirectory string
		EnableUpload      bool
		EnableSeeding     bool
		EnableEncryption  bool
		AutoStart         bool
		IncomingPort      int
	}
}

func (t *torrentFS) Name() string {
	return "Torrents"
}

func (t *torrentFS) Mode() fs.FSMode {
	return fs.RW
}

func (t *torrentFS) Configure(raw json.RawMessage) (interface{}, error) {
	if err := json.Unmarshal(raw, &t.config); err != nil {
		return nil, err
	}
	unset := t.config.PeerID == "" && t.config.IncomingPort == 0
	if t.config.PeerID == "" {
		t.config.PeerID = "Cloud Torrent"
	}
	if t.config.IncomingPort == 0 {
		t.config.IncomingPort = 4479
	}
	if unset {
		t.config.EnableEncryption = true
		t.config.EnableSeeding = true
		t.config.EnableUpload = true
	}
	return &t.config, nil
}

func (t *torrentFS) Update(chan fs.Node) error {
	return nil
}

func (t *torrentFS) Create(name string) (afero.File, error) {
	return &file{}, nil
}

func (t *torrentFS) Open(name string) (afero.File, error) {
	return &file{}, nil
}

func (t *torrentFS) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return t.Open(name)
}

func (t *torrentFS) Mkdir(name string, perm os.FileMode) error {
	return errors.New("not supported yet")
}

func (t *torrentFS) MkdirAll(path string, perm os.FileMode) error {
	return errors.New("not supported yet")
}

func (t *torrentFS) Remove(name string) error {
	return errors.New("not supported yet")
}

func (t *torrentFS) RemoveAll(path string) error {
	return errors.New("not supported yet")
}

func (t *torrentFS) Rename(oldname, newname string) error {
	return errors.New("not supported yet")
}

func (t *torrentFS) Stat(name string) (os.FileInfo, error) {
	return nil, errors.New("not supported yet")
}

func (t *torrentFS) Chmod(name string, mode os.FileMode) error {
	return errors.New("not supported yet")
}

func (t *torrentFS) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return errors.New("not supported yet")
}

func logf(format string, args ...interface{}) {
	log.Printf("[Torrents] "+format, args...)
}
