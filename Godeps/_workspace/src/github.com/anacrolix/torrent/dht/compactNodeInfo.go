package dht

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/anacrolix/torrent/bencode"
)

type CompactIPv4NodeInfo []NodeInfo

var _ bencode.Unmarshaler = &CompactIPv4NodeInfo{}

func (me *CompactIPv4NodeInfo) UnmarshalBencode(_b []byte) (err error) {
	var b []byte
	err = bencode.Unmarshal(_b, &b)
	if err != nil {
		return
	}
	if len(b)%CompactIPv4NodeInfoLen != 0 {
		err = fmt.Errorf("bad length: %d", len(b))
		return
	}
	for i := 0; i < len(b); i += CompactIPv4NodeInfoLen {
		var ni NodeInfo
		err = ni.UnmarshalCompactIPv4(b[i : i+CompactIPv4NodeInfoLen])
		if err != nil {
			return
		}
		*me = append(*me, ni)
	}
	return
}

func (me CompactIPv4NodeInfo) MarshalBencode() (ret []byte, err error) {
	var buf bytes.Buffer
	for _, ni := range me {
		buf.Write(ni.ID[:])
		if ni.Addr == nil {
			err = errors.New("nil addr in node info")
			return
		}
		buf.Write(ni.Addr.UDPAddr().IP.To4())
		binary.Write(&buf, binary.BigEndian, uint16(ni.Addr.UDPAddr().Port))
	}
	return bencode.Marshal(buf.Bytes())
}
