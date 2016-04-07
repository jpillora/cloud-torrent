package metainfo

import "github.com/anacrolix/missinggo"

type Piece struct {
	Info *InfoEx
	i    int
}

func (me Piece) Length() int64 {
	if me.i == me.Info.NumPieces()-1 {
		return me.Info.TotalLength() - int64(me.i)*me.Info.PieceLength
	}
	return me.Info.PieceLength
}

func (me Piece) Offset() int64 {
	return int64(me.i) * me.Info.PieceLength
}

func (me Piece) Hash() (ret Hash) {
	missinggo.CopyExact(&ret, me.Info.Pieces[me.i*20:(me.i+1)*20])
	return
}
