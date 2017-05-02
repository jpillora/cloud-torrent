package dht

import (
	"container/heap"
	"encoding/hex"
	"fmt"

	"github.com/anacrolix/missinggo/container/xheap"
	"github.com/anacrolix/missinggo/iter"
)

type nodeIDAndDistance struct {
	nodeID   nodeID
	distance int160
}

func (me nodeIDAndDistance) String() string {
	return fmt.Sprintf("%s: %q", hex.EncodeToString(me.distance.Bytes()), me.nodeID)
}

type kClosestNodeIDs struct {
	sl     []interface{}
	target nodeID
	h      heap.Interface
	k      int
}

func (me *kClosestNodeIDs) Push(id nodeID) {
	d := me.target.Distance(&id)
	heap.Push(me.h, nodeIDAndDistance{id, d})
	if me.h.Len() > me.k {
		heap.Pop(me.h)
	}
}

func newKClosestNodeIDs(k int, target nodeID) (ret *kClosestNodeIDs) {
	ret = &kClosestNodeIDs{
		target: target,
		k:      k,
	}
	ret.h = xheap.Slice(&ret.sl, func(l, r interface{}) bool {
		return l.(nodeIDAndDistance).distance.Cmp(r.(nodeIDAndDistance).distance) > 0
	})
	return
}

func (me *kClosestNodeIDs) IDs() iter.Iterator {
	return iter.Map(iter.Slice(me.sl), func(i interface{}) interface{} {
		return i.(nodeIDAndDistance).nodeID
	})
}
