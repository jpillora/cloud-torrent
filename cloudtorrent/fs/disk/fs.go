package disk

import (
	"encoding/json"
	"log"

	"github.com/jpillora/cloud-torrent/cloudtorrent/fs"
	"github.com/spf13/afero"
)

func New() fs.FS {
	return &diskFS{
		Fs: afero.NewOsFs(),
	}
}

type diskFS struct {
	afero.Fs //already done!
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
	return &d.config, nil
}

func (d *diskFS) Sync(chan fs.Node) error {
	return nil
}

func logf(format string, args ...interface{}) {
	log.Printf("[Disk] "+format, args...)
}
