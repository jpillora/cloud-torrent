package torrent

import (
	"github.com/anacrolix/dht/krpc"
	"github.com/anacrolix/torrent/tracker"
)

type Peers []Peer

func (me *Peers) FromPex(nas []krpc.NodeAddr, fs []pexPeerFlags) {
	for i, na := range nas {
		var p Peer
		var f pexPeerFlags
		if i < len(fs) {
			f = fs[i]
		}
		p.FromPex(na, f)
		*me = append(*me, p)
	}
}

func (ret Peers) FromTracker(ps []tracker.Peer) Peers {
	for _, p := range ps {
		_p := Peer{
			IP:     p.IP,
			Port:   p.Port,
			Source: peerSourceTracker,
		}
		copy(_p.Id[:], p.ID)
		ret = append(ret, _p)
	}
	return ret
}
