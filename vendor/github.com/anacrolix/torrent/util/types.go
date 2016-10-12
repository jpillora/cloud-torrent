package util

import (
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"github.com/bradfitz/iter"

	"github.com/anacrolix/torrent/bencode"
)

// Concatenated 6-byte peer addresses.
type CompactIPv4Peers []CompactPeer

var (
	// This allows bencode.Unmarshal to do better than a string or []byte.
	_ bencode.Unmarshaler      = &CompactIPv4Peers{}
	_ encoding.BinaryMarshaler = CompactIPv4Peers{}
)

// This allows bencode.Unmarshal to do better than a string or []byte.
func (cps *CompactIPv4Peers) UnmarshalBencode(b []byte) (err error) {
	var bb []byte
	err = bencode.Unmarshal(b, &bb)
	if err != nil {
		return
	}
	*cps, err = UnmarshalIPv4CompactPeers(bb)
	return
}

func (cps CompactIPv4Peers) MarshalBinary() (ret []byte, err error) {
	ret = make([]byte, len(cps)*6)
	for i, cp := range cps {
		copy(ret[6*i:], cp.IP.To4())
		binary.BigEndian.PutUint16(ret[6*i+4:], uint16(cp.Port))
	}
	return
}

// Represents peer address in either IPv6 or IPv4 form.
type CompactPeer struct {
	IP   net.IP
	Port int
}

var (
	_ bencode.Marshaler   = &CompactPeer{}
	_ bencode.Unmarshaler = &CompactPeer{}
)

func (cp CompactPeer) MarshalBencode() (ret []byte, err error) {
	ip := cp.IP
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	ret = make([]byte, len(ip)+2)
	copy(ret, ip)
	binary.BigEndian.PutUint16(ret[len(ip):], uint16(cp.Port))
	return bencode.Marshal(ret)
}

func (cp *CompactPeer) UnmarshalBinary(b []byte) error {
	switch len(b) {
	case 18:
		cp.IP = make([]byte, 16)
	case 6:
		cp.IP = make([]byte, 4)
	default:
		return fmt.Errorf("bad compact peer string: %q", b)
	}
	copy(cp.IP, b)
	b = b[len(cp.IP):]
	cp.Port = int(binary.BigEndian.Uint16(b))
	return nil
}

func (cp *CompactPeer) UnmarshalBencode(b []byte) (err error) {
	var _b []byte
	err = bencode.Unmarshal(b, &_b)
	if err != nil {
		return
	}
	return cp.UnmarshalBinary(_b)
}

func UnmarshalIPv4CompactPeers(b []byte) (ret []CompactPeer, err error) {
	if len(b)%6 != 0 {
		err = errors.New("bad length")
		return
	}
	num := len(b) / 6
	ret = make([]CompactPeer, num)
	for i := range iter.N(num) {
		off := i * 6
		err = ret[i].UnmarshalBinary(b[off : off+6])
		if err != nil {
			return
		}
	}
	return
}
