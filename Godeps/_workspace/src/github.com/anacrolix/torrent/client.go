package torrent

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log"
	"math/big"
	mathRand "math/rand"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anacrolix/missinggo"
	. "github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/perf"
	"github.com/anacrolix/missinggo/pubsub"
	"github.com/anacrolix/sync"
	"github.com/anacrolix/utp"
	"github.com/bradfitz/iter"
	"github.com/edsrzf/mmap-go"

	"github.com/anacrolix/torrent/bencode"
	filePkg "github.com/anacrolix/torrent/data/file"
	"github.com/anacrolix/torrent/dht"
	"github.com/anacrolix/torrent/internal/pieceordering"
	"github.com/anacrolix/torrent/iplist"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/mse"
	pp "github.com/anacrolix/torrent/peer_protocol"
	"github.com/anacrolix/torrent/tracker"
)

var (
	unwantedChunksReceived   = expvar.NewInt("chunksReceivedUnwanted")
	unexpectedChunksReceived = expvar.NewInt("chunksReceivedUnexpected")
	chunksReceived           = expvar.NewInt("chunksReceived")

	peersAddedBySource = expvar.NewMap("peersAddedBySource")

	uploadChunksPosted    = expvar.NewInt("uploadChunksPosted")
	unexpectedCancels     = expvar.NewInt("unexpectedCancels")
	postedCancels         = expvar.NewInt("postedCancels")
	duplicateConnsAvoided = expvar.NewInt("duplicateConnsAvoided")

	pieceHashedCorrect    = expvar.NewInt("pieceHashedCorrect")
	pieceHashedNotCorrect = expvar.NewInt("pieceHashedNotCorrect")

	unsuccessfulDials = expvar.NewInt("dialSuccessful")
	successfulDials   = expvar.NewInt("dialUnsuccessful")

	acceptUTP    = expvar.NewInt("acceptUTP")
	acceptTCP    = expvar.NewInt("acceptTCP")
	acceptReject = expvar.NewInt("acceptReject")

	peerExtensions                    = expvar.NewMap("peerExtensions")
	completedHandshakeConnectionFlags = expvar.NewMap("completedHandshakeConnectionFlags")
	// Count of connections to peer with same client ID.
	connsToSelf = expvar.NewInt("connsToSelf")
	// Number of completed connections to a client we're already connected with.
	duplicateClientConns       = expvar.NewInt("duplicateClientConns")
	receivedMessageTypes       = expvar.NewMap("receivedMessageTypes")
	supportedExtensionMessages = expvar.NewMap("supportedExtensionMessages")
)

const (
	// Justification for set bits follows.
	//
	// Extension protocol ([5]|=0x10):
	// http://www.bittorrent.org/beps/bep_0010.html
	//
	// Fast Extension ([7]|=0x04):
	// http://bittorrent.org/beps/bep_0006.html.
	// Disabled until AllowedFast is implemented.
	//
	// DHT ([7]|=1):
	// http://www.bittorrent.org/beps/bep_0005.html
	defaultExtensionBytes = "\x00\x00\x00\x00\x00\x10\x00\x01"

	socketsPerTorrent     = 80
	torrentPeersHighWater = 200
	torrentPeersLowWater  = 50

	// Limit how long handshake can take. This is to reduce the lingering
	// impact of a few bad apples. 4s loses 1% of successful handshakes that
	// are obtained with 60s timeout, and 5% of unsuccessful handshakes.
	btHandshakeTimeout = 4 * time.Second
	handshakesTimeout  = 20 * time.Second

	// These are our extended message IDs.
	metadataExtendedId = iota + 1 // 0 is reserved for deleting keys
	pexExtendedId

	// Updated occasionally to when there's been some changes to client
	// behaviour in case other clients are assuming anything of us. See also
	// `bep20`.
	extendedHandshakeClientVersion = "go.torrent dev 20150624"
)

// Currently doesn't really queue, but should in the future.
func (cl *Client) queuePieceCheck(t *torrent, pieceIndex int) {
	piece := &t.Pieces[pieceIndex]
	if piece.QueuedForHash {
		return
	}
	piece.QueuedForHash = true
	t.publishPieceChange(int(pieceIndex))
	go cl.verifyPiece(t, int(pieceIndex))
}

// Queue a piece check if one isn't already queued, and the piece has never
// been checked before.
func (cl *Client) queueFirstHash(t *torrent, piece int) {
	p := &t.Pieces[piece]
	if p.EverHashed || p.Hashing || p.QueuedForHash || t.pieceComplete(piece) {
		return
	}
	cl.queuePieceCheck(t, piece)
}

// Clients contain zero or more Torrents. A client manages a blocklist, the
// TCP/UDP protocol ports, and DHT as desired.
type Client struct {
	halfOpenLimit  int
	peerID         [20]byte
	listeners      []net.Listener
	utpSock        *utp.Socket
	dHT            *dht.Server
	ipBlockList    iplist.Ranger
	bannedTorrents map[InfoHash]struct{}
	config         Config
	pruneTimer     *time.Timer
	extensionBytes peerExtensionBytes
	// Set of addresses that have our client ID. This intentionally will
	// include ourselves if we end up trying to connect to our own address
	// through legitimate channels.
	dopplegangerAddrs map[string]struct{}

	torrentDataOpener TorrentDataOpener

	mu    sync.RWMutex
	event sync.Cond
	quit  chan struct{}

	torrents map[InfoHash]*torrent
}

func (me *Client) IPBlockList() iplist.Ranger {
	me.mu.Lock()
	defer me.mu.Unlock()
	return me.ipBlockList
}

func (me *Client) SetIPBlockList(list iplist.Ranger) {
	me.mu.Lock()
	defer me.mu.Unlock()
	me.ipBlockList = list
	if me.dHT != nil {
		me.dHT.SetIPBlockList(list)
	}
}

func (me *Client) PeerID() string {
	return string(me.peerID[:])
}

func (me *Client) ListenAddr() (addr net.Addr) {
	for _, l := range me.listeners {
		addr = l.Addr()
		break
	}
	return
}

type hashSorter struct {
	Hashes []InfoHash
}

func (me hashSorter) Len() int {
	return len(me.Hashes)
}

func (me hashSorter) Less(a, b int) bool {
	return (&big.Int{}).SetBytes(me.Hashes[a][:]).Cmp((&big.Int{}).SetBytes(me.Hashes[b][:])) < 0
}

func (me hashSorter) Swap(a, b int) {
	me.Hashes[a], me.Hashes[b] = me.Hashes[b], me.Hashes[a]
}

func (cl *Client) sortedTorrents() (ret []*torrent) {
	var hs hashSorter
	for ih := range cl.torrents {
		hs.Hashes = append(hs.Hashes, ih)
	}
	sort.Sort(hs)
	for _, ih := range hs.Hashes {
		ret = append(ret, cl.torrent(ih))
	}
	return
}

// Writes out a human readable status of the client, such as for writing to a
// HTTP status page.
func (cl *Client) WriteStatus(_w io.Writer) {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	w := bufio.NewWriter(_w)
	defer w.Flush()
	if addr := cl.ListenAddr(); addr != nil {
		fmt.Fprintf(w, "Listening on %s\n", cl.ListenAddr())
	} else {
		fmt.Fprintln(w, "Not listening!")
	}
	fmt.Fprintf(w, "Peer ID: %+q\n", cl.peerID)
	if cl.dHT != nil {
		dhtStats := cl.dHT.Stats()
		fmt.Fprintf(w, "DHT nodes: %d (%d good, %d banned)\n", dhtStats.Nodes, dhtStats.GoodNodes, dhtStats.BadNodes)
		fmt.Fprintf(w, "DHT Server ID: %x\n", cl.dHT.ID())
		fmt.Fprintf(w, "DHT port: %d\n", addrPort(cl.dHT.Addr()))
		fmt.Fprintf(w, "DHT announces: %d\n", dhtStats.ConfirmedAnnounces)
		fmt.Fprintf(w, "Outstanding transactions: %d\n", dhtStats.OutstandingTransactions)
	}
	fmt.Fprintf(w, "# Torrents: %d\n", len(cl.torrents))
	fmt.Fprintln(w)
	for _, t := range cl.sortedTorrents() {
		if t.Name() == "" {
			fmt.Fprint(w, "<unknown name>")
		} else {
			fmt.Fprint(w, t.Name())
		}
		fmt.Fprint(w, "\n")
		if t.haveInfo() {
			fmt.Fprintf(w, "%f%% of %d bytes", 100*(1-float32(t.bytesLeft())/float32(t.length)), t.length)
		} else {
			w.WriteString("<missing metainfo>")
		}
		fmt.Fprint(w, "\n")
		t.writeStatus(w, cl)
		fmt.Fprintln(w)
	}
}

func dataReadAt(d Data, b []byte, off int64) (n int, err error) {
	// defer func() {
	// 	if err == io.ErrUnexpectedEOF && n != 0 {
	// 		err = nil
	// 	}
	// }()
	// log.Println("data read at", len(b), off)
	return d.ReadAt(b, off)
}

// Calculates the number of pieces to set to Readahead priority, after the
// Now, and Next pieces.
func readaheadPieces(readahead, pieceLength int64) (ret int) {
	// Expand the readahead to fit any partial pieces. Subtract 1 for the
	// "next" piece that is assigned.
	ret = int((readahead+pieceLength-1)/pieceLength - 1)
	// Lengthen the "readahead tail" to smooth blockiness that occurs when the
	// piece length is much larger than the readahead.
	if ret < 2 {
		ret++
	}
	return
}

func (cl *Client) readRaisePiecePriorities(t *torrent, off, readaheadBytes int64) {
	index := int(off / int64(t.usualPieceSize()))
	cl.raisePiecePriority(t, index, PiecePriorityNow)
	index++
	if index >= t.numPieces() {
		return
	}
	cl.raisePiecePriority(t, index, PiecePriorityNext)
	for range iter.N(readaheadPieces(readaheadBytes, t.Info.PieceLength)) {
		index++
		if index >= t.numPieces() {
			break
		}
		cl.raisePiecePriority(t, index, PiecePriorityReadahead)
	}
}

