package tracker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/torrent/bencode"
)

func TestUnmarshalHTTPResponsePeerDicts(t *testing.T) {
	var hr httpResponse
	require.NoError(t, bencode.Unmarshal([]byte("d5:peersl"+
		"d2:ip7:1.2.3.47:peer id20:thisisthe20bytepeeri4:porti9999ee"+
		"d7:peer id20:thisisthe20bytepeeri2:ip39:2001:0db8:85a3:0000:0000:8a2e:0370:73344:porti9998ee"+
		"ee"), &hr))
	ps, err := hr.UnmarshalPeers()
	require.NoError(t, err)
	require.Len(t, ps, 2)
	assert.Equal(t, []byte("thisisthe20bytepeeri"), ps[0].ID)
	assert.EqualValues(t, 9999, ps[0].Port)
	assert.EqualValues(t, 9998, ps[1].Port)
	assert.NotNil(t, ps[0].IP)
	assert.NotNil(t, ps[1].IP)
}
