package storage

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anacrolix/torrent/metainfo"
)

func testMarkedCompleteMissingOnRead(t *testing.T, csf func(string) ClientImpl) {
	td, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(td)
	cs := NewClient(csf(td))
	info := &metainfo.Info{
		PieceLength: 1,
		Files:       []metainfo.FileInfo{{Path: []string{"a"}, Length: 1}},
	}
	ts, err := cs.OpenTorrent(info, metainfo.Hash{})
	require.NoError(t, err)
	p := ts.Piece(info.Piece(0))
	require.NoError(t, p.MarkComplete())
	// require.False(t, p.GetIsComplete())
	n, err := p.ReadAt(make([]byte, 1), 0)
	require.Error(t, err)
	require.EqualValues(t, 0, n)
	require.False(t, p.GetIsComplete())
}

func TestMarkedCompleteMissingOnReadFile(t *testing.T) {
	testMarkedCompleteMissingOnRead(t, NewFile)
}

func TestMarkedCompleteMissingOnReadFileBoltDB(t *testing.T) {
	testMarkedCompleteMissingOnRead(t, NewBoltDB)
}
