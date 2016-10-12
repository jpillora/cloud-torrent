package dht

import (
	"crypto"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/anacrolix/missinggo"
	"github.com/tylertreat/BoomFilters"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/dht/krpc"
	"github.com/anacrolix/torrent/iplist"
	"github.com/anacrolix/torrent/logonce"
)

// A Server defines parameters for a DHT node server that is able to
// send queries, and respond to the ones from the network.
// Each node has a globally unique identifier known as the "node ID."
// Node IDs are chosen at random from the same 160-bit space
// as BitTorrent infohashes and define the behaviour of the node.
// Zero valued Server does not have a valid ID and thus
// is unable to function properly. Use `NewServer(nil)`
// to initialize a default node.
type Server struct {
	id               string
	socket           net.PacketConn
	transactions     map[transactionKey]*Transaction
	transactionIDInt uint64
	nodes            map[string]*node // Keyed by dHTAddr.String().
	mu               sync.Mutex
	closed           missinggo.Event
	ipBlockList      iplist.Ranger
	badNodes         *boom.BloomFilter

	numConfirmedAnnounces int
	bootstrapNodes        []string
	config                ServerConfig
}

// Stats returns statistics for the server.
func (s *Server) Stats() (ss ServerStats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range s.nodes {
		if n.DefinitelyGood() {
			ss.GoodNodes++
		}
	}
	ss.Nodes = len(s.nodes)
	ss.OutstandingTransactions = len(s.transactions)
	ss.ConfirmedAnnounces = s.numConfirmedAnnounces
	ss.BadNodes = s.badNodes.Count()
	return
}

// Addr returns the listen address for the server. Packets arriving to this address
// are processed by the server (unless aliens are involved).
func (s *Server) Addr() net.Addr {
	return s.socket.LocalAddr()
}

// NewServer initializes a new DHT node server.
func NewServer(c *ServerConfig) (s *Server, err error) {
	if c == nil {
		c = &ServerConfig{}
	}
	s = &Server{
		config:      *c,
		ipBlockList: c.IPBlocklist,
		badNodes:    boom.NewBloomFilter(1000, 0.1),
	}
	if c.Conn != nil {
		s.socket = c.Conn
	} else {
		s.socket, err = makeSocket(c.Addr)
		if err != nil {
			return
		}
	}
	s.bootstrapNodes = c.BootstrapNodes
	if c.NodeIdHex != "" {
		var rawID []byte
		rawID, err = hex.DecodeString(c.NodeIdHex)
		if err != nil {
			return
		}
		s.id = string(rawID)
	}
	err = s.init()
	if err != nil {
		return
	}
	go func() {
		err := s.serve()
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed.IsSet() {
			return
		}
		if err != nil {
			panic(err)
		}
	}()
	go func() {
		err := s.bootstrap()
		if err != nil {
			s.mu.Lock()
			if !s.closed.IsSet() {
				log.Printf("error bootstrapping DHT: %s", err)
			}
			s.mu.Unlock()
		}
	}()
	return
}

// Returns a description of the Server. Python repr-style.
func (s *Server) String() string {
	return fmt.Sprintf("dht server on %s", s.socket.LocalAddr())
}

// Packets to and from any address matching a range in the list are dropped.
func (s *Server) SetIPBlockList(list iplist.Ranger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ipBlockList = list
}

func (s *Server) IPBlocklist() iplist.Ranger {
	return s.ipBlockList
}

func (s *Server) init() (err error) {
	err = s.setDefaults()
	if err != nil {
		return
	}
	s.transactions = make(map[transactionKey]*Transaction)
	return
}

