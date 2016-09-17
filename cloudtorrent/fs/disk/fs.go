package disk

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jpillora/cloud-torrent/cloudtorrent/fs"
	"github.com/jpillora/filenotify"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/afero"
)

func New() fs.FS {
	return &diskFS{
		Fs: nil,
	}
}

type diskFS struct {
	afero.Fs //already done!
	watcher  filenotify.FileWatcher
	config   struct {
		Base string
	}
}

func (d *diskFS) Name() string {
	return "Disk"
}

func (d *diskFS) Mode() fs.FSMode {
	return fs.RW
}

func (d *diskFS) Configure(raw json.RawMessage) (interface{}, error) {
	if err := json.Unmarshal(raw, &d.config); err != nil {
		return nil, err
	}
	base := d.config.Base
	if base == "" {
		if hd, err := homedir.Dir(); err == nil {
			base = filepath.Join(hd, "downloads")
		} else if wd, err := os.Getwd(); err == nil {
			base = filepath.Join(wd, "downloads")
		} else {
			return nil, errors.New("Cannot find default base directory")
		}
	}
	info, err := os.Stat(base)
	if os.IsNotExist(err) {
		return nil, errors.New("Cannot find directory")
	} else if err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, errors.New("Path is not a directory")
	}
	//ready!
	d.config.Base = base
	d.Fs = afero.NewBasePathFs(afero.NewOsFs(), base)
	return &d.config, nil
}

func (d *diskFS) Update(chan fs.Node) error {
	d.watcher = filenotify.New()
	//set poll interval (if polling is being used)
	filenotify.SetPollInterval(d.watcher, time.Second)
	d.watcher.Add(d.config.Base)
	for event := range d.watcher.Events() {
		log.Printf("event %+v", event)
	}
	return nil
}

func logf(format string, args ...interface{}) {
	log.Printf("[Disk] "+format, args...)
}
