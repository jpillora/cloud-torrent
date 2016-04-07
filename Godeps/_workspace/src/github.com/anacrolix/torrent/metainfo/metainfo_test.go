package metainfo

import (
	"io"
	"io/ioutil"
	"path"
	"testing"

	"github.com/anacrolix/missinggo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/torrent/bencode"
)

func testFile(t *testing.T, filename string) {
	mi, err := LoadFromFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	if len(mi.Info.Files) == 1 {
		t.Logf("Single file: %s (length: %d)\n", mi.Info.Name, mi.Info.Files[0].Length)
	} else {
		t.Logf("Multiple files: %s\n", mi.Info.Name)
		for _, f := range mi.Info.Files {
			t.Logf(" - %s (length: %d)\n", path.Join(f.Path...), f.Length)
		}
	}

	for _, group := range mi.AnnounceList {
		for _, tracker := range group {
			t.Logf("Tracker: %s\n", tracker)
		}
	}
	// for _, url := range mi.WebSeedURLs {
	// 	t.Logf("URL: %s\n", url)
	// }

	b, err := bencode.Marshal(mi.Info)
	require.NoError(t, err)
	assert.EqualValues(t, b, mi.Info.Bytes)
}

func TestFile(t *testing.T) {
	testFile(t, "testdata/archlinux-2011.08.19-netinstall-i686.iso.torrent")
	testFile(t, "testdata/continuum.torrent")
	testFile(t, "testdata/23516C72685E8DB0C8F15553382A927F185C4F01.torrent")
	testFile(t, "testdata/trackerless.torrent")
}

// Ensure that the correct number of pieces are generated when hashing files.
func TestNumPieces(t *testing.T) {
	for _, _case := range []struct {
		PieceLength int64
		Files       []FileInfo
		NumPieces   int
	}{
		{256 * 1024, []FileInfo{{Length: 1024*1024 + -1}}, 4},
		{256 * 1024, []FileInfo{{Length: 1024 * 1024}}, 4},
		{256 * 1024, []FileInfo{{Length: 1024*1024 + 1}}, 5},
		{5, []FileInfo{{Length: 1}, {Length: 12}}, 3},
		{5, []FileInfo{{Length: 4}, {Length: 12}}, 4},
	} {
		info := Info{
			Files:       _case.Files,
			PieceLength: _case.PieceLength,
		}
		err := info.GeneratePieces(func(fi FileInfo) (io.ReadCloser, error) {
			return ioutil.NopCloser(missinggo.ZeroReader), nil
		})
		assert.NoError(t, err)
		assert.EqualValues(t, _case.NumPieces, info.NumPieces())
	}
}