func (s *Server) processPacket(b []byte, addr Addr) {
	if len(b) < 2 || b[0] != 'd' || b[len(b)-1] != 'e' {
		// KRPC messages are bencoded dicts.
		readNotKRPCDict.Add(1)
		return
	}
	var d krpc.Msg
	err := bencode.Unmarshal(b, &d)
	if err != nil {
		readUnmarshalError.Add(1)
		func() {
			if se, ok := err.(*bencode.SyntaxError); ok {
				// The message was truncated.
				if int(se.Offset) == len(b) {
					return
				}
				// Some messages seem to drop to nul chars abrubtly.
				if int(se.Offset) < len(b) && b[se.Offset] == 0 {
					return
				}
				// The message isn't bencode from the first.
				if se.Offset == 0 {
					return
				}
			}
			// if missinggo.CryHeard() {
			// 	log.Printf("%s: received bad krpc message from %s: %s: %+q", s, addr, err, b)
			// }
		}()
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.IsSet() {
		return
	}
	if d.Y == "q" {
		readQuery.Add(1)
		s.handleQuery(addr, d)
		return
	}
	t := s.findResponseTransaction(d.T, addr)
	if t == nil {
		//log.Printf("unexpected message: %#v", d)
		return
	}
	node := s.getNode(addr, d.SenderID())
	node.lastGotResponse = time.Now()
	// TODO: Update node ID as this is an authoritative packet.
	go t.handleResponse(d)
	s.deleteTransaction(t)
}

func (s *Server) serve() error {
	var b [0x10000]byte
	for {
		n, addr, err := s.socket.ReadFrom(b[:])
		if err != nil {
			return err
		}
		read.Add(1)
		if n == len(b) {
			logonce.Stderr.Printf("received dht packet exceeds buffer size")
			continue
		}
		s.mu.Lock()
		blocked := s.ipBlocked(missinggo.AddrIP(addr))
		s.mu.Unlock()
		if blocked {
			readBlocked.Add(1)
			continue
		}
		s.processPacket(b[:n], NewAddr(addr.(*net.UDPAddr)))
	}
}

func (s *Server) ipBlocked(ip net.IP) (blocked bool) {
	if s.ipBlockList == nil {
		return
	}
	_, blocked = s.ipBlockList.Lookup(ip)
	return
}

// Adds directly to the node table.
func (s *Server) AddNode(ni krpc.NodeInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nodes == nil {
		s.nodes = make(map[string]*node)
	}
	s.getNode(NewAddr(ni.Addr), string(ni.ID[:]))
}

func (s *Server) nodeByID(id string) *node {
	for _, node := range s.nodes {
		if node.idString() == id {
			return node
		}
	}
	return nil
}

func (s *Server) handleQuery(source Addr, m krpc.Msg) {
	node := s.getNode(source, m.SenderID())
	node.lastGotQuery = time.Now()
	if s.config.OnQuery != nil {
		propagate := s.config.OnQuery(&m, source.UDPAddr())
		if !propagate {
			return
		}
	}
	// Don't respond.
	if s.config.Passive {
		return
	}
	args := m.A
	switch m.Q {
	case "ping":
		s.reply(source, m.T, krpc.Return{})
	case "get_peers": // TODO: Extract common behaviour with find_node.
		targetID := args.InfoHash
		if len(targetID) != 20 {
			break
		}
		var rNodes []krpc.NodeInfo
		// TODO: Reply with "values" list if we have peers instead.
		for _, node := range s.closestGoodNodes(8, targetID) {
			rNodes = append(rNodes, node.NodeInfo())
		}
		s.reply(source, m.T, krpc.Return{
			Nodes: rNodes,
			// TODO: Generate this dynamically, and store it for the source.
			Token: "hi",
		})
	case "find_node": // TODO: Extract common behaviour with get_peers.
		targetID := args.Target
		if len(targetID) != 20 {
			log.Printf("bad DHT query: %v", m)
			return
		}
		var rNodes []krpc.NodeInfo
		if node := s.nodeByID(targetID); node != nil {
			rNodes = append(rNodes, node.NodeInfo())
		} else {
			// This will probably cause a crash for IPv6, but meh.
			for _, node := range s.closestGoodNodes(8, targetID) {
				rNodes = append(rNodes, node.NodeInfo())
			}
		}
		s.reply(source, m.T, krpc.Return{
			Nodes: rNodes,
		})
	case "announce_peer":
		// TODO(anacrolix): Implement this lolz.
		// log.Print(m)
	case "vote":
		// TODO(anacrolix): Or reject, I don't think I want this.
	default:
		log.Printf("%s: not handling received query: q=%s", s, m.Q)
		return
	}
}

func (s *Server) reply(addr Addr, t string, r krpc.Return) {
	r.ID = s.ID()
	m := krpc.Msg{
		T: t,
		Y: "r",
		R: &r,
	}
	b, err := bencode.Marshal(m)
	if err != nil {
		panic(err)
	}
	err = s.writeToNode(b, addr)
	if err != nil {
		log.Printf("error replying to %s: %s", addr, err)
	}
}

// Returns a node struct for the addr. It is taken from the table or created
// and possibly added if required and meets validity constraints.
func (s *Server) getNode(addr Addr, id string) (n *node) {
	addrStr := addr.String()
	n = s.nodes[addrStr]
	if n != nil {
		if id != "" {
			n.SetIDFromString(id)
		}
		return
	}
	n = &node{
		addr: addr,
	}
	if len(id) == 20 {
		n.SetIDFromString(id)
	}
	if len(s.nodes) >= maxNodes {
		return
	}
	// Exclude insecure nodes from the node table.
	if !s.config.NoSecurity && !n.IsSecure() {
		return
	}
	if s.badNodes.Test([]byte(addrStr)) {
		return
	}
	s.nodes[addrStr] = n
	return
}

func (s *Server) nodeTimedOut(addr Addr) {
	node, ok := s.nodes[addr.String()]
	if !ok {
		return
	}
	if node.DefinitelyGood() {
		return
	}
	if len(s.nodes) < maxNodes {
		return
	}
	delete(s.nodes, addr.String())
}

func (s *Server) writeToNode(b []byte, node Addr) (err error) {
	if list := s.ipBlockList; list != nil {
		if r, ok := list.Lookup(missinggo.AddrIP(node.UDPAddr())); ok {
			err = fmt.Errorf("write to %s blocked: %s", node, r.Description)
			return
		}
	}
	n, err := s.socket.WriteTo(b, node.UDPAddr())
	writes.Add(1)
	if err != nil {
		writeErrors.Add(1)
		err = fmt.Errorf("error writing %d bytes to %s: %s", len(b), node, err)
		return
	}
	if n != len(b) {
		err = io.ErrShortWrite
		return
	}
	return
}

func (s *Server) findResponseTransaction(transactionID string, sourceNode Addr) *Transaction {
	return s.transactions[transactionKey{
		sourceNode.String(),
		transactionID}]
}

func (s *Server) nextTransactionID() string {
	var b [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(b[:], s.transactionIDInt)
	s.transactionIDInt++
	return string(b[:n])
}

func (s *Server) deleteTransaction(t *Transaction) {
	delete(s.transactions, t.key())
}

func (s *Server) addTransaction(t *Transaction) {
	if _, ok := s.transactions[t.key()]; ok {
		panic("transaction not unique")
	}
	s.transactions[t.key()] = t
}

// ID returns the 20-byte server ID. This is the ID used to communicate with the
// DHT network.
func (s *Server) ID() string {
	if len(s.id) != 20 {
		panic("bad node id")
	}
	return s.id
}

func (s *Server) query(node Addr, q string, a map[string]interface{}, onResponse func(krpc.Msg)) (t *Transaction, err error) {
	tid := s.nextTransactionID()
	if a == nil {
		a = make(map[string]interface{}, 1)
	}
	a["id"] = s.ID()
	d := map[string]interface{}{
		"t": tid,
		"y": "q",
		"q": q,
		"a": a,
	}
	// BEP 43. Outgoing queries from uncontactiable nodes should contain
	// "ro":1 in the top level dictionary.
	if s.config.Passive {
		d["ro"] = 1
	}
	b, err := bencode.Marshal(d)
	if err != nil {
		return
	}
	_t := &Transaction{
		remoteAddr:  node,
		t:           tid,
		response:    make(chan krpc.Msg, 1),
		done:        make(chan struct{}),
		queryPacket: b,
		s:           s,
		onResponse:  onResponse,
	}
	err = _t.sendQuery()
	if err != nil {
		return
	}
	s.getNode(node, "").lastSentQuery = time.Now()
	_t.mu.Lock()
	_t.startTimer()
	_t.mu.Unlock()
	s.addTransaction(_t)
	t = _t
	return
}

// Sends a ping query to the address given.
func (s *Server) Ping(node *net.UDPAddr) (*Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.query(NewAddr(node), "ping", nil, nil)
}

func (s *Server) announcePeer(node Addr, infoHash string, port int, token string, impliedPort bool) (err error) {
	if port == 0 && !impliedPort {
		return errors.New("nothing to announce")
	}
	_, err = s.query(node, "announce_peer", map[string]interface{}{
		"implied_port": func() int {
			if impliedPort {
				return 1
			} else {
				return 0
			}
		}(),
		"info_hash": infoHash,
		"port":      port,
		"token":     token,
	}, func(m krpc.Msg) {
		if err := m.Error(); err != nil {
			announceErrors.Add(1)
			// log.Print(token)
			// logonce.Stderr.Printf("announce_peer response: %s", err)
			return
		}
		s.numConfirmedAnnounces++
	})
	return
}

// Add response nodes to node table.
func (s *Server) liftNodes(d krpc.Msg) {
	if d.Y != "r" {
		return
	}
	for _, cni := range d.R.Nodes {
		if cni.Addr.Port == 0 {
			// TODO: Why would people even do this?
			continue
		}
		if s.ipBlocked(cni.Addr.IP) {
			continue
		}
		n := s.getNode(NewAddr(cni.Addr), string(cni.ID[:]))
		n.SetIDFromBytes(cni.ID[:])
	}
}

// Sends a find_node query to addr. targetID is the node we're looking for.
func (s *Server) findNode(addr Addr, targetID string) (t *Transaction, err error) {
	t, err = s.query(addr, "find_node", map[string]interface{}{"target": targetID}, func(d krpc.Msg) {
		// Scrape peers from the response to put in the server's table before
		// handing the response back to the caller.
		s.liftNodes(d)
	})
	if err != nil {
		return
	}
	return
}

// Adds bootstrap nodes directly to table, if there's room. Node ID security
// is bypassed, but the IP blocklist is not.
func (s *Server) addRootNodes() error {
	addrs, err := bootstrapAddrs(s.bootstrapNodes)
	if err != nil {
		return err
	}
	for _, addr := range addrs {
		if len(s.nodes) >= maxNodes {
			break
		}
		if s.nodes[addr.String()] != nil {
			continue
		}
		if s.ipBlocked(addr.IP) {
			log.Printf("dht root node is in the blocklist: %s", addr.IP)
			continue
		}
		s.nodes[addr.String()] = &node{
			addr: NewAddr(addr),
		}
	}
	return nil
}

// Populates the node table.
func (s *Server) bootstrap() (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.nodes) == 0 && !s.config.NoDefaultBootstrap {
		err = s.addRootNodes()
	}
	if err != nil {
		return
	}
	for {
		var outstanding sync.WaitGroup
		for _, node := range s.nodes {
			var t *Transaction
			t, err = s.findNode(node.addr, s.id)
			if err != nil {
				err = fmt.Errorf("error sending find_node: %s", err)
				return
			}
			outstanding.Add(1)
			t.SetResponseHandler(func(krpc.Msg, bool) {
				outstanding.Done()
			})
		}
		noOutstanding := make(chan struct{})
		go func() {
			outstanding.Wait()
			close(noOutstanding)
		}()
		s.mu.Unlock()
		select {
		case <-s.closed.LockedChan(&s.mu):
			s.mu.Lock()
			return
		case <-time.After(15 * time.Second):
		case <-noOutstanding:
		}
		s.mu.Lock()
		// log.Printf("now have %d nodes", len(s.nodes))
		if s.numGoodNodes() >= 160 {
			break
		}
	}
	return
}

func (s *Server) numGoodNodes() (num int) {
	for _, n := range s.nodes {
		if n.DefinitelyGood() {
			num++
		}
	}
	return
}

// Returns how many nodes are in the node table.
func (s *Server) NumNodes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.nodes)
}

