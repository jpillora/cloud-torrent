package torrent

import (
	"crypto"
	"errors"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	pp "github.com/anacrolix/torrent/peer_protocol"
)

const (
	pieceHash        = crypto.SHA1
	maxRequests      = 250    // Maximum pending requests we allow peers to send us.
	defaultChunkSize = 0x4000 // 16KiB
	// Peer ID client identifier prefix. We'll update this occasionally to
	// reflect changes to client behaviour that other clients may depend on.
	// Also see `extendedHandshakeClientVersion`.
	bep20              = "-GT0001-"
	nominalDialTimeout = time.Second * 30
	minDialTimeout     = 5 * time.Second
)

type chunkSpec struct {
	Begin, Length pp.Integer
}

type request struct {
	Index pp.Integer
	chunkSpec
}

func newRequest(index, begin, length pp.Integer) request {
	return request{index, chunkSpec{begin, length}}
}

// The size in bytes of a metadata extension piece.
func metadataPieceSize(totalSize int, piece int) int {
	ret := totalSize - piece*(1<<14)
	if ret > 1<<14 {
		ret = 1 << 14
	}
	return ret
}

// Return the request that would include the given offset into the torrent data.
func torrentOffsetRequest(torrentLength, pieceSize, chunkSize, offset int64) (
	r request, ok bool) {
	if offset < 0 || offset >= torrentLength {
		return
	}
	r.Index = pp.Integer(offset / pieceSize)
	r.Begin = pp.Integer(offset % pieceSize / chunkSize * chunkSize)
	r.Length = pp.Integer(chunkSize)
	pieceLeft := pp.Integer(pieceSize - int64(r.Begin))
	if r.Length > pieceLeft {
		r.Length = pieceLeft
	}
	torrentLeft := torrentLength - int64(r.Index)*pieceSize - int64(r.Begin)
	if int64(r.Length) > torrentLeft {
		r.Length = pp.Integer(torrentLeft)
	}
	ok = true
	return
}

func torrentRequestOffset(torrentLength, pieceSize int64, r request) (off int64) {
	off = int64(r.Index)*pieceSize + int64(r.Begin)
	if off < 0 || off >= torrentLength {
		panic("invalid request")
	}
	return
}

func validateInfo(info *metainfo.Info) error {
	if len(info.Pieces)%20 != 0 {
		return errors.New("pieces has invalid length")
	}
	if int((info.TotalLength()+info.PieceLength-1)/info.PieceLength) != info.NumPieces() {
		return errors.New("piece count and file lengths are at odds")
	}
	return nil
}

func chunkIndexSpec(index int, pieceLength, chunkSize pp.Integer) chunkSpec {
	ret := chunkSpec{pp.Integer(index) * chunkSize, chunkSize}
	if ret.Begin+ret.Length > pieceLength {
		ret.Length = pieceLength - ret.Begin
	}
	return ret
}
