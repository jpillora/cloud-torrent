package krpc

import (
	"encoding/binary"
	"errors"
	"net"

	"github.com/anacrolix/missinggo"
)

// The size in bytes of a NodeInfo in its compact binary representation.
const CompactIPv4NodeInfoLen = 26

type NodeInfo struct {
	ID   [20]byte
	Addr *net.UDPAddr
}

// Writes the node info to its compact binary representation in b. See
// CompactNodeInfoLen.
func (ni *NodeInfo) PutCompact(b []byte) error {
	if n := copy(b[:], ni.ID[:]); n != 20 {
		panic(n)
	}
	ip := ni.Addr.IP.To4()
	if len(ip) != 4 {
		return errors.New("expected ipv4 address")
	}
	if n := copy(b[20:], ip); n != 4 {
		panic(n)
	}
	binary.BigEndian.PutUint16(b[24:], uint16(ni.Addr.Port))
	return nil
}

func (ni *NodeInfo) UnmarshalCompactIPv4(b []byte) error {
	if len(b) != CompactIPv4NodeInfoLen {
		return errors.New("expected 26 bytes")
	}
	missinggo.CopyExact(ni.ID[:], b[:20])
	ni.Addr = &net.UDPAddr{
		IP:   append(make([]byte, 0, 4), b[20:24]...),
		Port: int(binary.BigEndian.Uint16(b[24:26])),
	}
	return nil
}
