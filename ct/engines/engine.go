package engine

import "github.com/jpillora/cloud-torrent/ct/shared"
import "github.com/jpillora/cloud-torrent/ct/engines/native"

type ID string

//the common engine interface
type Engine interface {
	Name() string           //name (lower(name)->id)
	NewConfig() interface{} //*Config object
	SetConfig(interface{}) error
	Magnet(uri string) error
	Torrents() <-chan *shared.Torrent
}

//TODO engines which require polling
type EnginePoller interface {
	Poll() error
	PollTorrent(*shared.Torrent) error
}

//insert each of the cloud-torrent bundled engines
var Bundled = []Engine{
	&native.Native{},
}
