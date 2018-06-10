package torrent

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"

	"github.com/anacrolix/dht"
	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/expect"
	"github.com/anacrolix/torrent/iplist"
	"github.com/anacrolix/torrent/storage"
)

var DefaultHTTPClient = &http.Client{
	Timeout: time.Second * 15,
	Transport: &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 15 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 15 * time.Second,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
	},
}
var DefaultHTTPUserAgent = "Go-Torrent/1.0"

// Override Client defaults.
type Config struct {
	// Store torrent file data in this directory unless .DefaultStorage is
	// specified.
	DataDir string `long:"data-dir" description:"directory to store downloaded torrent data"`
	// The address to listen for new uTP and TCP bittorrent protocol
	// connections. DHT shares a UDP socket with uTP unless configured
	// otherwise.
	ListenHost              func(network string) string
	ListenPort              int
	NoDefaultPortForwarding bool
	// Don't announce to trackers. This only leaves DHT to discover peers.
	DisableTrackers bool `long:"disable-trackers"`
	DisablePEX      bool `long:"disable-pex"`

	// Don't create a DHT.
	NoDHT            bool `long:"disable-dht"`
	DhtStartingNodes dht.StartingNodesGetter
	// Never send chunks to peers.
	NoUpload bool `long:"no-upload"`
	// Disable uploading even when it isn't fair.
	DisableAggressiveUpload bool `long:"disable-aggressive-upload"`
	// Upload even after there's nothing in it for us. By default uploading is
	// not altruistic, we'll upload slightly more than we download from each
	// peer.
	Seed bool `long:"seed"`
	// Only applies to chunks uploaded to peers, to maintain responsiveness
	// communicating local Client state to peers. Each limiter token
	// represents one byte. The Limiter's burst must be large enough to fit a
	// whole chunk, which is usually 16 KiB (see TorrentSpec.ChunkSize).
	UploadRateLimiter *rate.Limiter
	// Rate limits all reads from connections to peers. Each limiter token
	// represents one byte. The Limiter's burst must be bigger than the
	// largest Read performed on a the underlying rate-limiting io.Reader
	// minus one. This is likely to be the larger of the main read loop buffer
	// (~4096), and the requested chunk size (~16KiB, see
	// TorrentSpec.ChunkSize).
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

	EncryptionPolicy

	// Sets usage of Socks5 Proxy. Authentication should be included in the url if needed.
	// Example of setting: "socks5://demo:demo@192.168.99.100:1080"
	ProxyURL string

	IPBlocklist      iplist.Ranger
	DisableIPv6      bool `long:"disable-ipv6"`
	DisableIPv4      bool
	DisableIPv4Peers bool
	// Perform logging and any other behaviour that will help debug.
	Debug bool `help:"enable debugging"`

	// HTTP client used to query the tracker endpoint. Default is DefaultHTTPClient
	HTTP *http.Client
	// HTTPUserAgent changes default UserAgent for HTTP requests
	HTTPUserAgent string `long:"http-user-agent"`
	// Updated occasionally to when there's been some changes to client
	// behaviour in case other clients are assuming anything of us. See also
	// `bep20`.
	ExtendedHandshakeClientVersion string // default  "go.torrent dev 20150624"
	// Peer ID client identifier prefix. We'll update this occasionally to
	// reflect changes to client behaviour that other clients may depend on.
	// Also see `extendedHandshakeClientVersion`.
	Bep20 string // default "-GT0001-"

	NominalDialTimeout         time.Duration // default  time.Second * 30
	MinDialTimeout             time.Duration // default  5 * time.Second
	EstablishedConnsPerTorrent int           // default 80
	HalfOpenConnsPerTorrent    int           // default  80
	TorrentPeersHighWater      int           // default 200
	TorrentPeersLowWater       int           // default 50

	// Limit how long handshake can take. This is to reduce the lingering
	// impact of a few bad apples. 4s loses 1% of successful handshakes that
	// are obtained with 60s timeout, and 5% of unsuccessful handshakes.
	HandshakesTimeout time.Duration // default  20 * time.Second

	PublicIp4 net.IP
	PublicIp6 net.IP
}

func (cfg *Config) SetListenAddr(addr string) *Config {
	host, port, err := missinggo.ParseHostPort(addr)
	expect.Nil(err)
	cfg.ListenHost = func(string) string { return host }
	cfg.ListenPort = port
	return cfg
}

func (cfg *Config) setDefaults() {
	if cfg.HTTP == nil {
		cfg.HTTP = DefaultHTTPClient
		if cfg.ProxyURL != "" {
			cfg.setProxyURL()
		}
	}
	if cfg.HTTPUserAgent == "" {
		cfg.HTTPUserAgent = DefaultHTTPUserAgent
	}
	if cfg.ExtendedHandshakeClientVersion == "" {
		cfg.ExtendedHandshakeClientVersion = "go.torrent dev 20150624"
	}
	if cfg.Bep20 == "" {
		cfg.Bep20 = "-GT0001-"
	}
	if cfg.NominalDialTimeout == 0 {
		cfg.NominalDialTimeout = 30 * time.Second
	}
	if cfg.MinDialTimeout == 0 {
		cfg.MinDialTimeout = 5 * time.Second
	}
	if cfg.EstablishedConnsPerTorrent == 0 {
		cfg.EstablishedConnsPerTorrent = 50
	}
	if cfg.HalfOpenConnsPerTorrent == 0 {
		cfg.HalfOpenConnsPerTorrent = (cfg.EstablishedConnsPerTorrent + 1) / 2
	}
	if cfg.TorrentPeersHighWater == 0 {
		// Memory and freshness are the concern here.
		cfg.TorrentPeersHighWater = 500
	}
	if cfg.TorrentPeersLowWater == 0 {
		cfg.TorrentPeersLowWater = 2 * cfg.HalfOpenConnsPerTorrent
	}
	if cfg.HandshakesTimeout == 0 {
		cfg.HandshakesTimeout = 20 * time.Second
	}
	if cfg.DhtStartingNodes == nil {
		cfg.DhtStartingNodes = dht.GlobalBootstrapAddrs
	}
	if cfg.ListenHost == nil {
		cfg.ListenHost = func(string) string { return "" }
	}
}

func (cfg *Config) setProxyURL() {
	fixedURL, err := url.Parse(cfg.ProxyURL)
	if err != nil {
		return
	}

	cfg.HTTP.Transport = &http.Transport{
		Proxy:               http.ProxyURL(fixedURL),
		TLSHandshakeTimeout: 15 * time.Second,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
	}
}

type EncryptionPolicy struct {
	DisableEncryption  bool
	ForceEncryption    bool // Don't allow unobfuscated connections.
	PreferNoEncryption bool
}
