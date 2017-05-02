package metainfo

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/anacrolix/missinggo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/torrent/bencode"
)

func testFile(t *testing.T, filename string) {
	mi, err := LoadFromFile(filename)
	require.NoError(t, err)
	info, err := mi.UnmarshalInfo()
	require.NoError(t, err)

	if len(info.Files) == 1 {
		t.Logf("Single file: %s (length: %d)\n", info.Name, info.Files[0].Length)
	} else {
		t.Logf("Multiple files: %s\n", info.Name)
		for _, f := range info.Files {
			t.Logf(" - %s (length: %d)\n", path.Join(f.Path...), f.Length)
		}
	}

	for _, group := range mi.AnnounceList {
		for _, tracker := range group {
			t.Logf("Tracker: %s\n", tracker)
		}
	}

	b, err := bencode.Marshal(&info)
	require.NoError(t, err)
	assert.EqualValues(t, string(b), string(mi.InfoBytes))
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

func touchFile(path string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	err = f.Close()
	return
}

func TestBuildFromFilePathOrder(t *testing.T) {
	td, err := ioutil.TempDir("", "anacrolix")
	require.NoError(t, err)
	defer os.RemoveAll(td)
	require.NoError(t, touchFile(filepath.Join(td, "b")))
	require.NoError(t, touchFile(filepath.Join(td, "a")))
	info := Info{
		PieceLength: 1,
	}
	require.NoError(t, info.BuildFromFilePath(td))
	assert.EqualValues(t, []FileInfo{{
		Path: []string{"a"},
	}, {
		Path: []string{"b"},
	}}, info.Files)
}

func testUnmarshal(t *testing.T, input string, expected *MetaInfo) {
	var actual MetaInfo
	err := bencode.Unmarshal([]byte(input), &actual)
	if expected == nil {
		assert.Error(t, err)
		return
	}
	assert.NoError(t, err)
	assert.EqualValues(t, *expected, actual)
}

func TestUnmarshal(t *testing.T) {
	testUnmarshal(t, `de`, &MetaInfo{})
	testUnmarshal(t, `d4:infoe`, &MetaInfo{})
	testUnmarshal(t, `d4:infoabce`, nil)
	testUnmarshal(t, `d4:infodee`, &MetaInfo{InfoBytes: []byte("de")})
}
