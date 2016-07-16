package torrent

type Config struct {
	PeerID            string
	DownloadDirectory string
	EnableUpload      bool
	EnableSeeding     bool
	EnableEncryption  bool
	AutoStart         bool
	IncomingPort      int
	MediaSort         MediaSortConfig `json:"mediaSort"`
}

type MediaSortConfig struct {
	Enable           bool
	AutoStartMatched bool
	TVDir            string
	MovieDir         string
}
