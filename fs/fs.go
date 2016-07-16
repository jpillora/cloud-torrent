package fs

import "github.com/spf13/afero"

type FSMode int

const (
	R  FSMode = 1 << 0
	W         = 1 << 1
	RW        = R | W
)

type FS interface {
	Mode() FSMode
	Config() interface{}
	afero.Fs
}
