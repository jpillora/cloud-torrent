package dht

import (
	"container/heap"
)

type nodeMaxHeap struct {
	IDs    []nodeID
	Target nodeID
}

func (me nodeMaxHeap) Len() int { return len(me.IDs) }

func (me nodeMaxHeap) Less(i, j int) bool {
	m := me.IDs[i].Distance(&me.Target)
	n := me.IDs[j].Distance(&me.Target)
	return m.Cmp(&n) > 0
}

func (me *nodeMaxHeap) Pop() (ret interface{}) {
	ret, me.IDs = me.IDs[len(me.IDs)-1], me.IDs[:len(me.IDs)-1]
	return
}
func (me *nodeMaxHeap) Push(val interface{}) {
	me.IDs = append(me.IDs, val.(nodeID))
}
func (me nodeMaxHeap) Swap(i, j int) {
	me.IDs[i], me.IDs[j] = me.IDs[j], me.IDs[i]
}

type closestNodesSelector struct {
	closest nodeMaxHeap
	k       int
}

func (me *closestNodesSelector) Push(id nodeID) {
	heap.Push(&me.closest, id)
	if me.closest.Len() > me.k {
		heap.Pop(&me.closest)
	}
}

func (me *closestNodesSelector) IDs() []nodeID {
	return me.closest.IDs
}

func newKClosestNodesSelector(k int, targetID nodeID) (ret closestNodesSelector) {
	ret.k = k
	ret.closest.Target = targetID
	return
}
