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

type Piece struct {
	// The completed piece SHA1 hash, from the metainfo "pieces" field.
	hash  metainfo.Hash
	t     *Torrent
	index int
	// Chunks we've written to since the last check. The chunk offset and
	// length can be determined by the request chunkSize in use.
	dirtyChunks bitmap.Bitmap

	hashing       bool
	queuedForHash bool
	everHashed    bool
	numVerifies   int64

	publicPieceState PieceState
	priority         piecePriority

	pendingWritesMutex sync.Mutex
	pendingWrites      int
	noPendingWrites    sync.Cond
}

func (p *Piece) Info() metainfo.Piece {
	return p.t.info.Piece(p.index)
}

func (p *Piece) Storage() storage.Piece {
	return p.t.storage.Piece(p.Info())
}

func (p *Piece) pendingChunkIndex(chunkIndex int) bool {
	return !p.dirtyChunks.Contains(chunkIndex)
}

func (p *Piece) pendingChunk(cs chunkSpec, chunkSize pp.Integer) bool {
	return p.pendingChunkIndex(chunkIndex(cs, chunkSize))
}

func (p *Piece) hasDirtyChunks() bool {
	return p.dirtyChunks.Len() != 0
}

func (p *Piece) numDirtyChunks() (ret int) {
	return p.dirtyChunks.Len()
}

func (p *Piece) unpendChunkIndex(i int) {
	p.dirtyChunks.Add(i)
}

func (p *Piece) pendChunkIndex(i int) {
	p.dirtyChunks.Remove(i)
}

func (p *Piece) numChunks() int {
	return p.t.pieceNumChunks(p.index)
}

func (p *Piece) undirtiedChunkIndices() (ret bitmap.Bitmap) {
	ret = p.dirtyChunks.Copy()
	ret.FlipRange(0, p.numChunks())
	return
}

func (p *Piece) incrementPendingWrites() {
	p.pendingWritesMutex.Lock()
	p.pendingWrites++
	p.pendingWritesMutex.Unlock()
}

func (p *Piece) decrementPendingWrites() {
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

func (p *Piece) waitNoPendingWrites() {
	p.pendingWritesMutex.Lock()
	for p.pendingWrites != 0 {
		p.noPendingWrites.Wait()
	}
	p.pendingWritesMutex.Unlock()
}

func (p *Piece) chunkIndexDirty(chunk int) bool {
	return p.dirtyChunks.Contains(chunk)
}

func (p *Piece) chunkIndexSpec(chunk int) chunkSpec {
	return chunkIndexSpec(chunk, p.length(), p.chunkSize())
}

func (p *Piece) numDirtyBytes() (ret pp.Integer) {
	// defer func() {
	// 	if ret > p.length() {
	// 		panic("too many dirty bytes")
	// 	}
	// }()
	numRegularDirtyChunks := p.numDirtyChunks()
	if p.chunkIndexDirty(p.numChunks() - 1) {
		numRegularDirtyChunks--
		ret += p.chunkIndexSpec(p.lastChunkIndex()).Length
	}
	ret += pp.Integer(numRegularDirtyChunks) * p.chunkSize()
	return
}

func (p *Piece) length() pp.Integer {
	return p.t.pieceLength(p.index)
}

func (p *Piece) chunkSize() pp.Integer {
	return p.t.chunkSize
}

func (p *Piece) lastChunkIndex() int {
	return p.numChunks() - 1
}

func (p *Piece) bytesLeft() (ret pp.Integer) {
	if p.t.pieceComplete(p.index) {
		return 0
	}
	return p.length() - p.numDirtyBytes()
}

func (p *Piece) VerifyData() {
	p.t.cl.mu.Lock()
	defer p.t.cl.mu.Unlock()
	target := p.numVerifies + 1
	if p.hashing {
		target++
	}
	p.t.queuePieceCheck(p.index)
	for p.numVerifies < target {
		p.t.cl.event.Wait()
	}
}
