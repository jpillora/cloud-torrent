package engine

import "github.com/jpillora/cloud-torrent/ct/shared"
import "github.com/jpillora/cloud-torrent/ct/engines/native"

type ID string

//the common engine interface
type Engine interface {
	Name() string           //name (lower(name)->id)
	NewConfig() interface{} //*Config object
	SetConfig(interface{}) error
	NewTorrent(magnetURI string) error
	StartTorrent(infohash string) error
	StopTorrent(infohash string) error
	DeleteTorrent(infohash string) error
	StartFile(infohash, filepath string) error
	StopFile(infohash, filepath string) error
	GetTorrents() <-chan *shared.Torrent
}

//TODO engines which require polling
type EnginePoller interface {
	//Polls the status of all torrents and all files, passes updated torrents
	//down the Torrents channel
	Poll() error
}

//insert each of the cloud-torrent bundled engines
var Bundled = []Engine{
	&native.Native{},
}
