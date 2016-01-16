package engine

type Config struct {
	DownloadDirectory string
	EnableUpload      bool
	EnableSeeding     bool
	EnableEncryption  bool
	AutoStart         bool
	IncomingPort      int
}
