package torrent

import (
	"net"

	"github.com/anacrolix/dht/krpc"
)

type Peer struct {
	Id     [20]byte
	IP     net.IP
	Port   int
	Source peerSource
	// Peer is known to support encryption.
	SupportsEncryption bool
	pexPeerFlags
}

func (me *Peer) FromPex(na krpc.NodeAddr, fs pexPeerFlags) {
	me.IP = append([]byte(nil), na.IP...)
	me.Port = na.Port
	me.Source = peerSourcePEX
	// If they prefer encryption, they must support it.
	if fs.Get(pexPrefersEncryption) {
		me.SupportsEncryption = true
	}
	me.pexPeerFlags = fs
}

func (me Peer) addr() ipPort {
	return ipPort{me.IP, uint16(me.Port)}
}
