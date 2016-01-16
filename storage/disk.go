package storage

import "github.com/spf13/afero"

type DiskConfig struct {
	BasePath string `json:"basePath"`
}

func NewDisk(c DiskConfig) (afero.Fs, error) {
	return afero.NewBasePathFs(afero.NewOsFs(), c.BasePath), nil
}
