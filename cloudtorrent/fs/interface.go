package fs

import (
	"encoding/json"

	"github.com/spf13/afero"
)

type FSMode int

const (
	R  FSMode = 1 << 0
	W         = 1 << 1
	RW        = R | W
)

type FS interface {
	Mode() FSMode
	Configure(json.RawMessage) (interface{}, error)
	Update(chan Node) error
	afero.Fs
}

type Node interface {
	afero.File
	Children() []Node
}
