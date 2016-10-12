package dht

// get_peers and announce_peers.

import (
	"log"
	"time"

	"github.com/anacrolix/sync"
	"github.com/willf/bloom"

	"github.com/anacrolix/torrent/dht/krpc"
	"github.com/anacrolix/torrent/logonce"
)

// Maintains state for an ongoing Announce operation. An Announce is started
// by calling Server.Announce.
type Announce struct {
	mu    sync.Mutex
	Peers chan PeersValues
	// Inner chan is set to nil when on close.
	values     chan PeersValues
	stop       chan struct{}
	triedAddrs *bloom.BloomFilter
	// True when contact with all starting addrs has been initiated. This
	// prevents a race where the first transaction finishes before the rest
	// have been opened, sees no other transactions are pending and ends the
	// announce.
	contactedStartAddrs bool
	// How many transactions are still ongoing.
	pending  int
	server   *Server
	infoHash string
	// Count of (probably) distinct addresses we've sent get_peers requests
	// to.
	numContacted int
	// The torrent port that we're announcing.
	announcePort int
	// The torrent port should be determined by the receiver in case we're
	// being NATed.
	announcePortImplied bool
}

// Returns the number of distinct remote addresses the announce has queried.
func (a *Announce) NumContacted() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.numContacted
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
	if len(startAddrs) == 0 && !s.config.NoDefaultBootstrap {
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
	go func() {
		disc.mu.Lock()
		defer disc.mu.Unlock()
		for i, addr := range startAddrs {
			if i != 0 {
				disc.mu.Unlock()
				time.Sleep(time.Millisecond)
				disc.mu.Lock()
			}
			disc.contact(addr)
		}
		disc.contactedStartAddrs = true
		// If we failed to contact any of the starting addrs, no transactions
		// will complete triggering a check that there are no pending
		// responses.
		disc.maybeClose()
	}()
	return disc, nil
}

func validNodeAddr(addr Addr) bool {
	ua := addr.UDPAddr()
	if ua.Port == 0 {
		return false
	}
	if ip4 := ua.IP.To4(); ip4 != nil && ip4[0] == 0 {
		return false
	}
	return true
}

// TODO: Merge this with maybeGetPeersFromAddr.
func (a *Announce) gotNodeAddr(addr Addr) {
	if !validNodeAddr(addr) {
		return
	}
	if a.triedAddrs.Test([]byte(addr.String())) {
		return
	}
	if a.server.ipBlocked(addr.UDPAddr().IP) {
		return
	}
	a.server.mu.Lock()
	if a.server.badNodes.Test([]byte(addr.String())) {
		a.server.mu.Unlock()
		return
	}
	a.server.mu.Unlock()
	a.contact(addr)
}

// TODO: Merge this with maybeGetPeersFromAddr.
func (a *Announce) contact(addr Addr) {
	a.numContacted++
	a.triedAddrs.Add([]byte(addr.String()))
	if err := a.getPeers(addr); err != nil {
		log.Printf("error sending get_peers request to %s: %#v", addr, err)
		return
	}
	a.pending++
}

func (a *Announce) maybeClose() {
	if a.contactedStartAddrs && a.pending == 0 {
		a.close()
	}
}

func (a *Announce) transactionClosed() {
	a.pending--
	a.maybeClose()
}

func (a *Announce) responseNode(node krpc.NodeInfo) {
	a.gotNodeAddr(NewAddr(node.Addr))
}

func (a *Announce) closingCh() chan struct{} {
	return a.stop
}

// Announce to a peer, if appropriate.
func (a *Announce) maybeAnnouncePeer(to Addr, token, peerId string) {
	a.server.mu.Lock()
	defer a.server.mu.Unlock()
	if !a.server.config.NoSecurity {
		if len(peerId) != 20 {
			return
		}
		if !NodeIdSecure(peerId, to.UDPAddr().IP) {
			return
		}
	}
	err := a.server.announcePeer(to, a.infoHash, a.announcePort, token, a.announcePortImplied)
	if err != nil {
		logonce.Stderr.Printf("error announcing peer: %s", err)
	}
}

func (a *Announce) getPeers(addr Addr) error {
	a.server.mu.Lock()
	defer a.server.mu.Unlock()
	t, err := a.server.getPeers(addr, a.infoHash)
	if err != nil {
		return err
	}
	t.SetResponseHandler(func(m krpc.Msg, ok bool) {
		// Register suggested nodes closer to the target info-hash.
		if m.R != nil {
			a.mu.Lock()
			for _, n := range m.R.Nodes {
				a.responseNode(n)
			}
			a.mu.Unlock()

			if vs := m.R.Values; len(vs) != 0 {
				nodeInfo := krpc.NodeInfo{
					Addr: t.remoteAddr.UDPAddr(),
				}
				copy(nodeInfo.ID[:], m.SenderID())
				select {
				case a.values <- PeersValues{
					Peers: func() (ret []Peer) {
						for _, cp := range vs {
							ret = append(ret, Peer(cp))
						}
						return
					}(),
					NodeInfo: nodeInfo,
				}:
				case <-a.stop:
				}
			}

			a.maybeAnnouncePeer(addr, m.R.Token, m.SenderID())
		}

		a.mu.Lock()
		a.transactionClosed()
		a.mu.Unlock()
	})
	return nil
}

// Corresponds to the "values" key in a get_peers KRPC response. A list of
// peers that a node has reported as being in the swarm for a queried info
// hash.
type PeersValues struct {
	Peers         []Peer // Peers given in get_peers response.
	krpc.NodeInfo        // The node that gave the response.
}

// Stop the announce.
func (a *Announce) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.close()
}

func (a *Announce) close() {
	select {
	case <-a.stop:
	default:
		close(a.stop)
	}
}