// Exports the current node table.
func (s *Server) Nodes() (nis []krpc.NodeInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, node := range s.nodes {
		// if !node.Good() {
		// 	continue
		// }
		ni := krpc.NodeInfo{
			Addr: node.addr.UDPAddr(),
		}
		if n := copy(ni.ID[:], node.idString()); n != 20 && n != 0 {
			panic(n)
		}
		nis = append(nis, ni)
	}
	return
}

// Stops the server network activity. This is all that's required to clean-up a Server.
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed.Set()
	s.socket.Close()
}

func (s *Server) setDefaults() (err error) {
	if s.id == "" {
		var id [20]byte
		h := crypto.SHA1.New()
		ss, err := os.Hostname()
		if err != nil {
			log.Print(err)
		}
		ss += s.socket.LocalAddr().String()
		h.Write([]byte(ss))
		if b := h.Sum(id[:0:20]); len(b) != 20 {
			panic(len(b))
		}
		if len(id) != 20 {
			panic(len(id))
		}
		publicIP := func() net.IP {
			if s.config.PublicIP != nil {
				return s.config.PublicIP
			} else {
				return missinggo.AddrIP(s.socket.LocalAddr())
			}
		}()
		SecureNodeId(id[:], publicIP)
		s.id = string(id[:])
	}
	s.nodes = make(map[string]*node, maxNodes)
	return
}