func (cl *Client) addUrgentRequests(t *torrent, off int64, n int) {
	for n > 0 {
		req, ok := t.offsetRequest(off)
		if !ok {
			break
		}
		if _, ok := t.urgent[req]; !ok && !t.haveChunk(req) {
			if t.urgent == nil {
				t.urgent = make(map[request]struct{}, (n+int(t.chunkSize)-1)/int(t.chunkSize))
			}
			t.urgent[req] = struct{}{}
			cl.event.Broadcast() // Why?
			index := int(req.Index)
			cl.queueFirstHash(t, index)
			cl.pieceChanged(t, index)
		}
		reqOff := t.requestOffset(req)
		n1 := req.Length - pp.Integer(off-reqOff)
		off += int64(n1)
		n -= int(n1)
	}
	// log.Print(t.urgent)
}

func (cl *Client) configDir() string {
	if cl.config.ConfigDir == "" {
		return filepath.Join(os.Getenv("HOME"), ".config/torrent")
	}
	return cl.config.ConfigDir
}

// The directory where the Client expects to find and store configuration
// data. Defaults to $HOME/.config/torrent.
func (cl *Client) ConfigDir() string {
	return cl.configDir()
}

func (t *torrent) connPendPiece(c *connection, piece int) {
	c.pendPiece(piece, t.Pieces[piece].Priority, t)
}

func (cl *Client) raisePiecePriority(t *torrent, piece int, priority piecePriority) {
	if t.Pieces[piece].Priority < priority {
		cl.prioritizePiece(t, piece, priority)
	}
}

func (cl *Client) prioritizePiece(t *torrent, piece int, priority piecePriority) {
	if t.havePiece(piece) {
		priority = PiecePriorityNone
	}
	if priority != PiecePriorityNone {
		cl.queueFirstHash(t, piece)
	}
	p := &t.Pieces[piece]
	if p.Priority != priority {
		p.Priority = priority
		cl.pieceChanged(t, piece)
	}
}

func loadPackedBlocklist(filename string) (ret iplist.Ranger, err error) {
	f, err := os.Open(filename)
	if os.IsNotExist(err) {
		err = nil
		return
	}
	if err != nil {
		return
	}
	defer f.Close()
	mm, err := mmap.Map(f, mmap.RDONLY, 0)
	if err != nil {
		return
	}
	ret = iplist.NewFromPacked(mm)
	return
}

func (cl *Client) setEnvBlocklist() (err error) {
	filename := os.Getenv("TORRENT_BLOCKLIST_FILE")
	defaultBlocklist := filename == ""
	if defaultBlocklist {
		cl.ipBlockList, err = loadPackedBlocklist(filepath.Join(cl.configDir(), "packed-blocklist"))
		if err != nil {
			return
		}
		if cl.ipBlockList != nil {
			return
		}
		filename = filepath.Join(cl.configDir(), "blocklist")
	}
	f, err := os.Open(filename)
	if err != nil {
		if defaultBlocklist {
			err = nil
		}
		return
	}
	defer f.Close()
	cl.ipBlockList, err = iplist.NewFromReader(f)
	return
}

