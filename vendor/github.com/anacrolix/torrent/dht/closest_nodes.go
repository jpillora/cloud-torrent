package dht

import (
	"container/heap"
)

type nodeMaxHeap struct {
	IDs    []nodeID
	Target nodeID
}

func (mh nodeMaxHeap) Len() int { return len(mh.IDs) }

func (mh nodeMaxHeap) Less(i, j int) bool {
	m := mh.IDs[i].Distance(&mh.Target)
	n := mh.IDs[j].Distance(&mh.Target)
	return m.Cmp(&n) > 0
}

func (mh *nodeMaxHeap) Pop() (ret interface{}) {
	ret, mh.IDs = mh.IDs[len(mh.IDs)-1], mh.IDs[:len(mh.IDs)-1]
	return
}
func (mh *nodeMaxHeap) Push(val interface{}) {
	mh.IDs = append(mh.IDs, val.(nodeID))
}
func (mh nodeMaxHeap) Swap(i, j int) {
	mh.IDs[i], mh.IDs[j] = mh.IDs[j], mh.IDs[i]
}

type closestNodesSelector struct {
	closest nodeMaxHeap
	k       int
}

func (cns *closestNodesSelector) Push(id nodeID) {
	heap.Push(&cns.closest, id)
	if cns.closest.Len() > cns.k {
		heap.Pop(&cns.closest)
	}
}

func (cns *closestNodesSelector) IDs() []nodeID {
	return cns.closest.IDs
}

func newKClosestNodesSelector(k int, targetID nodeID) (ret closestNodesSelector) {
	ret.k = k
	ret.closest.Target = targetID
	return
}
