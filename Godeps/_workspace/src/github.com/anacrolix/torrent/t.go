package torrent

import (
	"github.com/anacrolix/torrent/metainfo"
)

// The public interface for a torrent within a Client.

// A handle to a live torrent within a Client.
type Torrent struct {
	cl *Client
	*torrent
}

func (t Torrent) InfoHash() InfoHash {
	return t.torrent.InfoHash
}

// Closed when the info (.Info()) for the torrent has become available. Using
// features of Torrent that require the info before it is available will have
// undefined behaviour.
func (t *Torrent) GotInfo() <-chan struct{} {
	return t.torrent.gotMetainfo
}

func (t *Torrent) Info() *metainfo.Info {
	return t.torrent.Info
}

// Returns a Reader bound to the torrent's data. All read calls block until
// the data requested is actually available. Priorities are set to ensure the
// data requested will be downloaded as soon as possible.
func (t *Torrent) NewReader() (ret *Reader) {
	ret = &Reader{
		t:         t,
		readahead: 5 * 1024 * 1024,
	}
	return
}

// Returns the state of pieces of the torrent. They are grouped into runs of
// same state. The sum of the state run lengths is the number of pieces
// in the torrent.
func (t *Torrent) PieceStateRuns() []PieceStateRun {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	return t.torrent.pieceStateRuns()
}

func (t Torrent) NumPieces() int {
	return t.numPieces()
}

func (t Torrent) Drop() {
	t.cl.mu.Lock()
	t.cl.dropTorrent(t.torrent.InfoHash)
	t.cl.mu.Unlock()
}

func (t Torrent) BytesCompleted() int64 {
	t.cl.mu.RLock()
	defer t.cl.mu.RUnlock()
	return t.bytesCompleted()
}
