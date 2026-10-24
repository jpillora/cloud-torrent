package engine

// Config controls the torrent engine. It is serialised to cloud-torrent.json
// and synced to the browser as part of the application state.
type Config struct {
	SeedRatioLimit     float64 // 0 disables the ratio limit
	SeedTimeMinutes    int     // 0 disables the active seeding time limit
	MaxActiveDownloads int     // 0 means unlimited; completed selections do not consume slots
	MaxActiveMetadata  int     // 0 selects the bounded default of four
	DownloadLimitKiB   int64   // global KiB/s, 0 explicitly means unlimited
	UploadLimitKiB     int64   // global KiB/s, 0 explicitly means unlimited
	AutoStart          bool
	DisableEncryption  bool // require plaintext peer transport; false prefers encryption with plaintext fallback
	DownloadDirectory  string
	EnableUpload       bool
	EnableSeeding      bool
	IncomingPort       int
}
