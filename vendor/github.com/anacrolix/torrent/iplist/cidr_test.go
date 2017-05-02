package iplist

import (
	"bytes"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPNetLast(t *testing.T) {
	_, in, err := net.ParseCIDR("138.255.252.0/22")
	require.NoError(t, err)
	assert.EqualValues(t, []byte{138, 255, 252, 0}, in.IP)
	assert.EqualValues(t, []byte{255, 255, 252, 0}, in.Mask)
	assert.EqualValues(t, []byte{138, 255, 255, 255}, IPNetLast(in))
	_, in, err = net.ParseCIDR("2400:cb00::/31")
	require.NoError(t, err)
	assert.EqualValues(t, []byte{0x24, 0, 0xcb, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, in.IP)
	assert.EqualValues(t, []byte{255, 255, 255, 254, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, in.Mask)
	assert.EqualValues(t, []byte{0x24, 0, 0xcb, 1, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, IPNetLast(in))
}

func TestParseCIDRList(t *testing.T) {
	r := bytes.NewBufferString(`2400:cb00::/32
2405:8100::/32
2405:b500::/32
2606:4700::/32
2803:f800::/32
2c0f:f248::/32
2a06:98c0::/29
`)
	rs, err := ParseCIDRListReader(r)
	require.NoError(t, err)
	require.Len(t, rs, 7)
	assert.EqualValues(t, Range{
		First: net.IP{0x28, 3, 0xf8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		Last:  net.IP{0x28, 3, 0xf8, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	}, rs[4])
}
