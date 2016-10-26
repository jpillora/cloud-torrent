package storage

import (
	"github.com/anacrolix/torrent/metainfo"
)

type mapPieceCompletion struct {
	m map[metainfo.PieceKey]struct{}
}

func (mapPieceCompletion) Close() error { return nil }

func (me *mapPieceCompletion) Get(pk metainfo.PieceKey) (bool, error) {
	_, ok := me.m[pk]
	return ok, nil
}

func (me *mapPieceCompletion) Set(pk metainfo.PieceKey, b bool) error {
	if b {
		if me.m == nil {
			me.m = make(map[metainfo.PieceKey]struct{})
		}
		me.m[pk] = struct{}{}
	} else {
		delete(me.m, pk)
	}
	return nil
}