func (cl *Client) initBannedTorrents() error {
	f, err := os.Open(filepath.Join(cl.configDir(), "banned_infohashes"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("error opening banned infohashes file: %s", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	cl.bannedTorrents = make(map[InfoHash]struct{})
	for scanner.Scan() {
		if strings.HasPrefix(strings.TrimSpace(scanner.Text()), "#") {
			continue
		}
		var ihs string
		n, err := fmt.Sscanf(scanner.Text(), "%x", &ihs)
		if err != nil {
			return fmt.Errorf("error reading infohash: %s", err)
		}
		if n != 1 {
			continue
		}
		if len(ihs) != 20 {
			return errors.New("bad infohash")
		}
		var ih InfoHash
		CopyExact(&ih, ihs)
		cl.bannedTorrents[ih] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error scanning file: %s", err)
	}
	return nil
}

// Creates a new client.
func NewClient(cfg *Config) (cl *Client, err error) {
	if cfg == nil {
		cfg = &Config{}
	}

	defer func() {
		if err != nil {
			cl = nil
		}
	}()
	cl = &Client{
		halfOpenLimit: socketsPerTorrent,
		config:        *cfg,
		torrentDataOpener: func(md *metainfo.Info) Data {
			return filePkg.TorrentData(md, cfg.DataDir)
		},
		dopplegangerAddrs: make(map[string]struct{}),

		quit:     make(chan struct{}),
		torrents: make(map[InfoHash]*torrent),
	}
	CopyExact(&cl.extensionBytes, defaultExtensionBytes)
	cl.event.L = &cl.mu
	if cfg.TorrentDataOpener != nil {
		cl.torrentDataOpener = cfg.TorrentDataOpener
	}

	if cfg.IPBlocklist != nil {
		cl.ipBlockList = cfg.IPBlocklist
	} else if !cfg.NoDefaultBlocklist {
		err = cl.setEnvBlocklist()
		if err != nil {
			return
		}
	}

	if err = cl.initBannedTorrents(); err != nil {
		err = fmt.Errorf("error initing banned torrents: %s", err)
		return
	}

	if cfg.PeerID != "" {
		CopyExact(&cl.peerID, cfg.PeerID)
	} else {
		o := copy(cl.peerID[:], bep20)
		_, err = rand.Read(cl.peerID[o:])
		if err != nil {
			panic("error generating peer id")
		}
	}

	// Returns the laddr string to listen on for the next Listen call.
	listenAddr := func() string {
		if addr := cl.ListenAddr(); addr != nil {
			return addr.String()
		}
		if cfg.ListenAddr == "" {
			return ":50007"
		}
		return cfg.ListenAddr
	}
	if !cl.config.DisableTCP {
		var l net.Listener
		l, err = net.Listen(func() string {
			if cl.config.DisableIPv6 {
				return "tcp4"
			} else {
				return "tcp"
			}
		}(), listenAddr())
		if err != nil {
			return
		}
		cl.listeners = append(cl.listeners, l)
		go cl.acceptConnections(l, false)
	}
	if !cl.config.DisableUTP {
		cl.utpSock, err = utp.NewSocket(func() string {
			if cl.config.DisableIPv6 {
				return "udp4"
			} else {
				return "udp"
			}
		}(), listenAddr())
		if err != nil {
			return
		}
		cl.listeners = append(cl.listeners, cl.utpSock)
		go cl.acceptConnections(cl.utpSock, true)
	}
	if !cfg.NoDHT {
		dhtCfg := cfg.DHTConfig
		if dhtCfg.IPBlocklist == nil {
			dhtCfg.IPBlocklist = cl.ipBlockList
		}
		if dhtCfg.Addr == "" {
			dhtCfg.Addr = listenAddr()
		}
		if dhtCfg.Conn == nil && cl.utpSock != nil {
			dhtCfg.Conn = cl.utpSock
		}
		cl.dHT, err = dht.NewServer(&dhtCfg)
		if err != nil {
			return
		}
	}

	return
}

func (cl *Client) stopped() bool {
	select {
	case <-cl.quit:
		return true
	default:
		return false
	}
}

// Stops the client. All connections to peers are closed and all activity will
// come to a halt.
func (me *Client) Close() {
	me.mu.Lock()
	defer me.mu.Unlock()
	select {
	case <-me.quit:
		return
	default:
	}
	close(me.quit)
	if me.dHT != nil {
		me.dHT.Close()
	}
	for _, l := range me.listeners {
		l.Close()
	}
	for _, t := range me.torrents {
		t.close()
	}
	me.event.Broadcast()
}

var ipv6BlockRange = iplist.Range{Description: "non-IPv4 address"}

func (cl *Client) ipBlockRange(ip net.IP) (r iplist.Range, blocked bool) {
	if cl.ipBlockList == nil {
		return
	}
	ip4 := ip.To4()
	// If blocklists are enabled, then block non-IPv4 addresses, because
	// blocklists do not yet support IPv6.
	if ip4 == nil {
		if missinggo.CryHeard() {
			log.Printf("blocking non-IPv4 address: %s", ip)
		}
		r = ipv6BlockRange
		blocked = true
		return
	}
	return cl.ipBlockList.Lookup(ip4)
}

func (cl *Client) waitAccept() {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	for {
		for _, t := range cl.torrents {
			if cl.wantConns(t) {
				return
			}
		}
		select {
		case <-cl.quit:
			return
		default:
		}
		cl.event.Wait()
	}
}

func (cl *Client) acceptConnections(l net.Listener, utp bool) {
	for {
		cl.waitAccept()
		// We accept all connections immediately, because we don't know what
		// torrent they're for.
		conn, err := l.Accept()
		select {
		case <-cl.quit:
			if conn != nil {
				conn.Close()
			}
			return
		default:
		}
		if err != nil {
			log.Print(err)
			return
		}
		if utp {
			acceptUTP.Add(1)
		} else {
			acceptTCP.Add(1)
		}
		cl.mu.RLock()
		doppleganger := cl.dopplegangerAddr(conn.RemoteAddr().String())
		_, blocked := cl.ipBlockRange(AddrIP(conn.RemoteAddr()))
		cl.mu.RUnlock()
		if blocked || doppleganger {
			acceptReject.Add(1)
			// log.Printf("inbound connection from %s blocked by %s", conn.RemoteAddr(), blockRange)
			conn.Close()
			continue
		}
		go cl.incomingConnection(conn, utp)
	}
}

func (cl *Client) incomingConnection(nc net.Conn, utp bool) {
	defer nc.Close()
	if tc, ok := nc.(*net.TCPConn); ok {
		tc.SetLinger(0)
	}
	c := newConnection()
	c.conn = nc
	c.rw = nc
	c.Discovery = peerSourceIncoming
	c.uTP = utp
	err := cl.runReceivedConn(c)
	if err != nil {
		// log.Print(err)
	}
}

// Returns a handle to the given torrent, if it's present in the client.
func (cl *Client) Torrent(ih InfoHash) (T Torrent, ok bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	t, ok := cl.torrents[ih]
	if !ok {
		return
	}
	T = Torrent{cl, t}
	return
}

func (me *Client) torrent(ih InfoHash) *torrent {
	return me.torrents[ih]
}

type dialResult struct {
	Conn net.Conn
	UTP  bool
}

func doDial(dial func(addr string, t *torrent) (net.Conn, error), ch chan dialResult, utp bool, addr string, t *torrent) {
	conn, err := dial(addr, t)
	if err != nil {
		if conn != nil {
			conn.Close()
		}
		conn = nil // Pedantic
	}
	ch <- dialResult{conn, utp}
	if err == nil {
		successfulDials.Add(1)
		return
	}
	unsuccessfulDials.Add(1)
}

func reducedDialTimeout(max time.Duration, halfOpenLimit int, pendingPeers int) (ret time.Duration) {
	ret = max / time.Duration((pendingPeers+halfOpenLimit)/halfOpenLimit)
	if ret < minDialTimeout {
		ret = minDialTimeout
	}
	return
}

// Returns whether an address is known to connect to a client with our own ID.
func (me *Client) dopplegangerAddr(addr string) bool {
	_, ok := me.dopplegangerAddrs[addr]
	return ok
}

// Start the process of connecting to the given peer for the given torrent if
// appropriate.
func (me *Client) initiateConn(peer Peer, t *torrent) {
	if peer.Id == me.peerID {
		return
	}
	addr := net.JoinHostPort(peer.IP.String(), fmt.Sprintf("%d", peer.Port))
	if me.dopplegangerAddr(addr) || t.addrActive(addr) {
		duplicateConnsAvoided.Add(1)
		return
	}
	if r, ok := me.ipBlockRange(peer.IP); ok {
		log.Printf("outbound connect to %s blocked by IP blocklist rule %s", peer.IP, r)
		return
	}
	t.HalfOpen[addr] = struct{}{}
	go me.outgoingConnection(t, addr, peer.Source)
}

func (me *Client) dialTimeout(t *torrent) time.Duration {
	me.mu.Lock()
	pendingPeers := len(t.Peers)
	me.mu.Unlock()
	return reducedDialTimeout(nominalDialTimeout, me.halfOpenLimit, pendingPeers)
}

func (me *Client) dialTCP(addr string, t *torrent) (c net.Conn, err error) {
	c, err = net.DialTimeout("tcp", addr, me.dialTimeout(t))
	if err == nil {
		c.(*net.TCPConn).SetLinger(0)
	}
	return
}

func (me *Client) dialUTP(addr string, t *torrent) (c net.Conn, err error) {
	return me.utpSock.DialTimeout(addr, me.dialTimeout(t))
}

// Returns a connection over UTP or TCP, whichever is first to connect.
func (me *Client) dialFirst(addr string, t *torrent) (conn net.Conn, utp bool) {
	// Initiate connections via TCP and UTP simultaneously. Use the first one
	// that succeeds.
	left := 0
	if !me.config.DisableUTP {
		left++
	}
	if !me.config.DisableTCP {
		left++
	}
	resCh := make(chan dialResult, left)
	if !me.config.DisableUTP {
		go doDial(me.dialUTP, resCh, true, addr, t)
	}
	if !me.config.DisableTCP {
		go doDial(me.dialTCP, resCh, false, addr, t)
	}
	var res dialResult
	// Wait for a successful connection.
	for ; left > 0 && res.Conn == nil; left-- {
		res = <-resCh
	}
	if left > 0 {
		// There are still incompleted dials.
		go func() {
			for ; left > 0; left-- {
				conn := (<-resCh).Conn
				if conn != nil {
					conn.Close()
				}
			}
		}()
	}
	conn = res.Conn
	utp = res.UTP
	return
}

func (me *Client) noLongerHalfOpen(t *torrent, addr string) {
	if _, ok := t.HalfOpen[addr]; !ok {
		panic("invariant broken")
	}
	delete(t.HalfOpen, addr)
	me.openNewConns(t)
}

// Performs initiator handshakes and returns a connection.
func (me *Client) handshakesConnection(nc net.Conn, t *torrent, encrypted, utp bool) (c *connection, err error) {
	c = newConnection()
	c.conn = nc
	c.rw = nc
	c.encrypted = encrypted
	c.uTP = utp
	err = nc.SetDeadline(time.Now().Add(handshakesTimeout))
	if err != nil {
		return
	}
	ok, err := me.initiateHandshakes(c, t)
	if !ok {
		c = nil
	}
	return
}

// Returns nil connection and nil error if no connection could be established
// for valid reasons.
func (me *Client) establishOutgoingConn(t *torrent, addr string) (c *connection, err error) {
	nc, utp := me.dialFirst(addr, t)
	if nc == nil {
		return
	}
	c, err = me.handshakesConnection(nc, t, !me.config.DisableEncryption, utp)
	if err != nil {
		nc.Close()
		return
	} else if c != nil {
		return
	}
	nc.Close()
	if me.config.DisableEncryption {
		// We already tried without encryption.
		return
	}
	// Try again without encryption, using whichever protocol type worked last
	// time.
	if utp {
		nc, err = me.dialUTP(addr, t)
	} else {
		nc, err = me.dialTCP(addr, t)
	}
	if err != nil {
		err = fmt.Errorf("error dialing for unencrypted connection: %s", err)
		return
	}
	c, err = me.handshakesConnection(nc, t, false, utp)
	if err != nil {
		nc.Close()
	}
	return
}

// Called to dial out and run a connection. The addr we're given is already
// considered half-open.
func (me *Client) outgoingConnection(t *torrent, addr string, ps peerSource) {
	c, err := me.establishOutgoingConn(t, addr)
	me.mu.Lock()
	defer me.mu.Unlock()
	// Don't release lock between here and addConnection, unless it's for
	// failure.
	me.noLongerHalfOpen(t, addr)
	if err != nil {
		return
	}
	if c == nil {
		return
	}
	defer c.Close()
	c.Discovery = ps
	err = me.runInitiatedHandshookConn(c, t)
	if err != nil {
		// log.Print(err)
	}
}

// The port number for incoming peer connections. 0 if the client isn't
// listening.
func (cl *Client) incomingPeerPort() int {
	listenAddr := cl.ListenAddr()
	if listenAddr == nil {
		return 0
	}
	return addrPort(listenAddr)
}

// Convert a net.Addr to its compact IP representation. Either 4 or 16 bytes
// per "yourip" field of http://www.bittorrent.org/beps/bep_0010.html.
func addrCompactIP(addr net.Addr) (string, error) {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "", err
	}
	ip := net.ParseIP(host)
	if v4 := ip.To4(); v4 != nil {
		if len(v4) != 4 {
			panic(v4)
		}
		return string(v4), nil
	}
	return string(ip.To16()), nil
}

func handshakeWriter(w io.Writer, bb <-chan []byte, done chan<- error) {
	var err error
	for b := range bb {
		_, err = w.Write(b)
		if err != nil {
			break
		}
	}
	done <- err
}

type (
	peerExtensionBytes [8]byte
	peerID             [20]byte
)

func (me *peerExtensionBytes) SupportsExtended() bool {
	return me[5]&0x10 != 0
}

func (me *peerExtensionBytes) SupportsDHT() bool {
	return me[7]&0x01 != 0
}

func (me *peerExtensionBytes) SupportsFast() bool {
	return me[7]&0x04 != 0
}

type handshakeResult struct {
	peerExtensionBytes
	peerID
	InfoHash
}

// ih is nil if we expect the peer to declare the InfoHash, such as when the
// peer initiated the connection. Returns ok if the handshake was successful,
// and err if there was an unexpected condition other than the peer simply
// abandoning the handshake.
func handshake(sock io.ReadWriter, ih *InfoHash, peerID [20]byte, extensions peerExtensionBytes) (res handshakeResult, ok bool, err error) {
	// Bytes to be sent to the peer. Should never block the sender.
	postCh := make(chan []byte, 4)
	// A single error value sent when the writer completes.
	writeDone := make(chan error, 1)
	// Performs writes to the socket and ensures posts don't block.
	go handshakeWriter(sock, postCh, writeDone)

	defer func() {
		close(postCh) // Done writing.
		if !ok {
			return
		}
		if err != nil {
			panic(err)
		}
		// Wait until writes complete before returning from handshake.
		err = <-writeDone
		if err != nil {
			err = fmt.Errorf("error writing: %s", err)
		}
	}()

	post := func(bb []byte) {
		select {
		case postCh <- bb:
		default:
			panic("mustn't block while posting")
		}
	}

	post([]byte(pp.Protocol))
	post(extensions[:])
	if ih != nil { // We already know what we want.
		post(ih[:])
		post(peerID[:])
	}
	var b [68]byte
	_, err = io.ReadFull(sock, b[:68])
	if err != nil {
		err = nil
		return
	}
	if string(b[:20]) != pp.Protocol {
		return
	}
	CopyExact(&res.peerExtensionBytes, b[20:28])
	CopyExact(&res.InfoHash, b[28:48])
	CopyExact(&res.peerID, b[48:68])
	peerExtensions.Add(hex.EncodeToString(res.peerExtensionBytes[:]), 1)

	// TODO: Maybe we can just drop peers here if we're not interested. This
	// could prevent them trying to reconnect, falsely believing there was
	// just a problem.
	if ih == nil { // We were waiting for the peer to tell us what they wanted.
		post(res.InfoHash[:])
		post(peerID[:])
	}

	ok = true
	return
}

// Wraps a raw connection and provides the interface we want for using the
// connection in the message loop.
type deadlineReader struct {
	nc net.Conn
	r  io.Reader
}

func (me deadlineReader) Read(b []byte) (n int, err error) {
	// Keep-alives should be received every 2 mins. Give a bit of gracetime.
	err = me.nc.SetReadDeadline(time.Now().Add(150 * time.Second))
	if err != nil {
		err = fmt.Errorf("error setting read deadline: %s", err)
	}
	n, err = me.r.Read(b)
	// Convert common errors into io.EOF.
	// if err != nil {
	// 	if opError, ok := err.(*net.OpError); ok && opError.Op == "read" && opError.Err == syscall.ECONNRESET {
	// 		err = io.EOF
	// 	} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
	// 		if n != 0 {
	// 			panic(n)
	// 		}
	// 		err = io.EOF
	// 	}
	// }
	return
}

type readWriter struct {
	io.Reader
	io.Writer
}

func maybeReceiveEncryptedHandshake(rw io.ReadWriter, skeys [][]byte) (ret io.ReadWriter, encrypted bool, err error) {
	var protocol [len(pp.Protocol)]byte
	_, err = io.ReadFull(rw, protocol[:])
	if err != nil {
		return
	}
	ret = readWriter{
		io.MultiReader(bytes.NewReader(protocol[:]), rw),
		rw,
	}
	if string(protocol[:]) == pp.Protocol {
		return
	}
	encrypted = true
	ret, err = mse.ReceiveHandshake(ret, skeys)
	return
}

func (cl *Client) receiveSkeys() (ret [][]byte) {
	for ih := range cl.torrents {
		ret = append(ret, ih[:])
	}
	return
}

func (me *Client) initiateHandshakes(c *connection, t *torrent) (ok bool, err error) {
	if c.encrypted {
		c.rw, err = mse.InitiateHandshake(c.rw, t.InfoHash[:], nil)
		if err != nil {
			return
		}
	}
	ih, ok, err := me.connBTHandshake(c, &t.InfoHash)
	if ih != t.InfoHash {
		ok = false
	}
	return
}

// Do encryption and bittorrent handshakes as receiver.
func (cl *Client) receiveHandshakes(c *connection) (t *torrent, err error) {
	cl.mu.Lock()
	skeys := cl.receiveSkeys()
	cl.mu.Unlock()
	if !cl.config.DisableEncryption {
		c.rw, c.encrypted, err = maybeReceiveEncryptedHandshake(c.rw, skeys)
		if err != nil {
			if err == mse.ErrNoSecretKeyMatch {
				err = nil
			}
			return
		}
	}
	ih, ok, err := cl.connBTHandshake(c, nil)
	if err != nil {
		err = fmt.Errorf("error during bt handshake: %s", err)
		return
	}
	if !ok {
		return
	}
	cl.mu.Lock()
	t = cl.torrents[ih]
	cl.mu.Unlock()
	return
}

// Returns !ok if handshake failed for valid reasons.
func (cl *Client) connBTHandshake(c *connection, ih *InfoHash) (ret InfoHash, ok bool, err error) {
	res, ok, err := handshake(c.rw, ih, cl.peerID, cl.extensionBytes)
	if err != nil || !ok {
		return
	}
	ret = res.InfoHash
	c.PeerExtensionBytes = res.peerExtensionBytes
	c.PeerID = res.peerID
	c.completedHandshake = time.Now()
	return
}

func (cl *Client) runInitiatedHandshookConn(c *connection, t *torrent) (err error) {
	if c.PeerID == cl.peerID {
		// Only if we initiated the connection is the remote address a
		// listen addr for a doppleganger.
		connsToSelf.Add(1)
		addr := c.conn.RemoteAddr().String()
		cl.dopplegangerAddrs[addr] = struct{}{}
		return
	}
	return cl.runHandshookConn(c, t)
}

func (cl *Client) runReceivedConn(c *connection) (err error) {
	err = c.conn.SetDeadline(time.Now().Add(handshakesTimeout))
	if err != nil {
		return
	}
	t, err := cl.receiveHandshakes(c)
	if err != nil {
		err = fmt.Errorf("error receiving handshakes: %s", err)
		return
	}
	if t == nil {
		return
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if c.PeerID == cl.peerID {
		return
	}
	return cl.runHandshookConn(c, t)
}

func (cl *Client) runHandshookConn(c *connection, t *torrent) (err error) {
	c.conn.SetWriteDeadline(time.Time{})
	c.rw = readWriter{
		deadlineReader{c.conn, c.rw},
		c.rw,
	}
	completedHandshakeConnectionFlags.Add(c.connectionFlags(), 1)
	if !cl.addConnection(t, c) {
		return
	}
	defer cl.dropConnection(t, c)
	go c.writer()
	go c.writeOptimizer(time.Minute)
	cl.sendInitialMessages(c, t)
	if t.haveInfo() {
		t.initRequestOrdering(c)
	}
	err = cl.connectionLoop(t, c)
	if err != nil {
		err = fmt.Errorf("error during connection loop: %s", err)
	}
	return
}

func (me *Client) sendInitialMessages(conn *connection, torrent *torrent) {
	if conn.PeerExtensionBytes.SupportsExtended() && me.extensionBytes.SupportsExtended() {
		conn.Post(pp.Message{
			Type:       pp.Extended,
			ExtendedID: pp.HandshakeExtendedID,
			ExtendedPayload: func() []byte {
				d := map[string]interface{}{
					"m": func() (ret map[string]int) {
						ret = make(map[string]int, 2)
						ret["ut_metadata"] = metadataExtendedId
						if !me.config.DisablePEX {
							ret["ut_pex"] = pexExtendedId
						}
						return
					}(),
					"v": extendedHandshakeClientVersion,
					// No upload queue is implemented yet.
					"reqq": 64,
				}
				if !me.config.DisableEncryption {
					d["e"] = 1
				}
				if torrent.metadataSizeKnown() {
					d["metadata_size"] = torrent.metadataSize()
				}
				if p := me.incomingPeerPort(); p != 0 {
					d["p"] = p
				}
				yourip, err := addrCompactIP(conn.remoteAddr())
				if err != nil {
					log.Printf("error calculating yourip field value in extension handshake: %s", err)
				} else {
					d["yourip"] = yourip
				}
				// log.Printf("sending %v", d)
				b, err := bencode.Marshal(d)
				if err != nil {
					panic(err)
				}
				return b
			}(),
		})
	}
	if torrent.haveAnyPieces() {
		conn.Bitfield(torrent.bitfield())
	} else if me.extensionBytes.SupportsFast() && conn.PeerExtensionBytes.SupportsFast() {
		conn.Post(pp.Message{
			Type: pp.HaveNone,
		})
	}
	if conn.PeerExtensionBytes.SupportsDHT() && me.extensionBytes.SupportsDHT() && me.dHT != nil {
		conn.Post(pp.Message{
			Type: pp.Port,
			Port: uint16(AddrPort(me.dHT.Addr())),
		})
	}
}

// Randomizes the piece order for this connection. Every connection will be
// given a different ordering. Having it stored per connection saves having to
// randomize during request filling, and constantly recalculate the ordering
// based on piece priorities.
func (t *torrent) initRequestOrdering(c *connection) {
	if c.pieceRequestOrder != nil || c.piecePriorities != nil {
		panic("double init of request ordering")
	}
	c.pieceRequestOrder = pieceordering.New()
	for i := range iter.N(t.Info.NumPieces()) {
		if !c.PeerHasPiece(i) {
			continue
		}
		if !t.wantPiece(i) {
			continue
		}
		t.connPendPiece(c, i)
	}
}

func (me *Client) peerGotPiece(t *torrent, c *connection, piece int) error {
	if !c.peerHasAll {
		if t.haveInfo() {
			if c.PeerPieces == nil {
				c.PeerPieces = make([]bool, t.numPieces())
			}
		} else {
			for piece >= len(c.PeerPieces) {
				c.PeerPieces = append(c.PeerPieces, false)
			}
		}
		if piece >= len(c.PeerPieces) {
			return errors.New("peer got out of range piece index")
		}
		c.PeerPieces[piece] = true
	}
	if t.wantPiece(piece) {
		t.connPendPiece(c, piece)
		me.replenishConnRequests(t, c)
	}
	return nil
}

func (me *Client) peerUnchoked(torrent *torrent, conn *connection) {
	me.replenishConnRequests(torrent, conn)
}

func (cl *Client) connCancel(t *torrent, cn *connection, r request) (ok bool) {
	ok = cn.Cancel(r)
	if ok {
		postedCancels.Add(1)
	}
	return
}

func (cl *Client) connDeleteRequest(t *torrent, cn *connection, r request) bool {
	if !cn.RequestPending(r) {
		return false
	}
	delete(cn.Requests, r)
	return true
}

func (cl *Client) requestPendingMetadata(t *torrent, c *connection) {
	if t.haveInfo() {
		return
	}
	if c.PeerExtensionIDs["ut_metadata"] == 0 {
		// Peer doesn't support this.
		return
	}
	// Request metadata pieces that we don't have in a random order.
	var pending []int
	for index := 0; index < t.metadataPieceCount(); index++ {
		if !t.haveMetadataPiece(index) && !c.requestedMetadataPiece(index) {
			pending = append(pending, index)
		}
	}
	for _, i := range mathRand.Perm(len(pending)) {
		c.requestMetadataPiece(pending[i])
	}
}

func (cl *Client) completedMetadata(t *torrent) {
	h := sha1.New()
	h.Write(t.MetaData)
	var ih InfoHash
	CopyExact(&ih, h.Sum(nil))
	if ih != t.InfoHash {
		log.Print("bad metadata")
		t.invalidateMetadata()
		return
	}
	var info metainfo.Info
	err := bencode.Unmarshal(t.MetaData, &info)
	if err != nil {
		log.Printf("error unmarshalling metadata: %s", err)
		t.invalidateMetadata()
		return
	}
	// TODO(anacrolix): If this fails, I think something harsher should be
	// done.
	err = cl.setMetaData(t, &info, t.MetaData)
	if err != nil {
		log.Printf("error setting metadata: %s", err)
		t.invalidateMetadata()
		return
	}
	if cl.config.Debug {
		log.Printf("%s: got metadata from peers", t)
	}
}

// Process incoming ut_metadata message.
func (cl *Client) gotMetadataExtensionMsg(payload []byte, t *torrent, c *connection) (err error) {
	var d map[string]int
	err = bencode.Unmarshal(payload, &d)
	if err != nil {
		err = fmt.Errorf("error unmarshalling payload: %s: %q", err, payload)
		return
	}
	msgType, ok := d["msg_type"]
	if !ok {
		err = errors.New("missing msg_type field")
		return
	}
	piece := d["piece"]
	switch msgType {
	case pp.DataMetadataExtensionMsgType:
		if t.haveInfo() {
			break
		}
		begin := len(payload) - metadataPieceSize(d["total_size"], piece)
		if begin < 0 || begin >= len(payload) {
			log.Printf("got bad metadata piece")
			break
		}
		if !c.requestedMetadataPiece(piece) {
			log.Printf("got unexpected metadata piece %d", piece)
			break
		}
		c.metadataRequests[piece] = false
		t.saveMetadataPiece(piece, payload[begin:])
		c.UsefulChunksReceived++
		c.lastUsefulChunkReceived = time.Now()
		if !t.haveAllMetadataPieces() {
			break
		}
		cl.completedMetadata(t)
	case pp.RequestMetadataExtensionMsgType:
		if !t.haveMetadataPiece(piece) {
			c.Post(t.newMetadataExtensionMessage(c, pp.RejectMetadataExtensionMsgType, d["piece"], nil))
			break
		}
		start := (1 << 14) * piece
		c.Post(t.newMetadataExtensionMessage(c, pp.DataMetadataExtensionMsgType, piece, t.MetaData[start:start+t.metadataPieceSize(piece)]))
	case pp.RejectMetadataExtensionMsgType:
	default:
		err = errors.New("unknown msg_type value")
	}
	return
}

// Extracts the port as an integer from an address string.
func addrPort(addr net.Addr) int {
	return AddrPort(addr)
}

func (cl *Client) peerHasAll(t *torrent, cn *connection) {
	cn.peerHasAll = true
	cn.PeerPieces = nil
	if t.haveInfo() {
		for i := 0; i < t.numPieces(); i++ {
			cl.peerGotPiece(t, cn, i)
		}
	}
}

func (me *Client) upload(t *torrent, c *connection) {
	if me.config.NoUpload {
		return
	}
	if !c.PeerInterested {
		return
	}
	seeding := me.seeding(t)
	if !seeding && !t.connHasWantedPieces(c) {
		return
	}
another:
	for seeding || c.chunksSent < c.UsefulChunksReceived+6 {
		c.Unchoke()
		for r := range c.PeerRequests {
			err := me.sendChunk(t, c, r)
			if err != nil {
				log.Printf("error sending chunk %+v to peer: %s", r, err)
			}
			delete(c.PeerRequests, r)
			goto another
		}
		return
	}
	c.Choke()
}

func (me *Client) sendChunk(t *torrent, c *connection, r request) error {
	// Count the chunk being sent, even if it isn't.
	c.chunksSent++
	b := make([]byte, r.Length)
	tp := &t.Pieces[r.Index]
	tp.pendingWritesMutex.Lock()
	for tp.pendingWrites != 0 {
		tp.noPendingWrites.Wait()
	}
	tp.pendingWritesMutex.Unlock()
	p := t.Info.Piece(int(r.Index))
	n, err := dataReadAt(t.data, b, p.Offset()+int64(r.Begin))
	if err != nil {
		return err
	}
	if n != len(b) {
		log.Fatal(b)
	}
	c.Post(pp.Message{
		Type:  pp.Piece,
		Index: r.Index,
		Begin: r.Begin,
		Piece: b,
	})
	uploadChunksPosted.Add(1)
	c.lastChunkSent = time.Now()
	return nil
}

// Processes incoming bittorrent messages. The client lock is held upon entry
// and exit.
func (me *Client) connectionLoop(t *torrent, c *connection) error {
	decoder := pp.Decoder{
		R:         bufio.NewReader(c.rw),
		MaxLength: 256 * 1024,
	}
	for {
		me.mu.Unlock()
		var msg pp.Message
		err := decoder.Decode(&msg)
		receivedMessageTypes.Add(strconv.FormatInt(int64(msg.Type), 10), 1)
		me.mu.Lock()
		c.lastMessageReceived = time.Now()
		select {
		case <-c.closing:
			return nil
		default:
		}
		if err != nil {
			if me.stopped() || err == io.EOF {
				return nil
			}
			return err
		}
		if msg.Keepalive {
			continue
		}
		switch msg.Type {
		case pp.Choke:
			c.PeerChoked = true
			for r := range c.Requests {
				me.connDeleteRequest(t, c, r)
			}
			// We can then reset our interest.
			me.replenishConnRequests(t, c)
		case pp.Reject:
			me.connDeleteRequest(t, c, newRequest(msg.Index, msg.Begin, msg.Length))
			me.replenishConnRequests(t, c)
		case pp.Unchoke:
			c.PeerChoked = false
			me.peerUnchoked(t, c)
		case pp.Interested:
			c.PeerInterested = true
			me.upload(t, c)
		case pp.NotInterested:
			c.PeerInterested = false
			c.Choke()
		case pp.Have:
			me.peerGotPiece(t, c, int(msg.Index))
		case pp.Request:
			if c.Choked {
				break
			}
			if !c.PeerInterested {
				err = errors.New("peer sent request but isn't interested")
				break
			}
			if c.PeerRequests == nil {
				c.PeerRequests = make(map[request]struct{}, maxRequests)
			}
			c.PeerRequests[newRequest(msg.Index, msg.Begin, msg.Length)] = struct{}{}
			me.upload(t, c)
		case pp.Cancel:
			req := newRequest(msg.Index, msg.Begin, msg.Length)
			if !c.PeerCancel(req) {
				unexpectedCancels.Add(1)
			}
		case pp.Bitfield:
			if c.PeerPieces != nil || c.peerHasAll {
				err = errors.New("received unexpected bitfield")
				break
			}
			if t.haveInfo() {
				if len(msg.Bitfield) < t.numPieces() {
					err = errors.New("received invalid bitfield")
					break
				}
				msg.Bitfield = msg.Bitfield[:t.numPieces()]
			}
			c.PeerPieces = msg.Bitfield
			for index, has := range c.PeerPieces {
				if has {
					me.peerGotPiece(t, c, index)
				}
			}
		case pp.HaveAll:
			if c.PeerPieces != nil || c.peerHasAll {
				err = errors.New("unexpected have-all")
				break
			}
			me.peerHasAll(t, c)
		case pp.HaveNone:
			if c.peerHasAll || c.PeerPieces != nil {
				err = errors.New("unexpected have-none")
				break
			}
			c.PeerPieces = make([]bool, func() int {
				if t.haveInfo() {
					return t.numPieces()
				} else {
					return 0
				}
			}())
		case pp.Piece:
			err = me.downloadedChunk(t, c, &msg)
		case pp.Extended:
			switch msg.ExtendedID {
			case pp.HandshakeExtendedID:
				// TODO: Create a bencode struct for this.
				var d map[string]interface{}
				err = bencode.Unmarshal(msg.ExtendedPayload, &d)
				if err != nil {
					err = fmt.Errorf("error decoding extended message payload: %s", err)
					break
				}
				// log.Printf("got handshake from %q: %#v", c.Socket.RemoteAddr().String(), d)
				if reqq, ok := d["reqq"]; ok {
					if i, ok := reqq.(int64); ok {
						c.PeerMaxRequests = int(i)
					}
				}
				if v, ok := d["v"]; ok {
					c.PeerClientName = v.(string)
				}
				m, ok := d["m"]
				if !ok {
					err = errors.New("handshake missing m item")
					break
				}
				mTyped, ok := m.(map[string]interface{})
				if !ok {
					err = errors.New("handshake m value is not dict")
					break
				}
				if c.PeerExtensionIDs == nil {
					c.PeerExtensionIDs = make(map[string]byte, len(mTyped))
				}
				for name, v := range mTyped {
					id, ok := v.(int64)
					if !ok {
						log.Printf("bad handshake m item extension ID type: %T", v)
						continue
					}
					if id == 0 {
						delete(c.PeerExtensionIDs, name)
					} else {
						if c.PeerExtensionIDs[name] == 0 {
							supportedExtensionMessages.Add(name, 1)
						}
						c.PeerExtensionIDs[name] = byte(id)
					}
				}
				metadata_sizeUntyped, ok := d["metadata_size"]
				if ok {
					metadata_size, ok := metadata_sizeUntyped.(int64)
					if !ok {
						log.Printf("bad metadata_size type: %T", metadata_sizeUntyped)
					} else {
						t.setMetadataSize(metadata_size, me)
					}
				}
				if _, ok := c.PeerExtensionIDs["ut_metadata"]; ok {
					me.requestPendingMetadata(t, c)
				}
			case metadataExtendedId:
				err = me.gotMetadataExtensionMsg(msg.ExtendedPayload, t, c)
				if err != nil {
					err = fmt.Errorf("error handling metadata extension message: %s", err)
				}
			case pexExtendedId:
				if me.config.DisablePEX {
					break
				}
				var pexMsg peerExchangeMessage
				err := bencode.Unmarshal(msg.ExtendedPayload, &pexMsg)
				if err != nil {
					err = fmt.Errorf("error unmarshalling PEX message: %s", err)
					break
				}
				go func() {
					me.mu.Lock()
					me.addPeers(t, func() (ret []Peer) {
						for i, cp := range pexMsg.Added {
							p := Peer{
								IP:     make([]byte, 4),
								Port:   int(cp.Port),
								Source: peerSourcePEX,
							}
							if i < len(pexMsg.AddedFlags) && pexMsg.AddedFlags[i]&0x01 != 0 {
								p.SupportsEncryption = true
							}
							CopyExact(p.IP, cp.IP[:])
							ret = append(ret, p)
						}
						return
					}())
					me.mu.Unlock()
				}()
			default:
				err = fmt.Errorf("unexpected extended message ID: %v", msg.ExtendedID)
			}
			if err != nil {
				// That client uses its own extension IDs for outgoing message
				// types, which is incorrect.
				if bytes.HasPrefix(c.PeerID[:], []byte("-SD0100-")) ||
					strings.HasPrefix(string(c.PeerID[:]), "-XL0012-") {
					return nil
				}
			}
		case pp.Port:
			if me.dHT == nil {
				break
			}
			pingAddr, err := net.ResolveUDPAddr("", c.remoteAddr().String())
			if err != nil {
				panic(err)
			}
			if msg.Port != 0 {
				pingAddr.Port = int(msg.Port)
			}
			_, err = me.dHT.Ping(pingAddr)
		default:
			err = fmt.Errorf("received unknown message type: %#v", msg.Type)
		}
		if err != nil {
			return err
		}
	}
}

// Returns true if connection is removed from torrent.Conns.
func (me *Client) deleteConnection(t *torrent, c *connection) bool {
	for i0, _c := range t.Conns {
		if _c != c {
			continue
		}
		i1 := len(t.Conns) - 1
		if i0 != i1 {
			t.Conns[i0] = t.Conns[i1]
		}
		t.Conns = t.Conns[:i1]
		return true
	}
	return false
}

func (me *Client) dropConnection(t *torrent, c *connection) {
	me.event.Broadcast()
	c.Close()
	if c.piecePriorities != nil {
		t.connPiecePriorites.Put(c.piecePriorities)
		// I wonder if it's safe to set it to nil. Probably not. Since it's
		// only read, it doesn't particularly matter if a closing connection
		// shares the slice with another connection.
	}
	if me.deleteConnection(t, c) {
		me.openNewConns(t)
	}
}

// Returns true if the connection is added.
func (me *Client) addConnection(t *torrent, c *connection) bool {
	if me.stopped() {
		return false
	}
	select {
	case <-t.ceasingNetworking:
		return false
	default:
	}
	if !me.wantConns(t) {
		return false
	}
	for _, c0 := range t.Conns {
		if c.PeerID == c0.PeerID {
			// Already connected to a client with that ID.
			duplicateClientConns.Add(1)
			return false
		}
	}
	if len(t.Conns) >= socketsPerTorrent {
		c := t.worstBadConn(me)
		if c == nil {
			return false
		}
		if me.config.Debug && missinggo.CryHeard() {
			log.Printf("%s: dropping connection to make room for new one:\n    %s", t, c)
		}
		c.Close()
		me.deleteConnection(t, c)
	}
	if len(t.Conns) >= socketsPerTorrent {
		panic(len(t.Conns))
	}
	t.Conns = append(t.Conns, c)
	return true
}

func (t *torrent) needData() bool {
	if !t.haveInfo() {
		return true
	}
	if len(t.urgent) != 0 {
		return true
	}
	for i := range t.Pieces {
		p := &t.Pieces[i]
		if p.Priority != PiecePriorityNone {
			return true
		}
	}
	return false
}

func (cl *Client) usefulConn(t *torrent, c *connection) bool {
	select {
	case <-c.closing:
		return false
	default:
	}
	if !t.haveInfo() {
		return c.supportsExtension("ut_metadata")
	}
	if cl.seeding(t) {
		return c.PeerInterested
	}
	return t.connHasWantedPieces(c)
}

func (me *Client) wantConns(t *torrent) bool {
	if !me.seeding(t) && !t.needData() {
		return false
	}
	if len(t.Conns) < socketsPerTorrent {
		return true
	}
	return t.worstBadConn(me) != nil
}

func (me *Client) openNewConns(t *torrent) {
	select {
	case <-t.ceasingNetworking:
		return
	default:
	}
	for len(t.Peers) != 0 {
		if !me.wantConns(t) {
			return
		}
		if len(t.HalfOpen) >= me.halfOpenLimit {
			return
		}
		var (
			k peersKey
			p Peer
		)
		for k, p = range t.Peers {
			break
		}
		delete(t.Peers, k)
		me.initiateConn(p, t)
	}
	t.wantPeers.Broadcast()
}

func (me *Client) addPeers(t *torrent, peers []Peer) {
	for _, p := range peers {
		if me.dopplegangerAddr(net.JoinHostPort(
			p.IP.String(),
			strconv.FormatInt(int64(p.Port), 10),
		)) {
			continue
		}
		if _, ok := me.ipBlockRange(p.IP); ok {
			continue
		}
		if p.Port == 0 {
			// The spec says to scrub these yourselves. Fine.
			continue
		}
		t.addPeer(p, me)
	}
}

func (cl *Client) cachedMetaInfoFilename(ih InfoHash) string {
	return filepath.Join(cl.configDir(), "torrents", ih.HexString()+".torrent")
}

func (cl *Client) saveTorrentFile(t *torrent) error {
	path := cl.cachedMetaInfoFilename(t.InfoHash)
	os.MkdirAll(filepath.Dir(path), 0777)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("error opening file: %s", err)
	}
	defer f.Close()
	e := bencode.NewEncoder(f)
	err = e.Encode(t.MetaInfo())
	if err != nil {
		return fmt.Errorf("error marshalling metainfo: %s", err)
	}
	mi, err := cl.torrentCacheMetaInfo(t.InfoHash)
	if err != nil {
		// For example, a script kiddy makes us load too many files, and we're
		// able to save the torrent, but not load it again to check it.
		return nil
	}
	if !bytes.Equal(mi.Info.Hash, t.InfoHash[:]) {
		log.Fatalf("%x != %x", mi.Info.Hash, t.InfoHash[:])
	}
	return nil
}

func (cl *Client) startTorrent(t *torrent) {
	if t.Info == nil || t.data == nil {
		panic("nope")
	}
	// If the client intends to upload, it needs to know what state pieces are
	// in.
	if !cl.config.NoUpload {
		// Queue all pieces for hashing. This is done sequentially to avoid
		// spamming goroutines.
		for i := range t.Pieces {
			t.Pieces[i].QueuedForHash = true
		}
		go func() {
			for i := range t.Pieces {
				cl.verifyPiece(t, i)
			}
		}()
	}
}

// Storage cannot be changed once it's set.
func (cl *Client) setStorage(t *torrent, td Data) (err error) {
	err = t.setStorage(td)
	cl.event.Broadcast()
	if err != nil {
		return
	}
	cl.startTorrent(t)
	return
}

type TorrentDataOpener func(*metainfo.Info) Data

func (cl *Client) setMetaData(t *torrent, md *metainfo.Info, bytes []byte) (err error) {
	err = t.setMetadata(md, bytes)
	if err != nil {
		return
	}
	if !cl.config.DisableMetainfoCache {
		if err := cl.saveTorrentFile(t); err != nil {
			log.Printf("error saving torrent file for %s: %s", t, err)
		}
	}
	cl.event.Broadcast()
	close(t.gotMetainfo)
	td := cl.torrentDataOpener(md)
	err = cl.setStorage(t, td)
	return
}

// Prepare a Torrent without any attachment to a Client. That means we can
// initialize fields all fields that don't require the Client without locking
// it.
func newTorrent(ih InfoHash) (t *torrent, err error) {
	t = &torrent{
		InfoHash:  ih,
		chunkSize: defaultChunkSize,
		Peers:     make(map[peersKey]Peer),

		closing:           make(chan struct{}),
		ceasingNetworking: make(chan struct{}),

		gotMetainfo: make(chan struct{}),

		HalfOpen:          make(map[string]struct{}),
		pieceStateChanges: pubsub.NewPubSub(),
	}
	t.wantPeers.L = &t.stateMu
	return
}

func init() {
	// For shuffling the tracker tiers.
	mathRand.Seed(time.Now().Unix())
}

// The trackers within each tier must be shuffled before use.
// http://stackoverflow.com/a/12267471/149482
// http://www.bittorrent.org/beps/bep_0012.html#order-of-processing
func shuffleTier(tier []tracker.Client) {
	for i := range tier {
		j := mathRand.Intn(i + 1)
		tier[i], tier[j] = tier[j], tier[i]
	}
}

func copyTrackers(base [][]tracker.Client) (copy [][]tracker.Client) {
	for _, tier := range base {
		copy = append(copy, append([]tracker.Client{}, tier...))
	}
	return
}

func mergeTier(tier []tracker.Client, newURLs []string) []tracker.Client {
nextURL:
	for _, url := range newURLs {
		for _, tr := range tier {
			if tr.URL() == url {
				continue nextURL
			}
		}
		tr, err := tracker.New(url)
		if err != nil {
			// log.Printf("error creating tracker client for %q: %s", url, err)
			continue
		}
		tier = append(tier, tr)
	}
	return tier
}

func (t *torrent) addTrackers(announceList [][]string) {
	newTrackers := copyTrackers(t.Trackers)
	for tierIndex, tier := range announceList {
		if tierIndex < len(newTrackers) {
			newTrackers[tierIndex] = mergeTier(newTrackers[tierIndex], tier)
		} else {
			newTrackers = append(newTrackers, mergeTier(nil, tier))
		}
		shuffleTier(newTrackers[tierIndex])
	}
	t.Trackers = newTrackers
}

// Don't call this before the info is available.
func (t *torrent) bytesCompleted() int64 {
	if !t.haveInfo() {
		return 0
	}
	return t.Info.TotalLength() - t.bytesLeft()
}

// A file-like handle to some torrent data resource.
type Handle interface {
	io.Reader
	io.Seeker
	io.Closer
	io.ReaderAt
}

// Returns handles to the files in the torrent. This requires the metainfo is
// available first.
func (t Torrent) Files() (ret []File) {
	t.cl.mu.Lock()
	info := t.Info()
	t.cl.mu.Unlock()
	if info == nil {
		return
	}
	var offset int64
	for _, fi := range info.UpvertedFiles() {
		ret = append(ret, File{
			t,
			strings.Join(append([]string{info.Name}, fi.Path...), "/"),
			offset,
			fi.Length,
			fi,
		})
		offset += fi.Length
	}
	return
}

// Marks the pieces in the given region for download.
func (t Torrent) SetRegionPriority(off, len int64) {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	pieceSize := int64(t.torrent.usualPieceSize())
	for i := off / pieceSize; i*pieceSize < off+len; i++ {
		t.cl.raisePiecePriority(t.torrent, int(i), PiecePriorityNormal)
	}
}

func (t Torrent) AddPeers(pp []Peer) error {
	cl := t.cl
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.addPeers(t.torrent, pp)
	return nil
}

// Marks the entire torrent for download. Requires the info first, see
// GotInfo.
func (t Torrent) DownloadAll() {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	for i := range iter.N(t.torrent.numPieces()) {
		t.cl.raisePiecePriority(t.torrent, i, PiecePriorityNormal)
	}
	// Nice to have the first and last pieces sooner for various interactive
	// purposes.
	t.cl.raisePiecePriority(t.torrent, 0, PiecePriorityReadahead)
	t.cl.raisePiecePriority(t.torrent, t.torrent.numPieces()-1, PiecePriorityReadahead)
}

// Returns nil metainfo if it isn't in the cache. Checks that the retrieved
// metainfo has the correct infohash.
func (cl *Client) torrentCacheMetaInfo(ih InfoHash) (mi *metainfo.MetaInfo, err error) {
	if cl.config.DisableMetainfoCache {
		return
	}
	f, err := os.Open(cl.cachedMetaInfoFilename(ih))
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return
	}
	defer f.Close()
	dec := bencode.NewDecoder(f)
	err = dec.Decode(&mi)
	if err != nil {
		return
	}
	if !bytes.Equal(mi.Info.Hash, ih[:]) {
		err = fmt.Errorf("cached torrent has wrong infohash: %x != %x", mi.Info.Hash, ih[:])
		return
	}
	return
}

// Specifies a new torrent for adding to a client. There are helpers for
// magnet URIs and torrent metainfo files.
type TorrentSpec struct {
	// The tiered tracker URIs.
	Trackers [][]string
	InfoHash InfoHash
	Info     *metainfo.InfoEx
	// The name to use if the Name field from the Info isn't available.
	DisplayName string
	// The chunk size to use for outbound requests. Defaults to 16KiB if not
	// set.
	ChunkSize int
}

func TorrentSpecFromMagnetURI(uri string) (spec *TorrentSpec, err error) {
	m, err := ParseMagnetURI(uri)
	if err != nil {
		return
	}
	spec = &TorrentSpec{
		Trackers:    [][]string{m.Trackers},
		DisplayName: m.DisplayName,
		InfoHash:    m.InfoHash,
	}
	return
}

func TorrentSpecFromMetaInfo(mi *metainfo.MetaInfo) (spec *TorrentSpec) {
	spec = &TorrentSpec{
		Trackers:    mi.AnnounceList,
		Info:        &mi.Info,
		DisplayName: mi.Info.Name,
	}

	if len(spec.Trackers) == 0 {
		spec.Trackers = [][]string{[]string{mi.Announce}}
	} else {
		spec.Trackers[0] = append(spec.Trackers[0], mi.Announce)
	}

	CopyExact(&spec.InfoHash, &mi.Info.Hash)
	return
}

// Add or merge a torrent spec. If the torrent is already present, the
// trackers will be merged with the existing ones. If the Info isn't yet
// known, it will be set. The display name is replaced if the new spec
// provides one. Returns new if the torrent wasn't already in the client.
func (cl *Client) AddTorrentSpec(spec *TorrentSpec) (T Torrent, new bool, err error) {
	T.cl = cl
	cl.mu.Lock()
	defer cl.mu.Unlock()

	t, ok := cl.torrents[spec.InfoHash]
	if !ok {
		new = true

		if _, ok := cl.bannedTorrents[spec.InfoHash]; ok {
			err = errors.New("banned torrent")
			return
		}

		t, err = newTorrent(spec.InfoHash)
		if err != nil {
			return
		}
		if spec.ChunkSize != 0 {
			t.chunkSize = pp.Integer(spec.ChunkSize)
		}
	}
	if spec.DisplayName != "" {
		t.setDisplayName(spec.DisplayName)
	}
	// Try to merge in info we have on the torrent. Any err left will
	// terminate the function.
	if t.Info == nil {
		if spec.Info != nil {
			err = cl.setMetaData(t, &spec.Info.Info, spec.Info.Bytes)
		} else {
			var mi *metainfo.MetaInfo
			mi, err = cl.torrentCacheMetaInfo(spec.InfoHash)
			if err != nil {
				log.Printf("error getting cached metainfo: %s", err)
				err = nil
			} else if mi != nil {
				t.addTrackers(mi.AnnounceList)
				err = cl.setMetaData(t, &mi.Info.Info, mi.Info.Bytes)
			}
		}
	}
	if err != nil {
		return
	}
	t.addTrackers(spec.Trackers)

	cl.torrents[spec.InfoHash] = t
	T.torrent = t

	// From this point onwards, we can consider the torrent a part of the
	// client.
	if new {
		if !cl.config.DisableTrackers {
			go cl.announceTorrentTrackers(T.torrent)
		}
		if cl.dHT != nil {
			go cl.announceTorrentDHT(T.torrent, true)
		}
	}
	return
}

func (me *Client) dropTorrent(infoHash InfoHash) (err error) {
	t, ok := me.torrents[infoHash]
	if !ok {
		err = fmt.Errorf("no such torrent")
		return
	}
	err = t.close()
	if err != nil {
		panic(err)
	}
	delete(me.torrents, infoHash)
	return
}

// Returns true when peers are required, or false if the torrent is closing.
func (cl *Client) waitWantPeers(t *torrent) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	for {
		select {
		case <-t.ceasingNetworking:
			return false
		default:
		}
		if len(t.Peers) > torrentPeersLowWater {
			goto wait
		}
		if t.needData() || cl.seeding(t) {
			return true
		}
	wait:
		cl.mu.Unlock()
		t.wantPeers.Wait()
		t.stateMu.Unlock()
		cl.mu.Lock()
		t.stateMu.Lock()
	}
}

