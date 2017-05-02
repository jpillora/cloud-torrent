package torrent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/missinggo"
	"github.com/bradfitz/iter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/internal/testutil"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/peer_protocol"
	"github.com/anacrolix/torrent/storage"
)

func r(i, b, l peer_protocol.Integer) request {
	return request{i, chunkSpec{b, l}}
}

// Check the given Request is correct for various torrent offsets.
func TestTorrentRequest(t *testing.T) {
	const s = 472183431 // Length of torrent.
	for _, _case := range []struct {
		off int64   // An offset into the torrent.
		req request // The expected Request. The zero value means !ok.
	}{
		// Invalid offset.
		{-1, request{}},
		{0, r(0, 0, 16384)},
		// One before the end of a piece.
		{1<<18 - 1, r(0, 1<<18-16384, 16384)},
		// Offset beyond torrent length.
		{472 * 1 << 20, request{}},
		// One before the end of the torrent. Complicates the chunk length.
		{s - 1, r((s-1)/(1<<18), (s-1)%(1<<18)/(16384)*(16384), 12935)},
		{1, r(0, 0, 16384)},
		// One before end of chunk.
		{16383, r(0, 0, 16384)},
		// Second chunk.
		{16384, r(0, 16384, 16384)},
	} {
		req, ok := torrentOffsetRequest(472183431, 1<<18, 16384, _case.off)
		if (_case.req == request{}) == ok {
			t.Fatalf("expected %v, got %v", _case.req, req)
		}
		if req != _case.req {
			t.Fatalf("expected %v, got %v", _case.req, req)
		}
	}
}

func TestAppendToCopySlice(t *testing.T) {
	orig := []int{1, 2, 3}
	dupe := append([]int{}, orig...)
	dupe[0] = 4
	if orig[0] != 1 {
		t.FailNow()
	}
}

func TestTorrentString(t *testing.T) {
	tor := &Torrent{}
	s := tor.InfoHash().HexString()
	if s != "0000000000000000000000000000000000000000" {
		t.FailNow()
	}
}

// This benchmark is from the observation that a lot of overlapping Readers on
// a large torrent with small pieces had a lot of overhead in recalculating
// piece priorities everytime a reader (possibly in another Torrent) changed.
func BenchmarkUpdatePiecePriorities(b *testing.B) {
	cl := &Client{}
	t := cl.newTorrent(metainfo.Hash{}, nil)
	t.info = &metainfo.Info{
		Pieces:      make([]byte, 20*13410),
		PieceLength: 256 << 10,
	}
	t.makePieces()
	assert.EqualValues(b, 13410, t.numPieces())
	for range iter.N(7) {
		r := t.NewReader()
		r.SetReadahead(32 << 20)
		r.Seek(3500000, 0)
	}
	assert.Len(b, t.readers, 7)
	t.pendPieceRange(0, t.numPieces())
	for i := 0; i < t.numPieces(); i += 3 {
		t.completedPieces.Set(i, true)
	}
	for range iter.N(b.N) {
		t.updateAllPiecePriorities()
	}
}

// Check that a torrent containing zero-length file(s) will start, and that
// they're created in the filesystem. The client storage is assumed to be
// file-based on the native filesystem based.
func testEmptyFilesAndZeroPieceLength(t *testing.T, cs storage.ClientImpl) {
	cfg := TestingConfig
	cfg.DefaultStorage = cs
	cl, err := NewClient(&TestingConfig)
	require.NoError(t, err)
	defer cl.Close()
	ib, err := bencode.Marshal(metainfo.Info{
		Name:        "empty",
		Length:      0,
		PieceLength: 0,
	})
	require.NoError(t, err)
	fp := filepath.Join(TestingConfig.DataDir, "empty")
	os.Remove(fp)
	assert.False(t, missinggo.FilePathExists(fp))
	tt, err := cl.AddTorrent(&metainfo.MetaInfo{
		InfoBytes: ib,
	})
	require.NoError(t, err)
	defer tt.Drop()
	tt.DownloadAll()
	require.True(t, cl.WaitAll())
	assert.True(t, missinggo.FilePathExists(fp))
}

func TestEmptyFilesAndZeroPieceLengthWithFileStorage(t *testing.T) {
	testEmptyFilesAndZeroPieceLength(t, storage.NewFile(TestingConfig.DataDir))
}

func TestEmptyFilesAndZeroPieceLengthWithMMapStorage(t *testing.T) {
	testEmptyFilesAndZeroPieceLength(t, storage.NewMMap(TestingConfig.DataDir))
}

func TestPieceHashFailed(t *testing.T) {
	mi := testutil.GreetingMetaInfo()
	tt := Torrent{
		cl:            new(Client),
		infoHash:      mi.HashInfoBytes(),
		storageOpener: storage.NewClient(badStorage{}),
		chunkSize:     2,
	}
	require.NoError(t, tt.setInfoBytes(mi.InfoBytes))
	tt.cl.mu.Lock()
	tt.pieces[1].DirtyChunks.AddRange(0, 3)
	require.True(t, tt.pieceAllDirty(1))
	tt.pieceHashed(1, false)
	// Dirty chunks should be cleared so we can try again.
	require.False(t, tt.pieceAllDirty(1))
	tt.cl.mu.Unlock()
}
