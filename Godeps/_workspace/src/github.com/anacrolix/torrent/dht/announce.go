package dht

// get_peers and announce_peers.

import (
	"log"
	"time"

	"github.com/anacrolix/sync"
	"github.com/willf/bloom"

	"github.com/anacrolix/torrent/logonce"
)

// Maintains state for an ongoing Announce operation. An Announce is started
// by calling Server.Announce.
type Announce struct {
	mu    sync.Mutex
	Peers chan PeersValues
	// Inner chan is set to nil when on close.
	values              chan PeersValues
	stop                chan struct{}
	triedAddrs          *bloom.BloomFilter
	pending             int
	server              *Server
	infoHash            string
	numContacted        int
	announcePort        int
	announcePortImplied bool
}

// Returns the number of distinct remote addresses the announce has queried.
func (me *Announce) NumContacted() int {
	me.mu.Lock()
	defer me.mu.Unlock()
	return me.numContacted
}

// This is kind of the main thing you want to do with DHT. It traverses the
// graph toward nodes that store peers for the infohash, streaming them to the
// caller, and announcing the local node to each node if allowed and
// specified.
func (s *Server) Announce(infoHash string, port int, impliedPort bool) (*Announce, error) {
	s.mu.Lock()
	startAddrs := func() (ret []Addr) {
		for _, n := range s.closestGoodNodes(160, infoHash) {
			ret = append(ret, n.addr)
		}
		return
	}()
	s.mu.Unlock()
	if len(startAddrs) == 0 {
		addrs, err := bootstrapAddrs(s.bootstrapNodes)
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			startAddrs = append(startAddrs, NewAddr(addr))
		}
	}
	disc := &Announce{
		Peers:               make(chan PeersValues, 100),
		stop:                make(chan struct{}),
		values:              make(chan PeersValues),
		triedAddrs:          bloom.NewWithEstimates(1000, 0.5),
		server:              s,
		infoHash:            infoHash,
		announcePort:        port,
		announcePortImplied: impliedPort,
	}
	// Function ferries from values to Values until discovery is halted.
	go func() {
		defer close(disc.Peers)
		for {
			select {
			case psv := <-disc.values:
				select {
				case disc.Peers <- psv:
				case <-disc.stop:
					return
				}
			case <-disc.stop:
				return
			}
		}
	}()
	for i, addr := range startAddrs {
		if i != 0 {
			time.Sleep(time.Millisecond)
		}
		disc.mu.Lock()
		disc.contact(addr)
		disc.mu.Unlock()
	}
	return disc, nil
}

func (me *Announce) gotNodeAddr(addr Addr) {
	if addr.UDPAddr().Port == 0 {
		// Not a contactable address.
		return
	}
	if me.triedAddrs.Test([]byte(addr.String())) {
		return
	}
	if me.server.ipBlocked(addr.UDPAddr().IP) {
		return
	}
	me.server.mu.Lock()
	if me.server.badNodes.Test([]byte(addr.String())) {
		me.server.mu.Unlock()
		return
	}
	me.server.mu.Unlock()
	me.contact(addr)
}

func (me *Announce) contact(addr Addr) {
	me.numContacted++
	me.triedAddrs.Add([]byte(addr.String()))
	if err := me.getPeers(addr); err != nil {
		log.Printf("error sending get_peers request to %s: %#v", addr, err)
		return
	}
	me.pending++
}

func (me *Announce) transactionClosed() {
	me.pending--
	if me.pending == 0 {
		me.close()
		return
	}
}

func (me *Announce) responseNode(node NodeInfo) {
	me.gotNodeAddr(node.Addr)
}

func (me *Announce) closingCh() chan struct{} {
	return me.stop
}

// Announce to a peer, if appropriate.
func (me *Announce) maybeAnnouncePeer(to Addr, token, peerId string) {
	me.server.mu.Lock()
	defer me.server.mu.Unlock()
	if !me.server.config.NoSecurity {
		if len(peerId) != 20 {
			return
		}
		if !NodeIdSecure(peerId, to.UDPAddr().IP) {
			return
		}
	}
	err := me.server.announcePeer(to, me.infoHash, me.announcePort, token, me.announcePortImplied)
	if err != nil {
		logonce.Stderr.Printf("error announcing peer: %s", err)
	}
}

func (me *Announce) getPeers(addr Addr) error {
	me.server.mu.Lock()
	defer me.server.mu.Unlock()
	t, err := me.server.getPeers(addr, me.infoHash)
	if err != nil {
		return err
	}
	t.SetResponseHandler(func(m Msg, ok bool) {
		// Register suggested nodes closer to the target info-hash.
		if m.R != nil {
			me.mu.Lock()
			for _, n := range m.R.Nodes {
				me.responseNode(n)
			}
			me.mu.Unlock()

			if vs := m.R.Values; len(vs) != 0 {
				nodeInfo := NodeInfo{
					Addr: t.remoteAddr,
				}
				copy(nodeInfo.ID[:], m.SenderID())
				select {
				case me.values <- PeersValues{
					Peers: func() (ret []Peer) {
						for _, cp := range vs {
							ret = append(ret, Peer(cp))
						}
						return
					}(),
					NodeInfo: nodeInfo,
				}:
				case <-me.stop:
				}
			}

			me.maybeAnnouncePeer(addr, m.R.Token, m.SenderID())
		}

		me.mu.Lock()
		me.transactionClosed()
		me.mu.Unlock()
	})
	return nil
}

// Corresponds to the "values" key in a get_peers KRPC response. A list of
// peers that a node has reported as being in the swarm for a queried info
// hash.
type PeersValues struct {
	Peers    []Peer // Peers given in get_peers response.
	NodeInfo        // The node that gave the response.
}

// Stop the announce.
func (me *Announce) Close() {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.close()
}

func (ps *Announce) close() {
	select {
	case <-ps.stop:
	default:
		close(ps.stop)
	}
}
