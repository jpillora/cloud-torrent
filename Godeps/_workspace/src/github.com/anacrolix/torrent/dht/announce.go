package dht

// get_peers and announce_peers.

import (
	"log"
	"time"

	"github.com/anacrolix/missinggo"
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
	startAddrs := func() (ret []dHTAddr) {
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
			startAddrs = append(startAddrs, newDHTAddr(addr))
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

func (me *Announce) gotNodeAddr(addr dHTAddr) {
	if missinggo.AddrPort(addr) == 0 {
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

func (me *Announce) contact(addr dHTAddr) {
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

func (me *Announce) announcePeer(to dHTAddr, token string) {
	me.server.mu.Lock()
	err := me.server.announcePeer(to, me.infoHash, me.announcePort, token, me.announcePortImplied)
	me.server.mu.Unlock()
	if err != nil {
		logonce.Stderr.Printf("error announcing peer: %s", err)
	}
}

func (me *Announce) getPeers(addr dHTAddr) error {
	me.server.mu.Lock()
	defer me.server.mu.Unlock()
	t, err := me.server.getPeers(addr, me.infoHash)
	if err != nil {
		return err
	}
	t.SetResponseHandler(func(m Msg) {
		// Register suggested nodes closer to the target info-hash.
		me.mu.Lock()
		for _, n := range m.Nodes() {
			me.responseNode(n)
		}
		me.mu.Unlock()

		if vs := m.Values(); vs != nil {
			for _, cp := range vs {
				if cp.Port == 0 {
					me.server.mu.Lock()
					me.server.badNode(addr)
					me.server.mu.Unlock()
					return
				}
			}
			nodeInfo := NodeInfo{
				Addr: t.remoteAddr,
			}
			copy(nodeInfo.ID[:], m.SenderID())
			select {
			case me.values <- PeersValues{
				Peers:    vs,
				NodeInfo: nodeInfo,
			}:
			case <-me.stop:
			}
		}

		if at, ok := m.AnnounceToken(); ok {
			me.announcePeer(addr, at)
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
