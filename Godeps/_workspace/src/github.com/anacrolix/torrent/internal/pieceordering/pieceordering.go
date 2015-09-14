// Implements ordering of torrent piece indices for such purposes as download
// prioritization.
package pieceordering

import (
	"math/rand"

	"github.com/ryszard/goskiplist/skiplist"
)

// Maintains piece integers by their ascending assigned keys.
type Instance struct {
	// Contains the ascending priority keys. The keys contain a slice of piece
	// indices.
	sl *skiplist.SkipList
	// Maps from piece index back to its key, so that it can be remove
	// efficiently from the skip list.
	pieceKeys map[int]int
}

func New() *Instance {
	return &Instance{
		sl: skiplist.NewIntMap(),
	}
}

// Add the piece with the given key. If the piece is already present, change
// its key.
func (me *Instance) SetPiece(piece, key int) {
	if existingKey, ok := me.pieceKeys[piece]; ok {
		if existingKey == key {
			return
		}
		me.removeKeyPiece(existingKey, piece)
	}
	var itemSl []int
	if exItem, ok := me.sl.Get(key); ok {
		itemSl = exItem.([]int)
	}
	me.sl.Set(key, append(itemSl, piece))
	if me.pieceKeys == nil {
		me.pieceKeys = make(map[int]int)
	}
	me.pieceKeys[piece] = key
	me.shuffleItem(key)
}

// Shuffle the piece indices that share a given key.
func (me *Instance) shuffleItem(key int) {
	_item, ok := me.sl.Get(key)
	if !ok {
		return
	}
	item := _item.([]int)
	for i := range item {
		j := i + rand.Intn(len(item)-i)
		item[i], item[j] = item[j], item[i]
	}
	me.sl.Set(key, item)
}

func (me *Instance) removeKeyPiece(key, piece int) {
	item, ok := me.sl.Get(key)
	if !ok {
		panic("no item for key")
	}
	itemSl := item.([]int)
	for i, piece1 := range itemSl {
		if piece1 == piece {
			itemSl[i] = itemSl[len(itemSl)-1]
			itemSl = itemSl[:len(itemSl)-1]
			break
		}
	}
	if len(itemSl) == 0 {
		me.sl.Delete(key)
	} else {
		me.sl.Set(key, itemSl)
	}
}

func (me *Instance) DeletePiece(piece int) {
	key, ok := me.pieceKeys[piece]
	if !ok {
		return
	}
	me.removeKeyPiece(key, piece)
	delete(me.pieceKeys, piece)
}

// Returns the piece with the lowest key.
func (me Instance) First() Element {
	i := me.sl.SeekToFirst()
	if i == nil {
		return nil
	}
	return &element{i, i.Value().([]int)}
}

type Element interface {
	Piece() int
	Next() Element
}

type element struct {
	i  skiplist.Iterator
	sl []int
}

func (e *element) Next() Element {
	e.sl = e.sl[1:]
	if len(e.sl) > 0 {
		return e
	}
	ok := e.i.Next()
	if !ok {
		return nil
	}
	e.sl = e.i.Value().([]int)
	return e
}

func (e element) Piece() int {
	return e.sl[0]
}