// Returns whether the client should make effort to seed the torrent.
func (cl *Client) seeding(t *torrent) bool {
	if cl.config.NoUpload {
		return false
	}
	if !cl.config.Seed {
		return false
	}
	if t.needData() {
		return false
	}
	return true
}

func (cl *Client) announceTorrentDHT(t *torrent, impliedPort bool) {
	for cl.waitWantPeers(t) {
		// log.Printf("getting peers for %q from DHT", t)
		ps, err := cl.dHT.Announce(string(t.InfoHash[:]), cl.incomingPeerPort(), impliedPort)
		if err != nil {
			log.Printf("error getting peers from dht: %s", err)
			return
		}
		// Count all the unique addresses we got during this announce.
		allAddrs := make(map[string]struct{})
	getPeers:
		for {
			select {
			case v, ok := <-ps.Peers:
				if !ok {
					break getPeers
				}
				addPeers := make([]Peer, 0, len(v.Peers))
				for _, cp := range v.Peers {
					if cp.Port == 0 {
						// Can't do anything with this.
						continue
					}
					addPeers = append(addPeers, Peer{
						IP:     cp.IP[:],
						Port:   int(cp.Port),
						Source: peerSourceDHT,
					})
					key := (&net.UDPAddr{
						IP:   cp.IP[:],
						Port: int(cp.Port),
					}).String()
					allAddrs[key] = struct{}{}
				}
				cl.mu.Lock()
				cl.addPeers(t, addPeers)
				numPeers := len(t.Peers)
				cl.mu.Unlock()
				if numPeers >= torrentPeersHighWater {
					break getPeers
				}
			case <-t.ceasingNetworking:
				ps.Close()
				return
			}
		}
		ps.Close()
		// log.Printf("finished DHT peer scrape for %s: %d peers", t, len(allAddrs))
	}
}

