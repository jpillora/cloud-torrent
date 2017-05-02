package storage

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/torrent/metainfo"
)

func TestBoltPieceCompletion(t *testing.T) {
	td, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(td)

	pc, err := newBoltPieceCompletion(td)
	require.NoError(t, err)
	defer pc.Close()

	pk := metainfo.PieceKey{}

	b, err := pc.Get(pk)
	require.NoError(t, err)
	assert.False(t, b)

	require.NoError(t, pc.Set(pk, false))

	b, err = pc.Get(pk)
	require.NoError(t, err)
	assert.False(t, b)

	require.NoError(t, pc.Set(pk, true))

	b, err = pc.Get(pk)
	require.NoError(t, err)
	assert.True(t, b)
}
