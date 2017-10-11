package torrent

import (
	"strings"

	"github.com/anacrolix/missinggo/pubsub"

	"github.com/anacrolix/torrent/metainfo"
)

// The torrent's infohash. This is fixed and cannot change. It uniquely
// identifies a torrent.
func (t *Torrent) InfoHash() metainfo.Hash {
	return t.infoHash
}

// Returns a channel that is closed when the info (.Info()) for the torrent
// has become available.
func (t *Torrent) GotInfo() <-chan struct{} {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	return t.gotMetainfo.C()
}

// Returns the metainfo info dictionary, or nil if it's not yet available.
func (t *Torrent) Info() *metainfo.Info {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	return t.info
}

// Returns a Reader bound to the torrent's data. All read calls block until
// the data requested is actually available.
func (t *Torrent) NewReader() (ret *Reader) {
	ret = &Reader{
		mu:        &t.cl.mu,
		t:         t,
		readahead: 5 * 1024 * 1024,
	}
	t.addReader(ret)
	return
}

// Returns the state of pieces of the torrent. They are grouped into runs of
// same state. The sum of the state run lengths is the number of pieces
// in the torrent.
func (t *Torrent) PieceStateRuns() []PieceStateRun {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	return t.pieceStateRuns()
}

func (t *Torrent) PieceState(piece int) PieceState {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	return t.pieceState(piece)
}

// The number of pieces in the torrent. This requires that the info has been
// obtained first.
func (t *Torrent) NumPieces() int {
	return t.numPieces()
}

// Get missing bytes count for specific piece.
func (t *Torrent) PieceBytesMissing(piece int) int64 {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()

	return int64(t.pieces[piece].bytesLeft())
}

// Drop the torrent from the client, and close it. It's always safe to do
// this. No data corruption can, or should occur to either the torrent's data,
// or connected peers.
func (t *Torrent) Drop() {
	t.cl.mu.Lock()
	t.cl.dropTorrent(t.infoHash)
	t.cl.mu.Unlock()
}

// Number of bytes of the entire torrent we have completed.
func (t *Torrent) BytesCompleted() int64 {
	t.cl.mu.RLock()
	defer t.cl.mu.RUnlock()
	return t.bytesCompleted()
}

// The subscription emits as (int) the index of pieces as their state changes.
// A state change is when the PieceState for a piece alters in value.
func (t *Torrent) SubscribePieceStateChanges() *pubsub.Subscription {
	return t.pieceStateChanges.Subscribe()
}

// Returns true if the torrent is currently being seeded. This occurs when the
// client is willing to upload without wanting anything in return.
func (t *Torrent) Seeding() bool {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	return t.seeding()
}

// Clobbers the torrent display name. The display name is used as the torrent
// name if the metainfo is not available.
func (t *Torrent) SetDisplayName(dn string) {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	t.setDisplayName(dn)
}

// The current working name for the torrent. Either the name in the info dict,
// or a display name given such as by the dn value in a magnet link, or "".
func (t *Torrent) Name() string {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	return t.name()
}

// The completed length of all the torrent data, in all its files. This is
// derived from the torrent info, when it is available.
func (t *Torrent) Length() int64 {
	if t.info == nil {
		panic("not valid until info obtained")
	}
	return t.length
}

// Returns a run-time generated metainfo for the torrent that includes the
// info bytes and announce-list as currently known to the client.
func (t *Torrent) Metainfo() metainfo.MetaInfo {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	return t.newMetaInfo()
}

func (t *Torrent) addReader(r *Reader) {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	if t.readers == nil {
		t.readers = make(map[*Reader]struct{})
	}
	t.readers[r] = struct{}{}
	r.posChanged()
}

func (t *Torrent) deleteReader(r *Reader) {
	delete(t.readers, r)
	t.readersChanged()
}

func (t *Torrent) DownloadPieces(begin, end int) {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	t.pendPieceRange(begin, end)
}

func (t *Torrent) CancelPieces(begin, end int) {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	t.unpendPieceRange(begin, end)
}

// Returns handles to the files in the torrent. This requires the metainfo is
// available first.
func (t *Torrent) Files() (ret []File) {
	info := t.Info()
	if info == nil {
		return
	}
	var offset int64
	for _, fi := range info.UpvertedFiles() {
		ret = append(ret, File{
			t,
			strings.Join(append([]string{info.Name}, fi.Path...), "/"),
			offset,
			fi.Length,
			fi,
		})
		offset += fi.Length
	}
	return
}

func (t *Torrent) AddPeers(pp []Peer) {
	cl := t.cl
	cl.mu.Lock()
	defer cl.mu.Unlock()
	t.addPeers(pp)
}

// Marks the entire torrent for download. Requires the info first, see
// GotInfo.
func (t *Torrent) DownloadAll() {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	t.pendPieceRange(0, t.numPieces())
}

func (t *Torrent) String() string {
	s := t.name()
	if s == "" {
		s = t.infoHash.HexString()
	}
	return s
}

func (t *Torrent) AddTrackers(announceList [][]string) {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	t.addTrackers(announceList)
}

func (t *Torrent) Piece(i int) *Piece {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	return &t.pieces[i]
}