func (cl *Client) trackerBlockedUnlocked(tr tracker.Client) (blocked bool, err error) {
	url_, err := url.Parse(tr.URL())
	if err != nil {
		return
	}
	host, _, err := net.SplitHostPort(url_.Host)
	if err != nil {
		host = url_.Host
	}
	addr, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return
	}
	cl.mu.RLock()
	_, blocked = cl.ipBlockRange(addr.IP)
	cl.mu.RUnlock()
	return
}

func (cl *Client) announceTorrentSingleTracker(tr tracker.Client, req *tracker.AnnounceRequest, t *torrent) error {
	blocked, err := cl.trackerBlockedUnlocked(tr)
	if err != nil {
		return fmt.Errorf("error determining if tracker blocked: %s", err)
	}
	if blocked {
		return fmt.Errorf("tracker blocked: %s", tr)
	}
	if err := tr.Connect(); err != nil {
		return fmt.Errorf("error connecting: %s", err)
	}
	resp, err := tr.Announce(req)
	if err != nil {
		return fmt.Errorf("error announcing: %s", err)
	}
	var peers []Peer
	for _, peer := range resp.Peers {
		peers = append(peers, Peer{
			IP:   peer.IP,
			Port: peer.Port,
		})
	}
	cl.mu.Lock()
	cl.addPeers(t, peers)
	cl.mu.Unlock()

	// log.Printf("%s: %d new peers from %s", t, len(peers), tr)

	time.Sleep(time.Second * time.Duration(resp.Interval))
	return nil
}

