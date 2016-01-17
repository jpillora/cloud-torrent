package torrent

import (
	"math/rand"
	"sync"

	"github.com/bradfitz/iter"

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
	// Chunks we've written to since the last check. The chunk offset and
	// length can be determined by the request chunkSize in use.
	DirtyChunks      []bool
	Hashing          bool
	QueuedForHash    bool
	EverHashed       bool
	Priority         piecePriority
	PublicPieceState PieceState

	pendingWritesMutex sync.Mutex
	pendingWrites      int
	noPendingWrites    sync.Cond
}

func (p *piece) pendingChunk(cs chunkSpec, chunkSize pp.Integer) bool {
	ci := chunkIndex(cs, chunkSize)
	if ci >= len(p.DirtyChunks) {
		return true
	}
	return !p.DirtyChunks[ci]
}

func (p *piece) numDirtyChunks() (ret int) {
	for _, dirty := range p.DirtyChunks {
		if dirty {
			ret++
		}
	}
	return
}

func (p *piece) unpendChunkIndex(i int) {
	for i >= len(p.DirtyChunks) {
		p.DirtyChunks = append(p.DirtyChunks, false)
	}
	p.DirtyChunks[i] = true
}

func chunkIndexSpec(index int, pieceLength, chunkSize pp.Integer) chunkSpec {
	ret := chunkSpec{pp.Integer(index) * chunkSize, chunkSize}
	if ret.Begin+ret.Length > pieceLength {
		ret.Length = pieceLength - ret.Begin
	}
	return ret
}

func (p *piece) shuffledPendingChunkSpecs(t *torrent, piece int) (css []chunkSpec) {
	// defer func() {
	// 	log.Println(piece, css)
	// }()
	numPending := t.pieceNumPendingChunks(piece)
	if numPending == 0 {
		return
	}
	css = make([]chunkSpec, 0, numPending)
	for ci := range iter.N(t.pieceNumChunks(piece)) {
		if ci >= len(p.DirtyChunks) || !p.DirtyChunks[ci] {
			css = append(css, t.chunkIndexSpec(ci, piece))
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
