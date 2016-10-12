package krpc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/anacrolix/torrent/bencode"
)

type CompactIPv4NodeInfo []NodeInfo

var _ bencode.Unmarshaler = &CompactIPv4NodeInfo{}

func (i *CompactIPv4NodeInfo) UnmarshalBencode(_b []byte) (err error) {
	var b []byte
	err = bencode.Unmarshal(_b, &b)
	if err != nil {
		return
	}
	if len(b)%CompactIPv4NodeInfoLen != 0 {
		err = fmt.Errorf("bad length: %d", len(b))
		return
	}
	for k := 0; k < len(b); k += CompactIPv4NodeInfoLen {
		var ni NodeInfo
		err = ni.UnmarshalCompactIPv4(b[k : k+CompactIPv4NodeInfoLen])
		if err != nil {
			return
		}
		*i = append(*i, ni)
	}
	return
}

func (i CompactIPv4NodeInfo) MarshalBencode() (ret []byte, err error) {
	var buf bytes.Buffer
	for _, ni := range i {
		buf.Write(ni.ID[:])
		if ni.Addr == nil {
			err = errors.New("nil addr in node info")
			return
		}
		buf.Write(ni.Addr.IP.To4())
		binary.Write(&buf, binary.BigEndian, uint16(ni.Addr.Port))
	}
	return bencode.Marshal(buf.Bytes())
}
