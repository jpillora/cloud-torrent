package storage

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/missinggo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/torrent/metainfo"
)

func TestShortFile(t *testing.T) {
	td, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(td)
	s := NewFile(td)
	info := &metainfo.InfoEx{
		Info: metainfo.Info{
			Name:        "a",
			Length:      2,
			PieceLength: missinggo.MiB,
		},
	}
	ts, err := s.OpenTorrent(info)
	assert.NoError(t, err)
	f, err := os.Create(filepath.Join(td, "a"))
	err = f.Truncate(1)
	f.Close()
	var buf bytes.Buffer
	p := info.Piece(0)
	n, err := io.Copy(&buf, io.NewSectionReader(ts.Piece(p), 0, p.Length()))
	assert.EqualValues(t, 1, n)
	assert.Equal(t, io.ErrUnexpectedEOF, err)
}
