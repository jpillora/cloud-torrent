package fs

import "encoding/json"

type FSMode int

const (
	R  FSMode = 1 << 0
	W         = 1 << 1
	RW        = R | W
)

type FS interface {
	Name() string
	Mode() FSMode
	Configure(json.RawMessage) (interface{}, error)
	Update(chan Node) error
}
