package pieceStore

import (
	"encoding/hex"
	"io"

	"github.com/anacrolix/torrent/metainfo"
)

type data struct {
	info  *metainfo.Info
	store *store
}

func (me *data) pieceHashHex(i int) string {
	return hex.EncodeToString(me.info.Pieces[i*20 : (i+1)*20])
}

func (me *data) Close() {}

// TODO: Make sure that reading completed can't read from incomplete. Then
// also it'll be possible to verify that the Content-Range on completed
// returns the correct piece length so there aren't short reads.

func (me *data) ReadAt(b []byte, off int64) (n int, err error) {
	for len(b) != 0 {
		if off >= me.info.TotalLength() {
			err = io.EOF
			break
		}
		p := me.info.Piece(int(off / me.info.PieceLength))
		b1 := b
		maxN1 := int(p.Length() - off%me.info.PieceLength)
		if len(b1) > maxN1 {
			b1 = b1[:maxN1]
		}
		var n1 int
		n1, err = me.store.pieceReadAt(p, b1, off%me.info.PieceLength)
		n += n1
		off += int64(n1)
		b = b[n1:]
		if err != nil {
			break
		}
	}
	return
}

// TODO: Rewrite this later, on short writes to a piece it will start to play up.
func (me *data) WriteAt(p []byte, off int64) (n int, err error) {
	i := int(off / me.info.PieceLength)
	off %= me.info.PieceLength
	for len(p) != 0 {
		p1 := p
		maxN := me.info.Piece(i).Length() - off
		if int64(len(p1)) > maxN {
			p1 = p1[:maxN]
		}
		var n1 int
		n1, err = me.store.pieceWriteAt(me.info.Piece(i), p1, off)
		n += n1
		if err != nil {
			return
		}
		p = p[n1:]
		off = 0
		i++
	}
	return
}

func (me *data) pieceReader(p metainfo.Piece, off int64) (ret io.ReadCloser, err error) {
	return me.store.getPieceRange(p, off, p.Length()-off)
}

func (me *data) WriteSectionTo(w io.Writer, off, n int64) (written int64, err error) {
	i := int(off / me.info.PieceLength)
	off %= me.info.PieceLength
	for n != 0 {
		if i >= me.info.NumPieces() {
			err = io.EOF
			break
		}
		p := me.info.Piece(i)
		if off >= p.Length() {
			err = io.EOF
			break
		}
		var pr io.ReadCloser
		pr, err = me.pieceReader(p, off)
		if err != nil {
			return
		}
		var n1 int64
		n1, err = io.CopyN(w, pr, n)
		pr.Close()
		written += n1
		n -= n1
		if err != nil {
			return
		}
		off = 0
		i++
	}
	return
}

func (me *data) PieceCompleted(index int) (err error) {
	return me.store.PieceCompleted(me.info.Piece(index))
}

func (me *data) PieceComplete(piece int) bool {
	return me.store.pieceComplete(me.info.Piece(piece))
}
