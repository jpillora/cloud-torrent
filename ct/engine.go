package ct

import "github.com/jpillora/cloud-torrent/ct/shared"
import "github.com/jpillora/cloud-torrent/ct/engines/native"

type engineID string

//the common engine interface
type Engine interface {
	Name() string           //name (lower(name)->id)
	GetConfig() interface{} //*Config object
	SetConfig() error
	Magnet(uri string) error
	List() ([]*shared.Torrent, error) //get a list of all Torrents
	Fetch(*shared.Torrent) error      //fetch the Files of a particular Torrent
}

//insert each of the cloud-torrent bundled engines
var bundledEngines = []Engine{
	&native.Native{},
}
