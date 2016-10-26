package torrent

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/pproffd"
	"github.com/anacrolix/missinggo/pubsub"
	"github.com/anacrolix/missinggo/slices"
	"github.com/anacrolix/sync"
	"github.com/anacrolix/utp"
	"github.com/dustin/go-humanize"
	"golang.org/x/time/rate"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/dht"
	"github.com/anacrolix/torrent/dht/krpc"
	"github.com/anacrolix/torrent/iplist"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/mse"
	pp "github.com/anacrolix/torrent/peer_protocol"
	"github.com/anacrolix/torrent/storage"
)

// Currently doesn't really queue, but should in the future.
func (cl *Client) queuePieceCheck(t *Torrent, pieceIndex int) {
	piece := &t.pieces[pieceIndex]
	if piece.QueuedForHash {
		return
	}
	piece.QueuedForHash = true
	t.publishPieceChange(pieceIndex)
	go cl.verifyPiece(t, pieceIndex)
}

// Queue a piece check if one isn't already queued, and the piece has never
// been checked before.
func (cl *Client) queueFirstHash(t *Torrent, piece int) {
	p := &t.pieces[piece]
	if p.EverHashed || p.Hashing || p.QueuedForHash || t.pieceComplete(piece) {
		return
	}
	cl.queuePieceCheck(t, piece)
}

// Clients contain zero or more Torrents. A Client manages a blocklist, the
// TCP/UDP protocol ports, and DHT as desired.
type Client struct {
	mu     sync.RWMutex
	event  sync.Cond
	closed missinggo.Event

	config Config

	halfOpenLimit  int
	peerID         [20]byte
	defaultStorage *storage.Client
	tcpListener    net.Listener
	utpSock        *utp.Socket
	dHT            *dht.Server
	ipBlockList    iplist.Ranger
	// Our BitTorrent protocol extension bytes, sent in our BT handshakes.
	extensionBytes peerExtensionBytes
	// The net.Addr.String part that should be common to all active listeners.
	listenAddr    string
	uploadLimit   *rate.Limiter
	downloadLimit *rate.Limiter

	// Set of addresses that have our client ID. This intentionally will
	// include ourselves if we end up trying to connect to our own address
	// through legitimate channels.
	dopplegangerAddrs map[string]struct{}
	badPeerIPs        map[string]struct{}
	torrents          map[metainfo.Hash]*Torrent
}

func (cl *Client) BadPeerIPs() []string {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return slices.FromMapKeys(cl.badPeerIPs).([]string)
}

func (cl *Client) IPBlockList() iplist.Ranger {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.ipBlockList
}

func (cl *Client) SetIPBlockList(list iplist.Ranger) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.ipBlockList = list
	if cl.dHT != nil {
		cl.dHT.SetIPBlockList(list)
	}
}

func (cl *Client) PeerID() string {
	return string(cl.peerID[:])
}

type torrentAddr string

func (me torrentAddr) Network() string { return "" }

func (me torrentAddr) String() string { return string(me) }

func (cl *Client) ListenAddr() net.Addr {
	if cl.listenAddr == "" {
		return nil
	}
	return torrentAddr(cl.listenAddr)
}

func (cl *Client) sortedTorrents() (ret []*Torrent) {
	return slices.Sort(slices.FromMapElems(cl.torrents), func(l, r metainfo.Hash) bool {
		return l.AsString() < r.AsString()
	}).([]*Torrent)
}

// Writes out a human readable status of the client, such as for writing to a
// HTTP status page.
func (cl *Client) WriteStatus(_w io.Writer) {
	w := bufio.NewWriter(_w)
	defer w.Flush()
	if addr := cl.ListenAddr(); addr != nil {
		fmt.Fprintf(w, "Listening on %s\n", addr)
	} else {
		fmt.Fprintln(w, "Not listening!")
	}
	fmt.Fprintf(w, "Peer ID: %+q\n", cl.PeerID())
	fmt.Fprintf(w, "Banned IPs: %d\n", len(cl.BadPeerIPs()))
	if dht := cl.DHT(); dht != nil {
		dhtStats := dht.Stats()
		fmt.Fprintf(w, "DHT nodes: %d (%d good, %d banned)\n", dhtStats.Nodes, dhtStats.GoodNodes, dhtStats.BadNodes)
		fmt.Fprintf(w, "DHT Server ID: %x\n", dht.ID())
		fmt.Fprintf(w, "DHT port: %d\n", missinggo.AddrPort(dht.Addr()))
		fmt.Fprintf(w, "DHT announces: %d\n", dhtStats.ConfirmedAnnounces)
		fmt.Fprintf(w, "Outstanding transactions: %d\n", dhtStats.OutstandingTransactions)
	}
	fmt.Fprintf(w, "# Torrents: %d\n", len(cl.Torrents()))
	fmt.Fprintln(w)
	for _, t := range slices.Sort(append([]*Torrent(nil), cl.Torrents()...), func(l, r *Torrent) bool {
		return l.InfoHash().AsString() < r.InfoHash().AsString()
	}).([]*Torrent) {
		if t.Name() == "" {
			fmt.Fprint(w, "<unknown name>")
		} else {
			fmt.Fprint(w, t.Name())
		}
		fmt.Fprint(w, "\n")
		if t.Info() != nil {
			fmt.Fprintf(w, "%f%% of %d bytes (%s)", 100*(1-float64(t.BytesMissing())/float64(t.Info().TotalLength())), t.length, humanize.Bytes(uint64(t.Info().TotalLength())))
		} else {
			w.WriteString("<missing metainfo>")
		}
		fmt.Fprint(w, "\n")
		t.writeStatus(w)
		fmt.Fprintln(w)
	}
}

