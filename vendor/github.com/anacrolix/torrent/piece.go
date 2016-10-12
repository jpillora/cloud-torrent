package torrent

import (
	"sync"

	"github.com/anacrolix/missinggo/bitmap"

	"github.com/anacrolix/torrent/metainfo"
	pp "github.com/anacrolix/torrent/peer_protocol"
	"github.com/anacrolix/torrent/storage"
)

// Piece priority describes the importance of obtaining a particular piece.

type piecePriority byte

func (pp *piecePriority) Raise(maybe piecePriority) {
	if maybe > *pp {
		*pp = maybe
	}
}

const (
	PiecePriorityNone      piecePriority = iota // Not wanted.
	PiecePriorityNormal                         // Wanted.
	PiecePriorityReadahead                      // May be required soon.
	// Succeeds a piece where a read occurred. Currently the same as Now, apparently due to issues with caching.
	PiecePriorityNext
	PiecePriorityNow // A Reader is reading in this piece.
)

type piece struct {
	// The completed piece SHA1 hash, from the metainfo "pieces" field.
	Hash  metainfo.Hash
	t     *Torrent
	index int
	// Chunks we've written to since the last check. The chunk offset and
	// length can be determined by the request chunkSize in use.
	DirtyChunks      bitmap.Bitmap
	Hashing          bool
	QueuedForHash    bool
	EverHashed       bool
	PublicPieceState PieceState
	priority         piecePriority

	pendingWritesMutex sync.Mutex
	pendingWrites      int
	noPendingWrites    sync.Cond
}

func (p *piece) Info() metainfo.Piece {
	return p.t.info.Piece(p.index)
}

func (p *piece) Storage() storage.Piece {
	return p.t.storage.Piece(p.Info())
}

func (p *piece) pendingChunkIndex(chunkIndex int) bool {
	return !p.DirtyChunks.Contains(chunkIndex)
}

func (p *piece) pendingChunk(cs chunkSpec, chunkSize pp.Integer) bool {
	return p.pendingChunkIndex(chunkIndex(cs, chunkSize))
}

func (p *piece) hasDirtyChunks() bool {
	return p.DirtyChunks.Len() != 0
}

func (p *piece) numDirtyChunks() (ret int) {
	return p.DirtyChunks.Len()
}

func (p *piece) unpendChunkIndex(i int) {
	p.DirtyChunks.Add(i)
}

func (p *piece) pendChunkIndex(i int) {
	p.DirtyChunks.Remove(i)
}

func (p *piece) numChunks() int {
	return p.t.pieceNumChunks(p.index)
}

func (p *piece) undirtiedChunkIndices() (ret bitmap.Bitmap) {
	ret = p.DirtyChunks.Copy()
	ret.FlipRange(0, p.numChunks())
	return
}

func (p *piece) incrementPendingWrites() {
	p.pendingWritesMutex.Lock()
	p.pendingWrites++
	p.pendingWritesMutex.Unlock()
}

func (p *piece) decrementPendingWrites() {
	p.pendingWritesMutex.Lock()
	if p.pendingWrites == 0 {
		panic("assertion")
	}
	p.pendingWrites--
	if p.pendingWrites == 0 {
		p.noPendingWrites.Broadcast()
	}
	p.pendingWritesMutex.Unlock()
}

func (p *piece) waitNoPendingWrites() {
	p.pendingWritesMutex.Lock()
	for p.pendingWrites != 0 {
		p.noPendingWrites.Wait()
	}
	p.pendingWritesMutex.Unlock()
}

func (p *piece) chunkIndexDirty(chunk int) bool {
	return p.DirtyChunks.Contains(chunk)
}

func (p *piece) chunkIndexSpec(chunk int) chunkSpec {
	return chunkIndexSpec(chunk, p.length(), p.chunkSize())
}

func (p *piece) numDirtyBytes() (ret pp.Integer) {
	defer func() {
		if ret > p.length() {
			panic("too many dirty bytes")
		}
	}()
	numRegularDirtyChunks := p.numDirtyChunks()
	if p.chunkIndexDirty(p.numChunks() - 1) {
		numRegularDirtyChunks--
		ret += p.chunkIndexSpec(p.lastChunkIndex()).Length
	}
	ret += pp.Integer(numRegularDirtyChunks) * p.chunkSize()
	return
}

func (p *piece) length() pp.Integer {
	return p.t.pieceLength(p.index)
}

func (p *piece) chunkSize() pp.Integer {
	return p.t.chunkSize
}

func (p *piece) lastChunkIndex() int {
	return p.numChunks() - 1
}

func (p *piece) bytesLeft() (ret pp.Integer) {
	if p.t.pieceComplete(p.index) {
		return 0
	}
	return p.length() - p.numDirtyBytes()
}