func (cl *Client) announceTorrentTrackersFastStart(req *tracker.AnnounceRequest, trackers [][]tracker.Client, t *torrent) (atLeastOne bool) {
	oks := make(chan bool)
	outstanding := 0
	for _, tier := range trackers {
		for _, tr := range tier {
			outstanding++
			go func(tr tracker.Client) {
				err := cl.announceTorrentSingleTracker(tr, req, t)
				oks <- err == nil
			}(tr)
		}
	}
	for outstanding > 0 {
		ok := <-oks
		outstanding--
		if ok {
			atLeastOne = true
		}
	}
	return
}

// Announce torrent to its trackers.
func (cl *Client) announceTorrentTrackers(t *torrent) {
	req := tracker.AnnounceRequest{
		Event:    tracker.Started,
		NumWant:  -1,
		Port:     uint16(cl.incomingPeerPort()),
		PeerId:   cl.peerID,
		InfoHash: t.InfoHash,
	}
	if !cl.waitWantPeers(t) {
		return
	}
	cl.mu.RLock()
	req.Left = uint64(t.bytesLeft())
	trackers := t.Trackers
	cl.mu.RUnlock()
	if cl.announceTorrentTrackersFastStart(&req, trackers, t) {
		req.Event = tracker.None
	}
newAnnounce:
	for cl.waitWantPeers(t) {
		cl.mu.RLock()
		req.Left = uint64(t.bytesLeft())
		trackers = t.Trackers
		cl.mu.RUnlock()
		numTrackersTried := 0
		for _, tier := range trackers {
			for trIndex, tr := range tier {
				numTrackersTried++
				err := cl.announceTorrentSingleTracker(tr, &req, t)
				if err != nil {
					continue
				}
				// Float the successful announce to the top of the tier. If
				// the trackers list has been changed, we'll be modifying an
				// old copy so it won't matter.
				cl.mu.Lock()
				tier[0], tier[trIndex] = tier[trIndex], tier[0]
				cl.mu.Unlock()

				req.Event = tracker.None
				continue newAnnounce
			}
		}
		if numTrackersTried != 0 {
			log.Printf("%s: all trackers failed", t)
		}
		// TODO: Wait until trackers are added if there are none.
		time.Sleep(10 * time.Second)
	}
}

