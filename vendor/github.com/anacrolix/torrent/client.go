package torrent

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/anacrolix/dht"
	"github.com/anacrolix/dht/krpc"
	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/pproffd"
	"github.com/anacrolix/missinggo/pubsub"
	"github.com/anacrolix/missinggo/slices"
	"github.com/anacrolix/sync"
	"github.com/dustin/go-humanize"
	"golang.org/x/time/rate"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/iplist"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/mse"
	pp "github.com/anacrolix/torrent/peer_protocol"
	"github.com/anacrolix/torrent/storage"
)

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
	onClose        []func()
	tcpListener    net.Listener
	utpSock        utpSocket
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
	return cl.badPeerIPsLocked()
}

func (cl *Client) badPeerIPsLocked() []string {
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

func (torrentAddr) Network() string { return "" }

func (me torrentAddr) String() string { return string(me) }

func (cl *Client) ListenAddr() net.Addr {
	if cl.listenAddr == "" {
		return nil
	}
	return torrentAddr(cl.listenAddr)
}

// Writes out a human readable status of the client, such as for writing to a
// HTTP status page.
func (cl *Client) WriteStatus(_w io.Writer) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	w := bufio.NewWriter(_w)
	defer w.Flush()
	if addr := cl.ListenAddr(); addr != nil {
		fmt.Fprintf(w, "Listening on %s\n", addr)
	} else {
		fmt.Fprintln(w, "Not listening!")
	}
	fmt.Fprintf(w, "Peer ID: %+q\n", cl.PeerID())
	fmt.Fprintf(w, "Banned IPs: %d\n", len(cl.badPeerIPsLocked()))
	if dht := cl.DHT(); dht != nil {
		dhtStats := dht.Stats()
		fmt.Fprintf(w, "DHT nodes: %d (%d good, %d banned)\n", dhtStats.Nodes, dhtStats.GoodNodes, dhtStats.BadNodes)
		fmt.Fprintf(w, "DHT Server ID: %x\n", dht.ID())
		fmt.Fprintf(w, "DHT port: %d\n", missinggo.AddrPort(dht.Addr()))
		fmt.Fprintf(w, "DHT announces: %d\n", dhtStats.ConfirmedAnnounces)
		fmt.Fprintf(w, "Outstanding transactions: %d\n", dhtStats.OutstandingTransactions)
	}
	fmt.Fprintf(w, "# Torrents: %d\n", len(cl.torrentsAsSlice()))
	fmt.Fprintln(w)
	for _, t := range slices.Sort(cl.torrentsAsSlice(), func(l, r *Torrent) bool {
		return l.InfoHash().AsString() < r.InfoHash().AsString()
	}).([]*Torrent) {
		if t.name() == "" {
			fmt.Fprint(w, "<unknown name>")
		} else {
			fmt.Fprint(w, t.name())
		}
		fmt.Fprint(w, "\n")
		if t.info != nil {
			fmt.Fprintf(w, "%f%% of %d bytes (%s)", 100*(1-float64(t.bytesMissingLocked())/float64(t.info.TotalLength())), t.length, humanize.Bytes(uint64(t.info.TotalLength())))
		} else {
			w.WriteString("<missing metainfo>")
		}
		fmt.Fprint(w, "\n")
		t.writeStatus(w)
		fmt.Fprintln(w)
	}
}

func listenUTP(networkSuffix, addr string) (utpSocket, error) {
	return NewUtpSocket("udp"+networkSuffix, addr)
}

func listenTCP(networkSuffix, addr string) (net.Listener, error) {
	return net.Listen("tcp"+networkSuffix, addr)
}

