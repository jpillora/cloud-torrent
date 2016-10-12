package dht

import (
	_ "crypto/sha1"
	"errors"
	"math/big"
	"math/rand"
	"net"
	"strconv"
	"time"

	"github.com/anacrolix/torrent/dht/krpc"
	"github.com/anacrolix/torrent/iplist"
)

const (
	maxNodes = 320
)

var (
	queryResendEvery = 5 * time.Second
)

var maxDistance big.Int

func init() {
	var zero big.Int
	maxDistance.SetBit(&zero, 160, 1)
}

// Uniquely identifies a transaction to us.
type transactionKey struct {
	RemoteAddr string // host:port
	T          string // The KRPC transaction ID.
}

// ServerConfig allows to set up a  configuration of the `Server` instance
// to be created with NewServer
type ServerConfig struct {
	// Listen address. Used if Conn is nil.
	Addr string

	// Set NodeId Manually. Caller must ensure that, if NodeId does not
	// conform to DHT Security Extensions, that NoSecurity is also set. This
	// should be given as a HEX string.
	NodeIdHex string

	Conn net.PacketConn
	// Don't respond to queries from other nodes.
	Passive bool
	// DHT Bootstrap nodes
	BootstrapNodes []string
	// Disable bootstrapping from global servers even if given no BootstrapNodes.
	// This creates a solitary node that awaits other nodes; it's only useful if
	// you're creating your own DHT and want to avoid accidental crossover, without
	// spoofing a bootstrap node and filling your logs with connection errors.
	NoDefaultBootstrap bool

	// Disable the DHT security extension:
	// http://www.libtorrent.org/dht_sec.html.
	NoSecurity bool
	// Initial IP blocklist to use. Applied before serving and bootstrapping
	// begins.
	IPBlocklist iplist.Ranger
	// Used to secure the server's ID. Defaults to the Conn's LocalAddr().
	PublicIP net.IP

	OnQuery func(*krpc.Msg, net.Addr) bool
}

// ServerStats instance is returned by Server.Stats() and stores Server metrics
type ServerStats struct {
	// Count of nodes in the node table that responded to our last query or
	// haven't yet been queried.
	GoodNodes int
	// Count of nodes in the node table.
	Nodes int
	// Transactions awaiting a response.
	OutstandingTransactions int
	// Individual announce_peer requests that got a success response.
	ConfirmedAnnounces int
	// Nodes that have been blocked.
	BadNodes uint
}

func makeSocket(addr string) (socket *net.UDPConn, err error) {
	addr_, err := net.ResolveUDPAddr("", addr)
	if err != nil {
		return
	}
	socket, err = net.ListenUDP("udp", addr_)
	return
}

type nodeID struct {
	i   big.Int
	set bool
}

func (nid *nodeID) IsUnset() bool {
	return !nid.set
}

func nodeIDFromString(s string) (ret nodeID) {
	if s == "" {
		return
	}
	ret.i.SetBytes([]byte(s))
	ret.set = true
	return
}

func (nid0 *nodeID) Distance(nid1 *nodeID) (ret big.Int) {
	if nid0.IsUnset() != nid1.IsUnset() {
		ret = maxDistance
		return
	}
	ret.Xor(&nid0.i, &nid1.i)
	return
}

func (nid *nodeID) ByteString() string {
	var buf [20]byte
	b := nid.i.Bytes()
	copy(buf[20-len(b):], b)
	return string(buf[:])
}

type node struct {
	addr          Addr
	id            nodeID
	announceToken string

	lastGotQuery    time.Time
	lastGotResponse time.Time
	lastSentQuery   time.Time
}

func (n *node) IsSecure() bool {
	if n.id.IsUnset() {
		return false
	}
	return NodeIdSecure(n.id.ByteString(), n.addr.UDPAddr().IP)
}

func (n *node) idString() string {
	return n.id.ByteString()
}

func (n *node) SetIDFromBytes(b []byte) {
	if len(b) != 20 {
		panic(b)
	}
	n.id.i.SetBytes(b)
	n.id.set = true
}

func (n *node) SetIDFromString(s string) {
	n.SetIDFromBytes([]byte(s))
}

func (n *node) IDNotSet() bool {
	return n.id.i.Int64() == 0
}

func (n *node) NodeInfo() (ret krpc.NodeInfo) {
	ret.Addr = n.addr.UDPAddr()
	if n := copy(ret.ID[:], n.idString()); n != 20 {
		panic(n)
	}
	return
}

func (n *node) DefinitelyGood() bool {
	if len(n.idString()) != 20 {
		return false
	}
	// No reason to think ill of them if they've never been queried.
	if n.lastSentQuery.IsZero() {
		return true
	}
	// They answered our last query.
	if n.lastSentQuery.Before(n.lastGotResponse) {
		return true
	}
	return true
}

func jitterDuration(average time.Duration, plusMinus time.Duration) time.Duration {
	return average - plusMinus/2 + time.Duration(rand.Int63n(int64(plusMinus)))
}

type Peer struct {
	IP   net.IP
	Port int
}

func (p *Peer) String() string {
	return net.JoinHostPort(p.IP.String(), strconv.FormatInt(int64(p.Port), 10))
}

func bootstrapAddrs(nodeAddrs []string) (addrs []*net.UDPAddr, err error) {
	bootstrapNodes := nodeAddrs
	if len(bootstrapNodes) == 0 {
		bootstrapNodes = []string{
			"router.utorrent.com:6881",
			"router.bittorrent.com:6881",
		}
	}
	for _, addrStr := range bootstrapNodes {
		udpAddr, err := net.ResolveUDPAddr("udp4", addrStr)
		if err != nil {
			continue
		}
		addrs = append(addrs, udpAddr)
	}
	if len(addrs) == 0 {
		err = errors.New("nothing resolved")
	}
	return
}
