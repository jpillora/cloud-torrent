package storage

import (
	"sync"

	"github.com/anacrolix/torrent/metainfo"
)

type mapPieceCompletion struct {
	mu sync.Mutex
	m  map[metainfo.PieceKey]struct{}
}

func NewMapPieceCompletion() PieceCompletion {
	return &mapPieceCompletion{m: make(map[metainfo.PieceKey]struct{})}
}

func (*mapPieceCompletion) Close() error { return nil }

func (me *mapPieceCompletion) Get(pk metainfo.PieceKey) (bool, error) {
	me.mu.Lock()
	_, ok := me.m[pk]
	me.mu.Unlock()
	return ok, nil
}

func (me *mapPieceCompletion) Set(pk metainfo.PieceKey, b bool) error {
	me.mu.Lock()
	if b {
		if me.m == nil {
			me.m = make(map[metainfo.PieceKey]struct{})
		}
		me.m[pk] = struct{}{}
	} else {
		delete(me.m, pk)
	}
	me.mu.Unlock()
	return nil
}
