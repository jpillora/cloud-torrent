package ctserver

//the common engine interface
type Engine interface {
	Name() string
	Configure(interface{}) error
	StartMagnet(string) error
	StartTorrent([]byte) error
	List() ([]*Torrent, error) //get a list of all Torrents
	Fetch(*Torrent) error      //fetch the Files of a particular Torrent
}

var bundledEngines = []Engine{}