func listenBothSameDynamicPort(networkSuffix, host string) (tcpL net.Listener, utpSock utpSocket, listenedAddr string, err error) {
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
func listen(tcp, utp bool, networkSuffix, addr string) (tcpL net.Listener, utpSock utpSocket, listenedAddr string, err error) {
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
		cfg = &Config{
			DHTConfig: dht.ServerConfig{
				StartingNodes: dht.GlobalBootstrapAddrs,
			},
		}
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
	defer func() {
		if err == nil {
			return
		}
		cl.Close()
	}()
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
		// We'd use mmap but HFS+ doesn't support sparse files.
		storageImpl = storage.NewFile(cfg.DataDir)
		cl.onClose = append(cl.onClose, func() {
			if err := storageImpl.Close(); err != nil {
				log.Printf("error closing default storage: %s", err)
			}
		})
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
		if dhtCfg.Conn == nil {
			if cl.utpSock != nil {
				dhtCfg.Conn = cl.utpSock
			} else {
				dhtCfg.Conn, err = net.ListenPacket("udp", firstNonEmptyString(cl.listenAddr, cl.config.ListenAddr))
				if err != nil {
					return
				}
			}
		}
		if dhtCfg.OnAnnouncePeer == nil {
			dhtCfg.OnAnnouncePeer = cl.onDHTAnnouncePeer
		}
		cl.dHT, err = dht.NewServer(&dhtCfg)
		if err != nil {
			return
		}
		go func() {
			if _, err := cl.dHT.Bootstrap(); err != nil {
				log.Printf("error bootstrapping dht: %s", err)
			}
		}()
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
		cl.utpSock.Close()
	}
	if cl.tcpListener != nil {
		cl.tcpListener.Close()
	}
	for _, t := range cl.torrents {
		t.close()
	}
	for _, f := range cl.onClose {
		f()
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

func countDialResult(err error) {
	if err == nil {
		successfulDials.Add(1)
	} else {
		unsuccessfulDials.Add(1)
	}
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
	t.halfOpen[addr] = peer
	go cl.outgoingConnection(t, addr, peer.Source)
}

func (cl *Client) dialTCP(ctx context.Context, addr string) (c net.Conn, err error) {
	d := net.Dialer{
	// LocalAddr: cl.tcpListener.Addr(),
	}
	c, err = d.DialContext(ctx, "tcp", addr)
	countDialResult(err)
	if err == nil {
		c.(*net.TCPConn).SetLinger(0)
	}
	c = pproffd.WrapNetConn(c)
	return
}

func (cl *Client) dialUTP(ctx context.Context, addr string) (c net.Conn, err error) {
	c, err = cl.utpSock.DialContext(ctx, addr)
	countDialResult(err)
	return
}

var (
	dialledFirstUtp    = expvar.NewInt("dialledFirstUtp")
	dialledFirstNotUtp = expvar.NewInt("dialledFirstNotUtp")
)

// Returns a connection over UTP or TCP, whichever is first to connect.
func (cl *Client) dialFirst(ctx context.Context, addr string) (conn net.Conn, utp bool) {
	ctx, cancel := context.WithCancel(ctx)
	// As soon as we return one connection, cancel the others.
	defer cancel()
	left := 0
	resCh := make(chan dialResult, left)
	if !cl.config.DisableUTP {
		left++
		go func() {
			c, _ := cl.dialUTP(ctx, addr)
			resCh <- dialResult{c, true}
		}()
	}
	if !cl.config.DisableTCP {
		left++
		go func() {
			c, _ := cl.dialTCP(ctx, addr)
			resCh <- dialResult{c, false}
		}()
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
	if conn != nil {
		if utp {
			dialledFirstUtp.Add(1)
		} else {
			dialledFirstNotUtp.Add(1)
		}
	}
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
func (cl *Client) handshakesConnection(ctx context.Context, nc net.Conn, t *Torrent, encryptHeader, utp bool) (c *connection, err error) {
	c = cl.newConnection(nc)
	c.headerEncrypted = encryptHeader
	c.uTP = utp
	ctx, cancel := context.WithTimeout(ctx, handshakesTimeout)
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		panic(ctx)
	}
	err = nc.SetDeadline(dl)
	if err != nil {
		panic(err)
	}
	ok, err = cl.initiateHandshakes(c, t)
	if !ok {
		c = nil
	}
	return
}

var (
	initiatedConnWithPreferredHeaderEncryption = expvar.NewInt("initiatedConnWithPreferredHeaderEncryption")
	initiatedConnWithFallbackHeaderEncryption  = expvar.NewInt("initiatedConnWithFallbackHeaderEncryption")
)

// Returns nil connection and nil error if no connection could be established
// for valid reasons.
func (cl *Client) establishOutgoingConn(t *Torrent, addr string) (c *connection, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	nc, utp := cl.dialFirst(ctx, addr)
	if nc == nil {
		return
	}
	obfuscatedHeaderFirst := !cl.config.DisableEncryption && !cl.config.PreferNoEncryption
	c, err = cl.handshakesConnection(ctx, nc, t, obfuscatedHeaderFirst, utp)
	if err != nil {
		// log.Printf("error initiating connection handshakes: %s", err)
		nc.Close()
		return
	} else if c != nil {
		initiatedConnWithPreferredHeaderEncryption.Add(1)
		return
	}
	nc.Close()
	if cl.config.ForceEncryption {
		// We should have just tried with an obfuscated header. A plaintext
		// header can't result in an encrypted connection, so we're done.
		if !obfuscatedHeaderFirst {
			panic(cl.config.EncryptionPolicy)
		}
		return
	}
	// Try again with encryption if we didn't earlier, or without if we did,
	// using whichever protocol type worked last time.
	if utp {
		nc, err = cl.dialUTP(ctx, addr)
	} else {
		nc, err = cl.dialTCP(ctx, addr)
	}
	if err != nil {
		err = fmt.Errorf("error dialing for header encryption fallback: %s", err)
		return
	}
	c, err = cl.handshakesConnection(ctx, nc, t, !obfuscatedHeaderFirst, utp)
	if err != nil || c == nil {
		nc.Close()
	}
	if err == nil && c != nil {
		initiatedConnWithFallbackHeaderEncryption.Add(1)
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

func handleEncryption(
	rw io.ReadWriter,
	skeys mse.SecretKeyIter,
	policy EncryptionPolicy,
) (
	ret io.ReadWriter,
	headerEncrypted bool,
	cryptoMethod uint32,
	err error,
) {
	if !policy.ForceEncryption {
		var protocol [len(pp.Protocol)]byte
		_, err = io.ReadFull(rw, protocol[:])
		if err != nil {
			return
		}
		rw = struct {
			io.Reader
			io.Writer
		}{
			io.MultiReader(bytes.NewReader(protocol[:]), rw),
			rw,
		}
		if string(protocol[:]) == pp.Protocol {
			ret = rw
			return
		}
	}
	headerEncrypted = true
	ret, err = mse.ReceiveHandshake(rw, skeys, func(provides uint32) uint32 {
		cryptoMethod = func() uint32 {
			switch {
			case policy.ForceEncryption:
				return mse.CryptoMethodRC4
			case policy.DisableEncryption:
				return mse.CryptoMethodPlaintext
			case policy.PreferNoEncryption && provides&mse.CryptoMethodPlaintext != 0:
				return mse.CryptoMethodPlaintext
			default:
				return mse.DefaultCryptoSelector(provides)
			}
		}()
		return cryptoMethod
	})
	return
}

func (cl *Client) initiateHandshakes(c *connection, t *Torrent) (ok bool, err error) {
	if c.headerEncrypted {
		var rw io.ReadWriter
		rw, err = mse.InitiateHandshake(
			struct {
				io.Reader
				io.Writer
			}{c.r, c.w},
			t.infoHash[:],
			nil,
			func() uint32 {
				switch {
				case cl.config.ForceEncryption:
					return mse.CryptoMethodRC4
				case cl.config.DisableEncryption:
					return mse.CryptoMethodPlaintext
				default:
					return mse.AllSupportedCrypto
				}
			}(),
		)
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

// Calls f with any secret keys.
func (cl *Client) forSkeys(f func([]byte) bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	for ih := range cl.torrents {
		if !f(ih[:]) {
			break
		}
	}
}

// Do encryption and bittorrent handshakes as receiver.
func (cl *Client) receiveHandshakes(c *connection) (t *Torrent, err error) {
	var rw io.ReadWriter
	rw, c.headerEncrypted, c.cryptoMethod, err = handleEncryption(c.rw(), cl.forSkeys, cl.config.EncryptionPolicy)
	c.setRW(rw)
	if err != nil {
		if err == mse.ErrNoSecretKeyMatch {
			err = nil
		}
		return
	}
	if cl.config.ForceEncryption && !c.headerEncrypted {
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
	cl.runHandshookConn(c, t, true)
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
	cl.runHandshookConn(c, t, false)
}

func (cl *Client) runHandshookConn(c *connection, t *Torrent, outgoing bool) {
	c.conn.SetWriteDeadline(time.Time{})
	c.r = deadlineReader{c.conn, c.r}
	completedHandshakeConnectionFlags.Add(c.connectionFlags(), 1)
	if !t.addConnection(c, outgoing) {
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

func (cl *Client) sendChunk(t *Torrent, c *connection, r request, msg func(pp.Message) bool) (more bool, err error) {
	// Count the chunk being sent, even if it isn't.
	b := make([]byte, r.Length)
	p := t.info.Piece(int(r.Index))
	n, err := t.readAt(b, p.Offset()+int64(r.Begin))
	if n != len(b) {
		if err == nil {
			panic("expected error")
		}
		return
	}
	more = msg(pp.Message{
		Type:  pp.Piece,
		Index: r.Index,
		Begin: r.Begin,
		Piece: b,
	})
	c.chunksSent++
	uploadChunksPosted.Add(1)
	c.lastChunkSent = time.Now()
	return
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
func (cl *Client) newTorrent(ih metainfo.Hash, specStorage storage.ClientImpl) (t *Torrent) {
	// use provided storage, if provided
	storageClient := cl.defaultStorage
	if specStorage != nil {
		storageClient = storage.NewClient(specStorage)
	}

	t = &Torrent{
		cl:       cl,
		infoHash: ih,
		peers:    make(map[peersKey]Peer),
		conns:    make(map[*connection]struct{}, 2*defaultEstablishedConnsPerTorrent),

		halfOpen:          make(map[string]Peer),
		pieceStateChanges: pubsub.NewPubSub(),

		storageOpener:       storageClient,
		maxEstablishedConns: defaultEstablishedConnsPerTorrent,

		networkingEnabled: true,
		requestStrategy:   2,
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
	return cl.AddTorrentInfoHashWithStorage(infoHash, nil)
}

// Adds a torrent by InfoHash with a custom Storage implementation.
// If the torrent already exists then this Storage is ignored and the
// existing torrent returned with `new` set to `false`
func (cl *Client) AddTorrentInfoHashWithStorage(infoHash metainfo.Hash, specStorage storage.ClientImpl) (t *Torrent, new bool) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	t, ok := cl.torrents[infoHash]
	if ok {
		return
	}
	new = true
	t = cl.newTorrent(infoHash, specStorage)
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
// Note that any `Storage` defined on the spec will be ignored if the
// torrent is already present (i.e. `new` return value is `true`)
func (cl *Client) AddTorrentSpec(spec *TorrentSpec) (t *Torrent, new bool, err error) {
	t, new = cl.AddTorrentInfoHashWithStorage(spec.InfoHash, spec.Storage)
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

// Returns handles to all the torrents loaded in the Client.
func (cl *Client) Torrents() []*Torrent {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.torrentsAsSlice()
}

func (cl *Client) torrentsAsSlice() (ret []*Torrent) {
	for _, t := range cl.torrents {
		ret = append(ret, t)
	}
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
	if cl.DHT() == nil {
		return
	}
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
	c.writerCond.L = &cl.mu
	c.setRW(connStatsReadWriter{nc, &cl.mu, c})
	c.r = rateLimitedReader{cl.downloadLimit, c.r}
	return
}

func (cl *Client) onDHTAnnouncePeer(ih metainfo.Hash, p dht.Peer) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	t := cl.torrent(ih)
	if t == nil {
		return
	}
	t.addPeers([]Peer{{
		IP:     p.IP,
		Port:   p.Port,
		Source: peerSourceDHTAnnouncePeer,
	}})
}
