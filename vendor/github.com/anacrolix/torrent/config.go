package torrent

import (
	"golang.org/x/time/rate"

	"github.com/anacrolix/torrent/dht"
	"github.com/anacrolix/torrent/iplist"
	"github.com/anacrolix/torrent/storage"
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

	// Never send chunks to peers.
	NoUpload bool `long:"no-upload"`
	// Upload even after there's nothing in it for us. By default uploading is
	// not altruistic, we'll upload slightly more than we download from each
	// peer.
	Seed bool `long:"seed"`
	// Events are data bytes sent in pieces. The burst must be large enough to
	// fit a whole chunk.
	UploadRateLimiter *rate.Limiter
	// The events are bytes read from connections. The burst must be bigger
	// than the largest Read performed on a Conn minus one. This is likely to
	// be the larger of the main read loop buffer (~4096), and the requested
	// chunk size (~16KiB).
	DownloadRateLimiter *rate.Limiter

	// User-provided Client peer ID. If not present, one is generated automatically.
	PeerID string
	// For the bittorrent protocol.
	DisableUTP bool
	// For the bittorrent protocol.
	DisableTCP bool `long:"disable-tcp"`
	// Called to instantiate storage for each added torrent. Builtin backends
	// are in the storage package. If not set, the "file" implementation is
	// used.
	DefaultStorage storage.ClientImpl

	DisableEncryption  bool `long:"disable-encryption"`
	ForceEncryption    bool // Don't allow unobfuscated connections.
	PreferNoEncryption bool

	IPBlocklist iplist.Ranger
	DisableIPv6 bool `long:"disable-ipv6"`
	// Perform logging and any other behaviour that will help debug.
	Debug bool `help:"enable debug logging"`
}
