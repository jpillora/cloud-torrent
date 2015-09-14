package torrent

import "github.com/anacrolix/torrent/data"

type statelessDataWrapper struct {
	data.Data
	complete []bool
}

func (me *statelessDataWrapper) PieceComplete(piece int) bool {
	return me.complete[piece]
}

func (me *statelessDataWrapper) PieceCompleted(piece int) error {
	me.complete[piece] = true
	return nil
}

func (me *statelessDataWrapper) Super() interface{} {
	return me.Data
}
