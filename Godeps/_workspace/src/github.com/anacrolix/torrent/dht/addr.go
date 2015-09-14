package dht

import (
	"net"

	"github.com/anacrolix/missinggo"
)

// Used internally to refer to node network addresses.
type dHTAddr interface {
	net.Addr
	UDPAddr() *net.UDPAddr
	IP() net.IP
}

// Speeds up some of the commonly called Addr methods.
type cachedAddr struct {
	a  net.Addr
	s  string
	ip net.IP
}

func (ca cachedAddr) Network() string {
	return ca.a.Network()
}

func (ca cachedAddr) String() string {
	return ca.s
}

func (ca cachedAddr) UDPAddr() *net.UDPAddr {
	return ca.a.(*net.UDPAddr)
}

func (ca cachedAddr) IP() net.IP {
	return ca.ip
}

func newDHTAddr(addr net.Addr) dHTAddr {
	return cachedAddr{addr, addr.String(), missinggo.AddrIP(addr)}
}
