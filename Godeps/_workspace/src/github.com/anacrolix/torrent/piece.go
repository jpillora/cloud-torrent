package torrent

import (
	"math/rand"
	"sync"

	pp "github.com/anacrolix/torrent/peer_protocol"
)

// Piece priority describes the importance of obtaining a particular piece.

type piecePriority byte

const (
	PiecePriorityNone      piecePriority = iota // Not wanted.
	PiecePriorityNormal                         // Wanted.
	PiecePriorityReadahead                      // May be required soon.
	PiecePriorityNext                           // Succeeds a piece where a read occurred.
	PiecePriorityNow                            // A read occurred in this piece.
)

type piece struct {
	// The completed piece SHA1 hash, from the metainfo "pieces" field.
	Hash pieceSum
	// Chunks we don't have. The offset and length can be determined by the
	// request chunkSize in use.
	PendingChunkSpecs []bool
	Hashing           bool
	QueuedForHash     bool
	EverHashed        bool
	Event             sync.Cond
	Priority          piecePriority

	pendingWritesMutex sync.Mutex
	pendingWrites      int
	noPendingWrites    sync.Cond
}

func (p *piece) pendingChunk(cs chunkSpec, chunkSize pp.Integer) bool {
	if p.PendingChunkSpecs == nil {
		return false
	}
	return p.PendingChunkSpecs[chunkIndex(cs, chunkSize)]
}

func (p *piece) numPendingChunks() (ret int) {
	for _, pending := range p.PendingChunkSpecs {
		if pending {
			ret++
		}
	}
	return
}

func (p *piece) unpendChunkIndex(i int) {
	if p.PendingChunkSpecs == nil {
		return
	}
	p.PendingChunkSpecs[i] = false
}

func chunkIndexSpec(index int, pieceLength, chunkSize pp.Integer) chunkSpec {
	ret := chunkSpec{pp.Integer(index) * chunkSize, chunkSize}
	if ret.Begin+ret.Length > pieceLength {
		ret.Length = pieceLength - ret.Begin
	}
	return ret
}

func (p *piece) shuffledPendingChunkSpecs(pieceLength, chunkSize pp.Integer) (css []chunkSpec) {
	if p.numPendingChunks() == 0 {
		return
	}
	css = make([]chunkSpec, 0, p.numPendingChunks())
	for i, pending := range p.PendingChunkSpecs {
		if pending {
			css = append(css, chunkIndexSpec(i, pieceLength, chunkSize))
		}
	}
	if len(css) <= 1 {
		return
	}
	for i := range css {
		j := rand.Intn(i + 1)
		css[i], css[j] = css[j], css[i]
	}
	return
}
