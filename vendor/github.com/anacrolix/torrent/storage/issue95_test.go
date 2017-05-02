package storage

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/anacrolix/missinggo/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/torrent/metainfo"
)

// Two different torrents opened from the same storage. Closing one should not
// break the piece completion on the other.
func testIssue95(t *testing.T, c ClientImpl) {
	i1 := &metainfo.Info{
		Files:  []metainfo.FileInfo{{Path: []string{"a"}}},
		Pieces: make([]byte, 20),
	}
	t1, err := c.OpenTorrent(i1, metainfo.HashBytes([]byte("a")))
	require.NoError(t, err)
	i2 := &metainfo.Info{
		Files:  []metainfo.FileInfo{{Path: []string{"a"}}},
		Pieces: make([]byte, 20),
	}
	t2, err := c.OpenTorrent(i2, metainfo.HashBytes([]byte("b")))
	require.NoError(t, err)
	t2p := t2.Piece(i2.Piece(0))
	assert.NoError(t, t1.Close())
	assert.NotPanics(t, func() { t2p.GetIsComplete() })
}

func TestIssue95File(t *testing.T) {
	td, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(td)
	testIssue95(t, NewFile(td))
}

func TestIssue95MMap(t *testing.T) {
	td, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(td)
	testIssue95(t, NewMMap(td))
}

func TestIssue95ResourcePieces(t *testing.T) {
	td, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(td)
	testIssue95(t, NewResourcePieces(resource.OSFileProvider{}))
}
