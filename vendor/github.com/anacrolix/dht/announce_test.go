package dht

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnnounceNoStartingNodes(t *testing.T) {
	s, err := NewServer(&ServerConfig{
		NoDefaultBootstrap: true,
	})
	require.NoError(t, err)
	defer s.Close()
	var ih [20]byte
	copy(ih[:], "blah")
	_, err = s.Announce(ih, 0, true)
	require.EqualError(t, err, "server has no starting nodes")
}
