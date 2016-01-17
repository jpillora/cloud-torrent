package torrent

import (
	"github.com/anacrolix/torrent/dht"
	"github.com/anacrolix/torrent/iplist"
)

// Override Client defaults.
type Config struct {
	// Store torrent file data in this directory unless TorrentDataOpener is
	// specified.
	DataDir string `long:"data-dir" description:"directory to store downloaded torrent data"`
	// The address to listen for new uTP and TCP bittorrent protocol
	// connections. DHT shares a UDP socket with uTP unless configured
	// otherwise.
	ListenAddr string `long:"listen-addr" value-name:"HOST:PORT"`
	// Don't announce to trackers. This only leaves DHT to discover peers.
	DisableTrackers bool `long:"disable-trackers"`
	DisablePEX      bool `long:"disable-pex"`
	// Don't create a DHT.
	NoDHT bool `long:"disable-dht"`
	// Overrides the default DHT configuration.
	DHTConfig dht.ServerConfig
	// Don't ever send chunks to peers.
	NoUpload bool `long:"no-upload"`
	// Upload even after there's nothing in it for us. By default uploading is
	// not altruistic.
	Seed bool `long:"seed"`
	// User-provided Client peer ID. If not present, one is generated automatically.
	PeerID string
	// For the bittorrent protocol.
	DisableUTP bool
	// For the bittorrent protocol.
	DisableTCP bool `long:"disable-tcp"`
	// Don't automatically load "$ConfigDir/blocklist".
	NoDefaultBlocklist bool
	// Defaults to "$HOME/.config/torrent". This is where "blocklist",
	// "torrents" and other operational files are stored.
	ConfigDir string
	// Don't save or load to a cache of torrent files stored in
	// "$ConfigDir/torrents".
	DisableMetainfoCache bool
	// Called to instantiate storage for each added torrent. Provided backends
	// are in $REPO/data. If not set, the "file" implementation is used.
	TorrentDataOpener
	DisableEncryption bool `long:"disable-encryption"`

	IPBlocklist *iplist.IPList
	DisableIPv6 bool `long:"disable-ipv6"`
	// Perform logging and any other behaviour that will help debug.
	Debug bool `help:"enable debug logging"`
}
