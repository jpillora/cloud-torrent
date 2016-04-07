package dht

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/util"
)

func testMarshalUnmarshalMsg(t *testing.T, m Msg, expected string) {
	b, err := bencode.Marshal(m)
	require.NoError(t, err)
	assert.Equal(t, expected, string(b))
	var _m Msg
	err = bencode.Unmarshal([]byte(expected), &_m)
	assert.NoError(t, err)
	assert.EqualValues(t, m, _m)
	assert.EqualValues(t, m.A, _m.A)
	assert.EqualValues(t, m.R, _m.R)
}

func TestMarshalUnmarshalMsg(t *testing.T) {
	testMarshalUnmarshalMsg(t, Msg{}, "d1:t0:1:y0:e")
	testMarshalUnmarshalMsg(t, Msg{
		Y: "q",
		Q: "ping",
		T: "hi",
	}, "d1:q4:ping1:t2:hi1:y1:qe")
	testMarshalUnmarshalMsg(t, Msg{
		Y: "e",
		T: "42",
		E: &KRPCError{Code: 200, Msg: "fuck"},
	}, "d1:eli200e4:fucke1:t2:421:y1:ee")
	testMarshalUnmarshalMsg(t, Msg{
		Y: "r",
		T: "\x8c%",
		R: &Return{},
	}, "d1:rd2:id0:e1:t2:\x8c%1:y1:re")
	testMarshalUnmarshalMsg(t, Msg{
		Y: "r",
		T: "\x8c%",
		R: &Return{
			Nodes: CompactIPv4NodeInfo{
				NodeInfo{
					Addr: NewAddr(&net.UDPAddr{
						IP:   net.IPv4(1, 2, 3, 4).To4(),
						Port: 0x1234,
					}),
				},
			},
		},
	}, "d1:rd2:id0:5:nodes26:\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x01\x02\x03\x04\x124e1:t2:\x8c%1:y1:re")
	testMarshalUnmarshalMsg(t, Msg{
		Y: "r",
		T: "\x8c%",
		R: &Return{
			Values: []util.CompactPeer{
				util.CompactPeer{
					IP:   net.IPv4(1, 2, 3, 4).To4(),
					Port: 0x5678,
				},
			},
		},
	}, "d1:rd2:id0:6:valuesl6:\x01\x02\x03\x04\x56\x78ee1:t2:\x8c%1:y1:re")
	testMarshalUnmarshalMsg(t, Msg{
		Y: "r",
		T: "\x03",
		R: &Return{
			ID: "\xeb\xff6isQ\xffJ\xec)ͺ\xab\xf2\xfb\xe3F|\xc2g",
		},
		IP: util.CompactPeer{
			IP:   net.IPv4(124, 168, 180, 8).To4(),
			Port: 62844,
		},
	}, "d2:ip6:|\xa8\xb4\b\xf5|1:rd2:id20:\xeb\xff6isQ\xffJ\xec)ͺ\xab\xf2\xfb\xe3F|\xc2ge1:t1:\x031:y1:re")
}

func TestUnmarshalGetPeersResponse(t *testing.T) {
	var msg Msg
	err := bencode.Unmarshal([]byte("d1:rd6:valuesl6:\x01\x02\x03\x04\x05\x066:\x07\x08\x09\x0a\x0b\x0ce5:nodes52:\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x02\x03\x04\x05\x06\x07\x08\x09\x02\x03\x04\x05\x06\x07\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x02\x03\x04\x05\x06\x07\x08\x09\x02\x03\x04\x05\x06\x07ee"), &msg)
	require.NoError(t, err)
	assert.Len(t, msg.R.Values, 2)
	assert.Len(t, msg.R.Nodes, 2)
	assert.Nil(t, msg.E)
}