func (cl *Client) allTorrentsCompleted() bool {
	for _, t := range cl.torrents {
		if !t.haveInfo() {
			return false
		}
		if t.numPiecesCompleted() != t.numPieces() {
			return false
		}
	}
	return true
}

// Returns true when all torrents are completely downloaded and false if the
// client is stopped before that.
func (me *Client) WaitAll() bool {
	me.mu.Lock()
	defer me.mu.Unlock()
	for !me.allTorrentsCompleted() {
		if me.stopped() {
			return false
		}
		me.event.Wait()
	}
	return true
}

func (me *Client) fillRequests(t *torrent, c *connection) {
	if c.Interested {
		if c.PeerChoked {
			return
		}
		if len(c.Requests) > c.requestsLowWater {
			return
		}
	}
	addRequest := func(req request) (again bool) {
		// TODO: Couldn't this check also be done *after* the request?
		if len(c.Requests) >= 64 {
			return false
		}
		return c.Request(req)
	}
	for req := range t.urgent {
		if !addRequest(req) {
			return
		}
	}
	for e := c.pieceRequestOrder.First(); e != nil; e = e.Next() {
		pieceIndex := e.Piece()
		if !c.PeerHasPiece(pieceIndex) {
			panic("piece in request order but peer doesn't have it")
		}
		if !t.wantPiece(pieceIndex) {
			log.Printf("unwanted piece %d in connection request order\n%s", pieceIndex, c)
			c.pieceRequestOrder.DeletePiece(pieceIndex)
			continue
		}
		piece := &t.Pieces[pieceIndex]
		for _, cs := range piece.shuffledPendingChunkSpecs(t, pieceIndex) {
			r := request{pp.Integer(pieceIndex), cs}
			if !addRequest(r) {
				return
			}
		}
	}
	return
}