func (s *Server) getPeers(addr Addr, infoHash string) (t *Transaction, err error) {
	if len(infoHash) != 20 {
		err = fmt.Errorf("infohash has bad length")
		return
	}
	t, err = s.query(addr, "get_peers", map[string]interface{}{"info_hash": infoHash}, func(m krpc.Msg) {
		s.liftNodes(m)
		if m.R != nil && m.R.Token != "" {
			s.getNode(addr, m.SenderID()).announceToken = m.R.Token
		}
	})
	return
}

func (s *Server) closestGoodNodes(k int, targetID string) []*node {
	return s.closestNodes(k, nodeIDFromString(targetID), func(n *node) bool { return n.DefinitelyGood() })
}

func (s *Server) closestNodes(k int, target nodeID, filter func(*node) bool) []*node {
	sel := newKClosestNodesSelector(k, target)
	idNodes := make(map[string]*node, len(s.nodes))
	for _, node := range s.nodes {
		if !filter(node) {
			continue
		}
		sel.Push(node.id)
		idNodes[node.idString()] = node
	}
	ids := sel.IDs()
	ret := make([]*node, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, idNodes[id.ByteString()])
	}
	return ret
}

func (s *Server) badNode(addr Addr) {
	s.badNodes.Add([]byte(addr.String()))
	delete(s.nodes, addr.String())
}