func listenUTP(networkSuffix, addr string) (*utp.Socket, error) {
	return utp.NewSocket("udp"+networkSuffix, addr)
}

func listenTCP(networkSuffix, addr string) (net.Listener, error) {
	return net.Listen("tcp"+networkSuffix, addr)
}

func listenBothSameDynamicPort(networkSuffix, host string) (tcpL net.Listener, utpSock *utp.Socket, listenedAddr string, err error) {
	for {
		tcpL, err = listenTCP(networkSuffix, net.JoinHostPort(host, "0"))
		if err != nil {
			return
		}
		listenedAddr = tcpL.Addr().String()
		utpSock, err = listenUTP(networkSuffix, listenedAddr)
		if err == nil {
			return
		}
		tcpL.Close()
		if !strings.Contains(err.Error(), "address already in use") {
			return
		}
	}
}

// Listen to enabled protocols, ensuring ports match.
func listen(tcp, utp bool, networkSuffix, addr string) (tcpL net.Listener, utpSock *utp.Socket, listenedAddr string, err error) {
	if addr == "" {
		addr = ":50007"
	}
	if tcp && utp {
		var host string
		var port int
		host, port, err = missinggo.ParseHostPort(addr)
		if err != nil {
			return
		}
		if port == 0 {
			// If both protocols are active, they need to have the same port.
			return listenBothSameDynamicPort(networkSuffix, host)
		}
	}
	defer func() {
		if err != nil {
			listenedAddr = ""
		}
	}()
	if tcp {
		tcpL, err = listenTCP(networkSuffix, addr)
		if err != nil {
			return
		}
		defer func() {
			if err != nil {
				tcpL.Close()
			}
		}()
		listenedAddr = tcpL.Addr().String()
	}
	if utp {
		utpSock, err = listenUTP(networkSuffix, addr)
		if err != nil {
			return
		}
		listenedAddr = utpSock.Addr().String()
	}
	return
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
		halfOpenLimit:     defaultHalfOpenConnsPerTorrent,
		config:            *cfg,
		dopplegangerAddrs: make(map[string]struct{}),
		torrents:          make(map[metainfo.Hash]*Torrent),
	}
	if cfg.UploadRateLimiter == nil {
		cl.uploadLimit = rate.NewLimiter(rate.Inf, 0)
	} else {
		cl.uploadLimit = cfg.UploadRateLimiter
	}
	if cfg.DownloadRateLimiter == nil {
		cl.downloadLimit = rate.NewLimiter(rate.Inf, 0)
	} else {
		cl.downloadLimit = cfg.DownloadRateLimiter
	}
	missinggo.CopyExact(&cl.extensionBytes, defaultExtensionBytes)
	cl.event.L = &cl.mu
	storageImpl := cfg.DefaultStorage
	if storageImpl == nil {
		storageImpl = storage.NewFile(cfg.DataDir)
	}
	cl.defaultStorage = storage.NewClient(storageImpl)
	if cfg.IPBlocklist != nil {
		cl.ipBlockList = cfg.IPBlocklist
	}

	if cfg.PeerID != "" {
		missinggo.CopyExact(&cl.peerID, cfg.PeerID)
	} else {
		o := copy(cl.peerID[:], bep20)
		_, err = rand.Read(cl.peerID[o:])
		if err != nil {
			panic("error generating peer id")
		}
	}

	cl.tcpListener, cl.utpSock, cl.listenAddr, err = listen(
		!cl.config.DisableTCP,
		!cl.config.DisableUTP,
		func() string {
			if cl.config.DisableIPv6 {
				return "4"
			} else {
				return ""
			}
		}(),
		cl.config.ListenAddr)
	if err != nil {
		return
	}
	if cl.tcpListener != nil {
		go cl.acceptConnections(cl.tcpListener, false)
	}
	if cl.utpSock != nil {
		go cl.acceptConnections(cl.utpSock, true)
	}
	if !cfg.NoDHT {
		dhtCfg := cfg.DHTConfig
		if dhtCfg.IPBlocklist == nil {
			dhtCfg.IPBlocklist = cl.ipBlockList
		}
		dhtCfg.Addr = firstNonEmptyString(dhtCfg.Addr, cl.listenAddr, cl.config.ListenAddr)
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

func firstNonEmptyString(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// Stops the client. All connections to peers are closed and all activity will
// come to a halt.
func (cl *Client) Close() {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.closed.Set()
	if cl.dHT != nil {
		cl.dHT.Close()
	}
	if cl.utpSock != nil {
		cl.utpSock.CloseNow()
	}
	if cl.tcpListener != nil {
		cl.tcpListener.Close()
	}
	for _, t := range cl.torrents {
		t.close()
	}
	cl.event.Broadcast()
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
	for {
		for _, t := range cl.torrents {
			if t.wantConns() {
				return
			}
		}
		if cl.closed.IsSet() {
			return
		}
		cl.event.Wait()
	}
}

func (cl *Client) acceptConnections(l net.Listener, utp bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	for {
		cl.waitAccept()
		cl.mu.Unlock()
		conn, err := l.Accept()
		conn = pproffd.WrapNetConn(conn)
		cl.mu.Lock()
		if cl.closed.IsSet() {
			if conn != nil {
				conn.Close()
			}
			return
		}
		if err != nil {
			log.Print(err)
			// I think something harsher should happen here? Our accept
			// routine just fucked off.
			return
		}
		if utp {
			acceptUTP.Add(1)
		} else {
			acceptTCP.Add(1)
		}
		if cl.config.Debug {
			log.Printf("accepted connection from %s", conn.RemoteAddr())
		}
		reject := cl.badPeerIPPort(
			missinggo.AddrIP(conn.RemoteAddr()),
			missinggo.AddrPort(conn.RemoteAddr()))
		if reject {
			if cl.config.Debug {
				log.Printf("rejecting connection from %s", conn.RemoteAddr())
			}
			acceptReject.Add(1)
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
	c := cl.newConnection(nc)
	c.Discovery = peerSourceIncoming
	c.uTP = utp
	cl.runReceivedConn(c)
}

// Returns a handle to the given torrent, if it's present in the client.
func (cl *Client) Torrent(ih metainfo.Hash) (t *Torrent, ok bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	t, ok = cl.torrents[ih]
	return
}

func (cl *Client) torrent(ih metainfo.Hash) *Torrent {
	return cl.torrents[ih]
}

type dialResult struct {
	Conn net.Conn
	UTP  bool
}

func doDial(dial func(addr string, t *Torrent) (net.Conn, error), ch chan dialResult, utp bool, addr string, t *Torrent) {
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
func (cl *Client) dopplegangerAddr(addr string) bool {
	_, ok := cl.dopplegangerAddrs[addr]
	return ok
}

// Start the process of connecting to the given peer for the given torrent if
// appropriate.
func (cl *Client) initiateConn(peer Peer, t *Torrent) {
	if peer.Id == cl.peerID {
		return
	}
	if cl.badPeerIPPort(peer.IP, peer.Port) {
		return
	}
	addr := net.JoinHostPort(peer.IP.String(), fmt.Sprintf("%d", peer.Port))
	if t.addrActive(addr) {
		return
	}
	t.halfOpen[addr] = struct{}{}
	go cl.outgoingConnection(t, addr, peer.Source)
}

func (cl *Client) dialTimeout(t *Torrent) time.Duration {
	cl.mu.Lock()
	pendingPeers := len(t.peers)
	cl.mu.Unlock()
	return reducedDialTimeout(nominalDialTimeout, cl.halfOpenLimit, pendingPeers)
}

func (cl *Client) dialTCP(addr string, t *Torrent) (c net.Conn, err error) {
	c, err = net.DialTimeout("tcp", addr, cl.dialTimeout(t))
	if err == nil {
		c.(*net.TCPConn).SetLinger(0)
	}
	c = pproffd.WrapNetConn(c)
	return
}

func (cl *Client) dialUTP(addr string, t *Torrent) (c net.Conn, err error) {
	return cl.utpSock.DialTimeout(addr, cl.dialTimeout(t))
}

// Returns a connection over UTP or TCP, whichever is first to connect.
func (cl *Client) dialFirst(addr string, t *Torrent) (conn net.Conn, utp bool) {
	// Initiate connections via TCP and UTP simultaneously. Use the first one
	// that succeeds.
	left := 0
	if !cl.config.DisableUTP {
		left++
	}
	if !cl.config.DisableTCP {
		left++
	}
	resCh := make(chan dialResult, left)
	if !cl.config.DisableUTP {
		go doDial(cl.dialUTP, resCh, true, addr, t)
	}
	if !cl.config.DisableTCP {
		go doDial(cl.dialTCP, resCh, false, addr, t)
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

func (cl *Client) noLongerHalfOpen(t *Torrent, addr string) {
	if _, ok := t.halfOpen[addr]; !ok {
		panic("invariant broken")
	}
	delete(t.halfOpen, addr)
	cl.openNewConns(t)
}

// Performs initiator handshakes and returns a connection. Returns nil
// *connection if no connection for valid reasons.
func (cl *Client) handshakesConnection(nc net.Conn, t *Torrent, encrypted, utp bool) (c *connection, err error) {
	c = cl.newConnection(nc)
	c.encrypted = encrypted
	c.uTP = utp
	err = nc.SetDeadline(time.Now().Add(handshakesTimeout))
	if err != nil {
		return
	}
	ok, err := cl.initiateHandshakes(c, t)
	if !ok {
		c = nil
	}
	return
}

// Returns nil connection and nil error if no connection could be established
// for valid reasons.
func (cl *Client) establishOutgoingConn(t *Torrent, addr string) (c *connection, err error) {
	nc, utp := cl.dialFirst(addr, t)
	if nc == nil {
		return
	}
	encryptFirst := !cl.config.DisableEncryption && !cl.config.PreferNoEncryption
	c, err = cl.handshakesConnection(nc, t, encryptFirst, utp)
	if err != nil {
		nc.Close()
		return
	} else if c != nil {
		return
	}
	nc.Close()
	if cl.config.DisableEncryption || cl.config.ForceEncryption {
		// There's no alternate encryption case to try.
		return
	}
	// Try again with encryption if we didn't earlier, or without if we did,
	// using whichever protocol type worked last time.
	if utp {
		nc, err = cl.dialUTP(addr, t)
	} else {
		nc, err = cl.dialTCP(addr, t)
	}
	if err != nil {
		err = fmt.Errorf("error dialing for unencrypted connection: %s", err)
		return
	}
	c, err = cl.handshakesConnection(nc, t, !encryptFirst, utp)
	if err != nil || c == nil {
		nc.Close()
	}
	return
}

// Called to dial out and run a connection. The addr we're given is already
// considered half-open.
func (cl *Client) outgoingConnection(t *Torrent, addr string, ps peerSource) {
	c, err := cl.establishOutgoingConn(t, addr)
	cl.mu.Lock()
	defer cl.mu.Unlock()
	// Don't release lock between here and addConnection, unless it's for
	// failure.
	cl.noLongerHalfOpen(t, addr)
	if err != nil {
		if cl.config.Debug {
			log.Printf("error establishing outgoing connection: %s", err)
		}
		return
	}
	if c == nil {
		return
	}
	defer c.Close()
	c.Discovery = ps
	cl.runInitiatedHandshookConn(c, t)
}

// The port number for incoming peer connections. 0 if the client isn't
// listening.
func (cl *Client) incomingPeerPort() int {
	if cl.listenAddr == "" {
		return 0
	}
	_, port, err := missinggo.ParseHostPort(cl.listenAddr)
	if err != nil {
		panic(err)
	}
	return port
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

func (pex *peerExtensionBytes) SupportsExtended() bool {
	return pex[5]&0x10 != 0
}

func (pex *peerExtensionBytes) SupportsDHT() bool {
	return pex[7]&0x01 != 0
}

func (pex *peerExtensionBytes) SupportsFast() bool {
	return pex[7]&0x04 != 0
}

type handshakeResult struct {
	peerExtensionBytes
	peerID
	metainfo.Hash
}

// ih is nil if we expect the peer to declare the InfoHash, such as when the
// peer initiated the connection. Returns ok if the handshake was successful,
// and err if there was an unexpected condition other than the peer simply
// abandoning the handshake.
func handshake(sock io.ReadWriter, ih *metainfo.Hash, peerID [20]byte, extensions peerExtensionBytes) (res handshakeResult, ok bool, err error) {
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
	missinggo.CopyExact(&res.peerExtensionBytes, b[20:28])
	missinggo.CopyExact(&res.Hash, b[28:48])
	missinggo.CopyExact(&res.peerID, b[48:68])
	peerExtensions.Add(hex.EncodeToString(res.peerExtensionBytes[:]), 1)

	// TODO: Maybe we can just drop peers here if we're not interested. This
	// could prevent them trying to reconnect, falsely believing there was
	// just a problem.
	if ih == nil { // We were waiting for the peer to tell us what they wanted.
		post(res.Hash[:])
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

func (r deadlineReader) Read(b []byte) (n int, err error) {
	// Keep-alives should be received every 2 mins. Give a bit of gracetime.
	err = r.nc.SetReadDeadline(time.Now().Add(150 * time.Second))
	if err != nil {
		err = fmt.Errorf("error setting read deadline: %s", err)
	}
	n, err = r.r.Read(b)
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

func maybeReceiveEncryptedHandshake(rw io.ReadWriter, skeys [][]byte) (ret io.ReadWriter, encrypted bool, err error) {
	var protocol [len(pp.Protocol)]byte
	_, err = io.ReadFull(rw, protocol[:])
	if err != nil {
		return
	}
	ret = struct {
		io.Reader
		io.Writer
	}{
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
		ret = append(ret, append([]byte(nil), ih[:]...))
	}
	return
}

func (cl *Client) initiateHandshakes(c *connection, t *Torrent) (ok bool, err error) {
	if c.encrypted {
		var rw io.ReadWriter
		rw, err = mse.InitiateHandshake(struct {
			io.Reader
			io.Writer
		}{c.r, c.w}, t.infoHash[:], nil)
		c.setRW(rw)
		if err != nil {
			return
		}
	}
	ih, ok, err := cl.connBTHandshake(c, &t.infoHash)
	if ih != t.infoHash {
		ok = false
	}
	return
}

// Do encryption and bittorrent handshakes as receiver.
func (cl *Client) receiveHandshakes(c *connection) (t *Torrent, err error) {
	cl.mu.Lock()
	skeys := cl.receiveSkeys()
	cl.mu.Unlock()
	if !cl.config.DisableEncryption {
		var rw io.ReadWriter
		rw, c.encrypted, err = maybeReceiveEncryptedHandshake(c.rw(), skeys)
		c.setRW(rw)
		if err != nil {
			if err == mse.ErrNoSecretKeyMatch {
				err = nil
			}
			return
		}
	}
	if cl.config.ForceEncryption && !c.encrypted {
		err = errors.New("connection not encrypted")
		return
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
func (cl *Client) connBTHandshake(c *connection, ih *metainfo.Hash) (ret metainfo.Hash, ok bool, err error) {
	res, ok, err := handshake(c.rw(), ih, cl.peerID, cl.extensionBytes)
	if err != nil || !ok {
		return
	}
	ret = res.Hash
	c.PeerExtensionBytes = res.peerExtensionBytes
	c.PeerID = res.peerID
	c.completedHandshake = time.Now()
	return
}

func (cl *Client) runInitiatedHandshookConn(c *connection, t *Torrent) {
	if c.PeerID == cl.peerID {
		connsToSelf.Add(1)
		addr := c.conn.RemoteAddr().String()
		cl.dopplegangerAddrs[addr] = struct{}{}
		return
	}
	cl.runHandshookConn(c, t)
}

func (cl *Client) runReceivedConn(c *connection) {
	err := c.conn.SetDeadline(time.Now().Add(handshakesTimeout))
	if err != nil {
		panic(err)
	}
	t, err := cl.receiveHandshakes(c)
	if err != nil {
		if cl.config.Debug {
			log.Printf("error receiving handshakes: %s", err)
		}
		return
	}
	if t == nil {
		return
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if c.PeerID == cl.peerID {
		// Because the remote address is not necessarily the same as its
		// client's torrent listen address, we won't record the remote address
		// as a doppleganger. Instead, the initiator can record *us* as the
		// doppleganger.
		return
	}
	cl.runHandshookConn(c, t)
}

func (cl *Client) runHandshookConn(c *connection, t *Torrent) {
	c.conn.SetWriteDeadline(time.Time{})
	c.r = deadlineReader{c.conn, c.r}
	completedHandshakeConnectionFlags.Add(c.connectionFlags(), 1)
	if !t.addConnection(c) {
		return
	}
	defer t.dropConnection(c)
	go c.writer(time.Minute)
	cl.sendInitialMessages(c, t)
	err := c.mainReadLoop()
	if err != nil && cl.config.Debug {
		log.Printf("error during connection loop: %s", err)
	}
}

func (cl *Client) sendInitialMessages(conn *connection, torrent *Torrent) {
	if conn.PeerExtensionBytes.SupportsExtended() && cl.extensionBytes.SupportsExtended() {
		conn.Post(pp.Message{
			Type:       pp.Extended,
			ExtendedID: pp.HandshakeExtendedID,
			ExtendedPayload: func() []byte {
				d := map[string]interface{}{
					"m": func() (ret map[string]int) {
						ret = make(map[string]int, 2)
						ret["ut_metadata"] = metadataExtendedId
						if !cl.config.DisablePEX {
							ret["ut_pex"] = pexExtendedId
						}
						return
					}(),
					"v": extendedHandshakeClientVersion,
					// No upload queue is implemented yet.
					"reqq": 64,
				}
				if !cl.config.DisableEncryption {
					d["e"] = 1
				}
				if torrent.metadataSizeKnown() {
					d["metadata_size"] = torrent.metadataSize()
				}
				if p := cl.incomingPeerPort(); p != 0 {
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
	} else if cl.extensionBytes.SupportsFast() && conn.PeerExtensionBytes.SupportsFast() {
		conn.Post(pp.Message{
			Type: pp.HaveNone,
		})
	}
	if conn.PeerExtensionBytes.SupportsDHT() && cl.extensionBytes.SupportsDHT() && cl.dHT != nil {
		conn.Post(pp.Message{
			Type: pp.Port,
			Port: uint16(missinggo.AddrPort(cl.dHT.Addr())),
		})
	}
}

func (cl *Client) peerUnchoked(torrent *Torrent, conn *connection) {
	conn.updateRequests()
}

func (cl *Client) connCancel(t *Torrent, cn *connection, r request) (ok bool) {
	ok = cn.Cancel(r)
	if ok {
		postedCancels.Add(1)
	}
	return
}

func (cl *Client) connDeleteRequest(t *Torrent, cn *connection, r request) bool {
	if !cn.RequestPending(r) {
		return false
	}
	delete(cn.Requests, r)
	return true
}

// Process incoming ut_metadata message.
func (cl *Client) gotMetadataExtensionMsg(payload []byte, t *Torrent, c *connection) error {
	var d map[string]int
	err := bencode.Unmarshal(payload, &d)
	if err != nil {
		return fmt.Errorf("error unmarshalling payload: %s: %q", err, payload)
	}
	msgType, ok := d["msg_type"]
	if !ok {
		return errors.New("missing msg_type field")
	}
	piece := d["piece"]
	switch msgType {
	case pp.DataMetadataExtensionMsgType:
		if !c.requestedMetadataPiece(piece) {
			return fmt.Errorf("got unexpected piece %d", piece)
		}
		c.metadataRequests[piece] = false
		begin := len(payload) - metadataPieceSize(d["total_size"], piece)
		if begin < 0 || begin >= len(payload) {
			return fmt.Errorf("data has bad offset in payload: %d", begin)
		}
		t.saveMetadataPiece(piece, payload[begin:])
		c.UsefulChunksReceived++
		c.lastUsefulChunkReceived = time.Now()
		return t.maybeCompleteMetadata()
	case pp.RequestMetadataExtensionMsgType:
		if !t.haveMetadataPiece(piece) {
			c.Post(t.newMetadataExtensionMessage(c, pp.RejectMetadataExtensionMsgType, d["piece"], nil))
			return nil
		}
		start := (1 << 14) * piece
		c.Post(t.newMetadataExtensionMessage(c, pp.DataMetadataExtensionMsgType, piece, t.metadataBytes[start:start+t.metadataPieceSize(piece)]))
		return nil
	case pp.RejectMetadataExtensionMsgType:
		return nil
	default:
		return errors.New("unknown msg_type value")
	}
}

// Also handles choking and unchoking of the remote peer.
func (cl *Client) upload(t *Torrent, c *connection) {
	if cl.config.NoUpload {
		return
	}
	if !c.PeerInterested {
		return
	}
	seeding := t.seeding()
	if !seeding && !t.connHasWantedPieces(c) {
		// There's no reason to upload to this peer.
		return
	}
	// Breaking or completing this loop means we don't want to upload to the
	// peer anymore, and we choke them.
another:
	for seeding || c.chunksSent < c.UsefulChunksReceived+6 {
		// We want to upload to the peer.
		c.Unchoke()
		for r := range c.PeerRequests {
			res := cl.uploadLimit.ReserveN(time.Now(), int(r.Length))
			delay := res.Delay()
			if delay > 0 {
				res.Cancel()
				go func() {
					time.Sleep(delay)
					cl.mu.Lock()
					defer cl.mu.Unlock()
					cl.upload(t, c)
				}()
				return
			}
			err := cl.sendChunk(t, c, r)
			if err != nil {
				i := int(r.Index)
				if t.pieceComplete(i) {
					t.updatePieceCompletion(i)
					if !t.pieceComplete(i) {
						// We had the piece, but not anymore.
						break another
					}
				}
				log.Printf("error sending chunk %+v to peer: %s", r, err)
				// If we failed to send a chunk, choke the peer to ensure they
				// flush all their requests. We've probably dropped a piece,
				// but there's no way to communicate this to the peer. If they
				// ask for it again, we'll kick them to allow us to send them
				// an updated bitfield.
				break another
			}
			delete(c.PeerRequests, r)
			goto another
		}
		return
	}
	c.Choke()
}

func (cl *Client) sendChunk(t *Torrent, c *connection, r request) error {
	// Count the chunk being sent, even if it isn't.
	b := make([]byte, r.Length)
	p := t.info.Piece(int(r.Index))
	n, err := t.readAt(b, p.Offset()+int64(r.Begin))
	if n != len(b) {
		if err == nil {
			panic("expected error")
		}
		return err
	}
	c.Post(pp.Message{
		Type:  pp.Piece,
		Index: r.Index,
		Begin: r.Begin,
		Piece: b,
	})
	c.chunksSent++
	uploadChunksPosted.Add(1)
	c.lastChunkSent = time.Now()
	return nil
}

func (cl *Client) openNewConns(t *Torrent) {
	defer t.updateWantPeersEvent()
	for len(t.peers) != 0 {
		if !t.wantConns() {
			return
		}
		if len(t.halfOpen) >= cl.halfOpenLimit {
			return
		}
		var (
			k peersKey
			p Peer
		)
		for k, p = range t.peers {
			break
		}
		delete(t.peers, k)
		cl.initiateConn(p, t)
	}
}

func (cl *Client) badPeerIPPort(ip net.IP, port int) bool {
	if port == 0 {
		return true
	}
	if cl.dopplegangerAddr(net.JoinHostPort(ip.String(), strconv.FormatInt(int64(port), 10))) {
		return true
	}
	if _, ok := cl.ipBlockRange(ip); ok {
		return true
	}
	if _, ok := cl.badPeerIPs[ip.String()]; ok {
		return true
	}
	return false
}

// Return a Torrent ready for insertion into a Client.
func (cl *Client) newTorrent(ih metainfo.Hash) (t *Torrent) {
	t = &Torrent{
		cl:       cl,
		infoHash: ih,
		peers:    make(map[peersKey]Peer),

		halfOpen:          make(map[string]struct{}),
		pieceStateChanges: pubsub.NewPubSub(),

		storageOpener:       cl.defaultStorage,
		maxEstablishedConns: defaultEstablishedConnsPerTorrent,
	}
	t.setChunkSize(defaultChunkSize)
	return
}

// A file-like handle to some torrent data resource.
type Handle interface {
	io.Reader
	io.Seeker
	io.Closer
	io.ReaderAt
}

func (cl *Client) AddTorrentInfoHash(infoHash metainfo.Hash) (t *Torrent, new bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	t, ok := cl.torrents[infoHash]
	if ok {
		return
	}
	new = true
	t = cl.newTorrent(infoHash)
	if cl.dHT != nil {
		go t.dhtAnnouncer()
	}
	cl.torrents[infoHash] = t
	t.updateWantPeersEvent()
	// Tickle Client.waitAccept, new torrent may want conns.
	cl.event.Broadcast()
	return
}

// Add or merge a torrent spec. If the torrent is already present, the
// trackers will be merged with the existing ones. If the Info isn't yet
// known, it will be set. The display name is replaced if the new spec
// provides one. Returns new if the torrent wasn't already in the client.
func (cl *Client) AddTorrentSpec(spec *TorrentSpec) (t *Torrent, new bool, err error) {
	t, new = cl.AddTorrentInfoHash(spec.InfoHash)
	if spec.DisplayName != "" {
		t.SetDisplayName(spec.DisplayName)
	}
	if spec.InfoBytes != nil {
		err = t.SetInfoBytes(spec.InfoBytes)
		if err != nil {
			return
		}
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if spec.ChunkSize != 0 {
		t.setChunkSize(pp.Integer(spec.ChunkSize))
	}
	t.addTrackers(spec.Trackers)
	t.maybeNewConns()
	return
}

func (cl *Client) dropTorrent(infoHash metainfo.Hash) (err error) {
	t, ok := cl.torrents[infoHash]
	if !ok {
		err = fmt.Errorf("no such torrent")
		return
	}
	err = t.close()
	if err != nil {
		panic(err)
	}
	delete(cl.torrents, infoHash)
	return
}

func (cl *Client) prepareTrackerAnnounceUnlocked(announceURL string) (blocked bool, urlToUse string, host string, err error) {
	_url, err := url.Parse(announceURL)
	if err != nil {
		return
	}
	hmp := missinggo.SplitHostMaybePort(_url.Host)
	if hmp.Err != nil {
		err = hmp.Err
		return
	}
	addr, err := net.ResolveIPAddr("ip", hmp.Host)
	if err != nil {
		return
	}
	cl.mu.RLock()
	_, blocked = cl.ipBlockRange(addr.IP)
	cl.mu.RUnlock()
	host = _url.Host
	hmp.Host = addr.String()
	_url.Host = hmp.String()
	urlToUse = _url.String()
	return
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
func (cl *Client) WaitAll() bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	for !cl.allTorrentsCompleted() {
		if cl.closed.IsSet() {
			return false
		}
		cl.event.Wait()
	}
	return true
}

// Handle a received chunk from a peer.
func (cl *Client) downloadedChunk(t *Torrent, c *connection, msg *pp.Message) {
	chunksReceived.Add(1)

	req := newRequest(msg.Index, msg.Begin, pp.Integer(len(msg.Piece)))

	// Request has been satisfied.
	if cl.connDeleteRequest(t, c, req) {
		defer c.updateRequests()
	} else {
		unexpectedChunksReceived.Add(1)
	}

	index := int(req.Index)
	piece := &t.pieces[index]

	// Do we actually want this chunk?
	if !t.wantPiece(req) {
		unwantedChunksReceived.Add(1)
		c.UnwantedChunksReceived++
		return
	}

	c.UsefulChunksReceived++
	c.lastUsefulChunkReceived = time.Now()

	cl.upload(t, c)

	// Need to record that it hasn't been written yet, before we attempt to do
	// anything with it.
	piece.incrementPendingWrites()
	// Record that we have the chunk.
	piece.unpendChunkIndex(chunkIndex(req.chunkSpec, t.chunkSize))

	// Cancel pending requests for this chunk.
	for _, c := range t.conns {
		if cl.connCancel(t, c, req) {
			c.updateRequests()
		}
	}

	cl.mu.Unlock()
	// Write the chunk out. Note that the upper bound on chunk writing
	// concurrency will be the number of connections.
	err := t.writeChunk(int(msg.Index), int64(msg.Begin), msg.Piece)
	cl.mu.Lock()

	piece.decrementPendingWrites()

	if err != nil {
		log.Printf("%s: error writing chunk %v: %s", t, req, err)
		t.pendRequest(req)
		t.updatePieceCompletion(int(msg.Index))
		return
	}

	// It's important that the piece is potentially queued before we check if
	// the piece is still wanted, because if it is queued, it won't be wanted.
	if t.pieceAllDirty(index) {
		cl.queuePieceCheck(t, int(req.Index))
	}

	if c.peerTouchedPieces == nil {
		c.peerTouchedPieces = make(map[int]struct{})
	}
	c.peerTouchedPieces[index] = struct{}{}

	cl.event.Broadcast()
	t.publishPieceChange(int(req.Index))
	return
}

// Return the connections that touched a piece, and clear the entry while
// doing it.
func (cl *Client) reapPieceTouches(t *Torrent, piece int) (ret []*connection) {
	for _, c := range t.conns {
		if _, ok := c.peerTouchedPieces[piece]; ok {
			ret = append(ret, c)
			delete(c.peerTouchedPieces, piece)
		}
	}
	return
}

func (cl *Client) pieceHashed(t *Torrent, piece int, correct bool) {
	if t.closed.IsSet() {
		return
	}
	p := &t.pieces[piece]
	if p.EverHashed {
		// Don't score the first time a piece is hashed, it could be an
		// initial check.
		if correct {
			pieceHashedCorrect.Add(1)
		} else {
			log.Printf("%s: piece %d (%x) failed hash", t, piece, p.Hash)
			pieceHashedNotCorrect.Add(1)
		}
	}
	p.EverHashed = true
	touchers := cl.reapPieceTouches(t, piece)
	if correct {
		for _, c := range touchers {
			c.goodPiecesDirtied++
		}
		err := p.Storage().MarkComplete()
		if err != nil {
			log.Printf("%T: error completing piece %d: %s", t.storage, piece, err)
		}
		t.updatePieceCompletion(piece)
	} else if len(touchers) != 0 {
		log.Printf("dropping and banning %d conns that touched piece", len(touchers))
		for _, c := range touchers {
			c.badPiecesDirtied++
			t.cl.banPeerIP(missinggo.AddrIP(c.remoteAddr()))
			t.dropConnection(c)
		}
	}
	cl.pieceChanged(t, piece)
}

func (cl *Client) onCompletedPiece(t *Torrent, piece int) {
	t.pendingPieces.Remove(piece)
	t.pendAllChunkSpecs(piece)
	for _, conn := range t.conns {
		conn.Have(piece)
		for r := range conn.Requests {
			if int(r.Index) == piece {
				conn.Cancel(r)
			}
		}
		// Could check here if peer doesn't have piece, but due to caching
		// some peers may have said they have a piece but they don't.
		cl.upload(t, conn)
	}
}

func (cl *Client) onFailedPiece(t *Torrent, piece int) {
	if t.pieceAllDirty(piece) {
		t.pendAllChunkSpecs(piece)
	}
	if !t.wantPieceIndex(piece) {
		return
	}
	cl.openNewConns(t)
	for _, conn := range t.conns {
		if conn.PeerHasPiece(piece) {
			conn.updateRequests()
		}
	}
}

func (cl *Client) pieceChanged(t *Torrent, piece int) {
	correct := t.pieceComplete(piece)
	defer cl.event.Broadcast()
	if correct {
		cl.onCompletedPiece(t, piece)
	} else {
		cl.onFailedPiece(t, piece)
	}
	t.updatePiecePriority(piece)
}

func (cl *Client) verifyPiece(t *Torrent, piece int) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	p := &t.pieces[piece]
	for p.Hashing || t.storage == nil {
		cl.event.Wait()
	}
	p.QueuedForHash = false
	if t.closed.IsSet() || t.pieceComplete(piece) {
		t.updatePiecePriority(piece)
		return
	}
	p.Hashing = true
	t.publishPieceChange(piece)
	cl.mu.Unlock()
	sum := t.hashPiece(piece)
	cl.mu.Lock()
	p.Hashing = false
	cl.pieceHashed(t, piece, sum == p.Hash)
}

// Returns handles to all the torrents loaded in the Client.
func (cl *Client) Torrents() (ret []*Torrent) {
	cl.mu.Lock()
	for _, t := range cl.torrents {
		ret = append(ret, t)
	}
	cl.mu.Unlock()
	return
}

func (cl *Client) AddMagnet(uri string) (T *Torrent, err error) {
	spec, err := TorrentSpecFromMagnetURI(uri)
	if err != nil {
		return
	}
	T, _, err = cl.AddTorrentSpec(spec)
	return
}

func (cl *Client) AddTorrent(mi *metainfo.MetaInfo) (T *Torrent, err error) {
	T, _, err = cl.AddTorrentSpec(TorrentSpecFromMetaInfo(mi))
	var ss []string
	slices.MakeInto(&ss, mi.Nodes)
	cl.AddDHTNodes(ss)
	return
}

func (cl *Client) AddTorrentFromFile(filename string) (T *Torrent, err error) {
	mi, err := metainfo.LoadFromFile(filename)
	if err != nil {
		return
	}
	return cl.AddTorrent(mi)
}

func (cl *Client) DHT() *dht.Server {
	return cl.dHT
}

func (cl *Client) AddDHTNodes(nodes []string) {
	for _, n := range nodes {
		hmp := missinggo.SplitHostMaybePort(n)
		ip := net.ParseIP(hmp.Host)
		if ip == nil {
			log.Printf("won't add DHT node with bad IP: %q", hmp.Host)
			continue
		}
		ni := krpc.NodeInfo{
			Addr: &net.UDPAddr{
				IP:   ip,
				Port: hmp.Port,
			},
		}
		cl.DHT().AddNode(ni)
	}
}

func (cl *Client) banPeerIP(ip net.IP) {
	if cl.badPeerIPs == nil {
		cl.badPeerIPs = make(map[string]struct{})
	}
	cl.badPeerIPs[ip.String()] = struct{}{}
}

func (cl *Client) newConnection(nc net.Conn) (c *connection) {
	c = &connection{
		conn: nc,

		Choked:          true,
		PeerChoked:      true,
		PeerMaxRequests: 250,
	}
	c.setRW(connStatsReadWriter{nc, &cl.mu, c})
	c.r = rateLimitedReader{cl.downloadLimit, c.r}
	return
}