func (me *Client) replenishConnRequests(t *torrent, c *connection) {
	if !t.haveInfo() {
		return
	}
	me.fillRequests(t, c)
	if len(c.Requests) == 0 && !c.PeerChoked {
		// So we're not choked, but we don't want anything right now. We may
		// have completed readahead, and the readahead window has not rolled
		// over to the next piece. Better to stay interested in case we're
		// going to want data in the near future.
		c.SetInterested(!t.haveAllPieces())
	}
}

// Handle a received chunk from a peer.
func (me *Client) downloadedChunk(t *torrent, c *connection, msg *pp.Message) error {
	chunksReceived.Add(1)

	req := newRequest(msg.Index, msg.Begin, pp.Integer(len(msg.Piece)))

	// Request has been satisfied.
	if me.connDeleteRequest(t, c, req) {
		defer me.replenishConnRequests(t, c)
	} else {
		unexpectedChunksReceived.Add(1)
	}

	index := int(req.Index)
	piece := &t.Pieces[index]

	// Do we actually want this chunk?
	if !t.wantChunk(req) {
		unwantedChunksReceived.Add(1)
		c.UnwantedChunksReceived++
		return nil
	}

	c.UsefulChunksReceived++
	c.lastUsefulChunkReceived = time.Now()

	me.upload(t, c)

	piece.pendingWritesMutex.Lock()
	piece.pendingWrites++
	piece.pendingWritesMutex.Unlock()
	go func() {
		defer func() {
			piece.pendingWritesMutex.Lock()
			piece.pendingWrites--
			if piece.pendingWrites == 0 {
				piece.noPendingWrites.Broadcast()
			}
			piece.pendingWritesMutex.Unlock()
		}()
		// Write the chunk out.
		tr := perf.NewTimer()
		err := t.writeChunk(int(msg.Index), int64(msg.Begin), msg.Piece)
		if err != nil {
			log.Printf("error writing chunk: %s", err)
			return
		}
		tr.Stop("write chunk")
		me.mu.Lock()
		if c.peerTouchedPieces == nil {
			c.peerTouchedPieces = make(map[int]struct{})
		}
		c.peerTouchedPieces[index] = struct{}{}
		me.mu.Unlock()
	}()

	// log.Println("got chunk", req)
	me.event.Broadcast()
	defer t.publishPieceChange(int(req.Index))
	// Record that we have the chunk.
	piece.unpendChunkIndex(chunkIndex(req.chunkSpec, t.chunkSize))
	delete(t.urgent, req)
	// It's important that the piece is potentially queued before we check if
	// the piece is still wanted, because if it is queued, it won't be wanted.
	if t.pieceAllDirty(index) {
		me.queuePieceCheck(t, int(req.Index))
	}
	if !t.wantPiece(int(req.Index)) {
		for _, c := range t.Conns {
			c.pieceRequestOrder.DeletePiece(int(req.Index))
		}
	}

	// Cancel pending requests for this chunk.
	for _, c := range t.Conns {
		if me.connCancel(t, c, req) {
			me.replenishConnRequests(t, c)
		}
	}

	return nil
}

// Return the connections that touched a piece, and clear the entry while
// doing it.
func (me *Client) reapPieceTouches(t *torrent, piece int) (ret []*connection) {
	for _, c := range t.Conns {
		if _, ok := c.peerTouchedPieces[piece]; ok {
			ret = append(ret, c)
			delete(c.peerTouchedPieces, piece)
		}
	}
	return
}

func (me *Client) pieceHashed(t *torrent, piece int, correct bool) {
	p := &t.Pieces[piece]
	if p.EverHashed {
		// Don't score the first time a piece is hashed, it could be an
		// initial check.
		if correct {
			pieceHashedCorrect.Add(1)
		} else {
			log.Printf("%s: piece %d failed hash", t, piece)
			pieceHashedNotCorrect.Add(1)
		}
	}
	p.EverHashed = true
	touchers := me.reapPieceTouches(t, int(piece))
	if correct {
		err := t.data.PieceCompleted(int(piece))
		if err != nil {
			log.Printf("error completing piece: %s", err)
			correct = false
		}
	} else if len(touchers) != 0 {
		log.Printf("dropping %d conns that touched piece", len(touchers))
		for _, c := range touchers {
			me.dropConnection(t, c)
		}
	}
	me.pieceChanged(t, int(piece))
}

func (me *Client) pieceChanged(t *torrent, piece int) {
	correct := t.pieceComplete(piece)
	p := &t.Pieces[piece]
	defer t.publishPieceChange(piece)
	defer me.event.Broadcast()
	if correct {
		p.Priority = PiecePriorityNone
		for req := range t.urgent {
			if int(req.Index) == piece {
				delete(t.urgent, req)
			}
		}
	} else {
		if t.pieceAllDirty(piece) {
			t.pendAllChunkSpecs(piece)
		}
		if t.wantPiece(piece) {
			me.openNewConns(t)
		}
	}
	for _, conn := range t.Conns {
		if correct {
			conn.Have(piece)
			for r := range conn.Requests {
				if int(r.Index) == piece {
					conn.Cancel(r)
				}
			}
			conn.pieceRequestOrder.DeletePiece(int(piece))
			me.upload(t, conn)
		} else if t.wantPiece(piece) && conn.PeerHasPiece(piece) {
			t.connPendPiece(conn, int(piece))
			me.replenishConnRequests(t, conn)
		}
	}
	me.event.Broadcast()
}

func (cl *Client) verifyPiece(t *torrent, piece int) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	p := &t.Pieces[piece]
	for p.Hashing || t.data == nil {
		cl.event.Wait()
	}
	p.QueuedForHash = false
	if t.isClosed() || t.pieceComplete(piece) {
		return
	}
	p.Hashing = true
	cl.mu.Unlock()
	sum := t.hashPiece(piece)
	cl.mu.Lock()
	select {
	case <-t.closing:
		return
	default:
	}
	p.Hashing = false
	cl.pieceHashed(t, piece, sum == p.Hash)
}

// Returns handles to all the torrents loaded in the Client.
func (me *Client) Torrents() (ret []Torrent) {
	me.mu.Lock()
	for _, t := range me.torrents {
		ret = append(ret, Torrent{me, t})
	}
	me.mu.Unlock()
	return
}

func (me *Client) AddMagnet(uri string) (T Torrent, err error) {
	spec, err := TorrentSpecFromMagnetURI(uri)
	if err != nil {
		return
	}
	T, _, err = me.AddTorrentSpec(spec)
	return
}

func (me *Client) AddTorrent(mi *metainfo.MetaInfo) (T Torrent, err error) {
	T, _, err = me.AddTorrentSpec(TorrentSpecFromMetaInfo(mi))
	return
}

func (me *Client) AddTorrentFromFile(filename string) (T Torrent, err error) {
	mi, err := metainfo.LoadFromFile(filename)
	if err != nil {
		return
	}
	T, _, err = me.AddTorrentSpec(TorrentSpecFromMetaInfo(mi))
	return
}

func (me *Client) DHT() *dht.Server {
	return me.dHT
}
