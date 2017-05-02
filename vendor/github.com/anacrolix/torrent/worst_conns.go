package torrent

import "container/heap"

func worseConn(l, r *connection) bool {
	if l.useful() != r.useful() {
		return r.useful()
	}
	if !l.lastHelpful().Equal(r.lastHelpful()) {
		return l.lastHelpful().Before(r.lastHelpful())
	}
	return l.completedHandshake.Before(r.completedHandshake)
}

type worseConnSlice struct {
	conns []*connection
}

var _ heap.Interface = &worseConnSlice{}

func (me worseConnSlice) Len() int {
	return len(me.conns)
}

func (me worseConnSlice) Less(i, j int) bool {
	return worseConn(me.conns[i], me.conns[j])
}

func (me *worseConnSlice) Pop() interface{} {
	i := len(me.conns) - 1
	ret := me.conns[i]
	me.conns = me.conns[:i]
	return ret
}

func (me *worseConnSlice) Push(x interface{}) {
	me.conns = append(me.conns, x.(*connection))
}

func (me worseConnSlice) Swap(i, j int) {
	me.conns[i], me.conns[j] = me.conns[j], me.conns[i]
}
