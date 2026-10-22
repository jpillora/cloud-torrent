package engine

// Config controls the torrent engine. It is serialised to cloud-torrent.json
// and synced to the browser as part of the application state.
type Config struct {
	AutoStart         bool
	DisableEncryption bool // require plaintext peer transport; false prefers encryption with plaintext fallback
	DownloadDirectory string
	EnableUpload      bool
	EnableSeeding     bool
	IncomingPort      int
}
