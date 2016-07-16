package disk

import (
	"github.com/jpillora/cloud-torrent/fs"
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
		Base string `json:"base"`
	}
}

func (d *diskFS) Name() string {
	return "Disk"
}

func (d *diskFS) Mode() fs.FSMode {
	return fs.RW
}

func (d *diskFS) Config() interface{} {
	return &d.config
}
