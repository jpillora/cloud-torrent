// Package utp implements uTP, the micro transport protocol as used with
// Bittorrent. It opts for simplicity and reliability over strict adherence to
// the (poor) spec. It allows using the underlying OS-level transport despite
// dispatching uTP on top to allow for example, shared socket use with DHT.
// Additionally, multiple uTP connections can share the same OS socket, to
// truly realize uTP's claim to be light on system and network switching
// resources.
//
// Socket is a wrapper of net.UDPConn, and performs dispatching of uTP packets
// to attached uTP Conns. Dial and Accept is done via Socket. Conn implements
// net.Conn over uTP, via aforementioned Socket.
package utp

import (
	"encoding/binary"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/anacrolix/jitter"
	"github.com/anacrolix/missinggo"
	"github.com/spacemonkeygo/monotime"
)

const (
	// Maximum received SYNs that haven't been accepted. If more SYNs are
	// received, a pseudo randomly selected SYN is replied to with a reset to
	// make room.
	backlog = 50

	// Experimentation on localhost on OSX gives me this value. It appears to
	// be the largest approximate datagram size before remote libutp starts
	// selectively acking.
	minMTU     = 576
	recvWindow = 0x8000 // 32KiB
	// uTP header of 20, +2 for the next extension, and 8 bytes of selective
	// ACK.
	maxHeaderSize  = 30
	maxPayloadSize = minMTU - maxHeaderSize
	maxRecvSize    = 0x2000

	// Maximum out-of-order packets to buffer.
	maxUnackedInbound = 64
)

var (
	ackSkippedResends = expvar.NewInt("utpAckSkippedResends")
	sendBufferPool    = sync.Pool{
		New: func() interface{} { return make([]byte, minMTU) },
	}
)

type deadlineCallback struct {
	deadline time.Time
	timer    *time.Timer
	callback func()
	inited   bool
}

func (me *deadlineCallback) deadlineExceeded() bool {
	return !me.deadline.IsZero() && !time.Now().Before(me.deadline)
}

func (me *deadlineCallback) updateTimer() {
	if me.timer != nil {
		me.timer.Stop()
	}
	if me.deadline.IsZero() {
		return
	}
	if me.callback == nil {
		panic("deadline callback is nil")
	}
	me.timer = time.AfterFunc(me.deadline.Sub(time.Now()), me.callback)
}

func (me *deadlineCallback) setDeadline(t time.Time) {
	me.deadline = t
	me.updateTimer()
}

func (me *deadlineCallback) setCallback(f func()) {
	me.callback = f
	me.updateTimer()
}

type connDeadlines struct {
	// mu          sync.Mutex
	read, write deadlineCallback
}

func (c *connDeadlines) SetDeadline(t time.Time) error {
	c.read.setDeadline(t)
	c.write.setDeadline(t)
	return nil
}

func (c *connDeadlines) SetReadDeadline(t time.Time) error {
	c.read.setDeadline(t)
	return nil
}

func (c *connDeadlines) SetWriteDeadline(t time.Time) error {
	c.write.setDeadline(t)
	return nil
}

// Strongly-type guarantee of resolved network address.
type resolvedAddrStr string

// Uniquely identifies any uTP connection on top of the underlying packet
// stream.
type connKey struct {
	remoteAddr resolvedAddrStr
	connID     uint16
}

// A Socket wraps a net.PacketConn, diverting uTP packets to its child uTP
// Conns.
type Socket struct {
	mu      sync.Mutex
	event   sync.Cond
	pc      net.PacketConn
	conns   map[connKey]*Conn
	backlog map[syn]struct{}
	reads   chan read
	closing chan struct{}

	raw packetConn
	// If a read error occurs on the underlying net.PacketConn, it is put
	// here. This is because reading is done in its own goroutine to dispatch
	// to uTP Conns.
	ReadErr error
}

// Returns a net.PacketConn that emulates the expected behaviour of the real
// net.PacketConn underlying the Socket. It is not however the real one, as
// Socket requires exclusive access to that.
func (s *Socket) PacketConn() net.PacketConn {
	return &s.raw
}

type packetConn struct {
	real net.PacketConn
	connDeadlines

	mu          sync.Mutex
	unusedReads chan read
	closed      bool
}

type read struct {
	data []byte
	from net.Addr
}

type syn struct {
	seq_nr, conn_id uint16
	addr            string
}

const (
	extensionTypeSelectiveAck = 1
)

type extensionField struct {
	Type  byte
	Bytes []byte
}

type header struct {
	Type          int
	Version       int
	ConnID        uint16
	Timestamp     uint32
	TimestampDiff uint32
	WndSize       uint32
	SeqNr         uint16
	AckNr         uint16
	Extensions    []extensionField
}

var (
	logLevel                   = 0
	artificialPacketDropChance = 0.0
)

func init() {
	logLevel, _ = strconv.Atoi(os.Getenv("GO_UTP_LOGGING"))
	fmt.Sscanf(os.Getenv("GO_UTP_PACKET_DROP"), "%f", &artificialPacketDropChance)

}

var (
	errClosed                   = errors.New("closed")
	errNotImplemented           = errors.New("not implemented")
	errTimeout        net.Error = timeoutError{"i/o timeout"}
	errAckTimeout               = timeoutError{"timed out waiting for ack"}
)

type timeoutError struct {
	msg string
}

func (me timeoutError) Timeout() bool   { return true }
func (me timeoutError) Error() string   { return me.msg }
func (me timeoutError) Temporary() bool { return false }

func unmarshalExtensions(_type byte, b []byte) (n int, ef []extensionField, err error) {
	for _type != 0 {
		if _type != extensionTypeSelectiveAck {
			// An extension type that is not known to us. Generally we're
			// unmarshalling an packet that isn't actually uTP but we don't
			// yet know for sure until we try to deliver it.

			// logonce.Stderr.Printf("utp extension %d", _type)
		}
		if len(b) < 2 || len(b) < int(b[1])+2 {
			err = fmt.Errorf("buffer ends prematurely: %x", b)
			return
		}
		ef = append(ef, extensionField{
			Type:  _type,
			Bytes: append([]byte{}, b[2:int(b[1])+2]...),
		})
		_type = b[0]
		n += 2 + int(b[1])
		b = b[2+int(b[1]):]
	}
	return
}

var errInvalidHeader = errors.New("invalid header")

func (h *header) Unmarshal(b []byte) (n int, err error) {
	h.Type = int(b[0] >> 4)
	h.Version = int(b[0] & 0xf)
	if h.Type > stMax || h.Version != 1 {
		err = errInvalidHeader
		return
	}
	n, h.Extensions, err = unmarshalExtensions(b[1], b[20:])
	if err != nil {
		return
	}
	h.ConnID = binary.BigEndian.Uint16(b[2:4])
	h.Timestamp = binary.BigEndian.Uint32(b[4:8])
	h.TimestampDiff = binary.BigEndian.Uint32(b[8:12])
	h.WndSize = binary.BigEndian.Uint32(b[12:16])
	h.SeqNr = binary.BigEndian.Uint16(b[16:18])
	h.AckNr = binary.BigEndian.Uint16(b[18:20])
	n += 20
	return
}

func (h *header) Marshal() (ret []byte) {
	hLen := 20 + func() (ret int) {
		for _, ext := range h.Extensions {
			ret += 2 + len(ext.Bytes)
		}
		return
	}()
	ret = sendBufferPool.Get().([]byte)[:hLen:minMTU]
	// ret = make([]byte, hLen, minMTU)
	p := ret // Used for manipulating ret.
	p[0] = byte(h.Type<<4 | 1)
	binary.BigEndian.PutUint16(p[2:4], h.ConnID)
	binary.BigEndian.PutUint32(p[4:8], h.Timestamp)
	binary.BigEndian.PutUint32(p[8:12], h.TimestampDiff)
	binary.BigEndian.PutUint32(p[12:16], h.WndSize)
	binary.BigEndian.PutUint16(p[16:18], h.SeqNr)
	binary.BigEndian.PutUint16(p[18:20], h.AckNr)
	// Pointer to the last type field so the next extension can set it.
	_type := &p[1]
	// We're done with the basic header.
	p = p[20:]
	for _, ext := range h.Extensions {
		*_type = ext.Type
		// The next extension's type will go here.
		_type = &p[0]
		p[1] = uint8(len(ext.Bytes))
		if int(p[1]) != copy(p[2:], ext.Bytes) {
			panic("unexpected extension length")
		}
		p = p[2+len(ext.Bytes):]
	}
	if len(p) != 0 {
		panic("header length changed")
	}
	return
}

var (
	_ net.Listener   = &Socket{}
	_ net.PacketConn = &packetConn{}
)

const (
	csInvalid = iota
	csSynSent
	csConnected
	csGotFin
	csSentFin
	csDestroy
)

const (
	stData = iota
	stFin
	stState
	stReset
	stSyn

	// Used for validating packet headers.
	stMax = stSyn
)

// Conn is a uTP stream and implements net.Conn. It owned by a Socket, which
// handles dispatching packets to and from Conns.
type Conn struct {
	mu    sync.Mutex
	event sync.Cond

	recv_id, send_id uint16
	seq_nr, ack_nr   uint16
	lastAck          uint16
	lastTimeDiff     uint32
	peerWndSize      uint32

	readBuf []byte

	socket     net.PacketConn
	remoteAddr net.Addr
	// The uTP timestamp.
	startTimestamp uint32
	// When the conn was allocated.
	created time.Time
	// Callback to unregister Conn from a parent Socket. Should be called when
	// no more packets will be handled.
	detach func()

	cs  int
	err error

	unackedSends []*send
	// Inbound payloads, the first is ack_nr+1.
	inbound []recv

	connDeadlines
}

type send struct {
	acked       chan struct{} // Closed with Conn lock.
	payloadSize uint32
	started     time.Time
	// This send was skipped in a selective ack.
	resend   func()
	timedOut func()

	mu          sync.Mutex
	acksSkipped int
	resendTimer *time.Timer
	numResends  int
}

func (s *send) Ack() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resendTimer.Stop()
	select {
	case <-s.acked:
	default:
		close(s.acked)
	}
}

type recv struct {
	seen bool
	data []byte
}

var (
	_ net.Conn = &Conn{}
)

func (c *Conn) age() time.Duration {
	return time.Since(c.created)
}

func (c *Conn) timestamp() uint32 {
	return nowTimestamp() - c.startTimestamp
}

func (c *Conn) connected() bool {
	return c.cs == csConnected
}

// addr is used to create a listening UDP conn which becomes the underlying
// net.PacketConn for the Socket.
func NewSocket(network, addr string) (s *Socket, err error) {
	s = &Socket{
		backlog: make(map[syn]struct{}, backlog),
		reads:   make(chan read, 1),
		closing: make(chan struct{}),
	}
	s.event.L = &s.mu
	s.pc, err = net.ListenPacket(network, addr)
	if err != nil {
		return
	}
	s.raw.unusedReads = make(chan read, 100)
	s.raw.real = s.pc
	go s.reader()
	go s.dispatcher()
	return
}

func packetDebugString(h *header, payload []byte) string {
	return fmt.Sprintf("%#v: %q", h, payload)
}

func (s *Socket) reader() {
	defer close(s.reads)
	var b [maxRecvSize]byte
	for {
		if s.pc == nil {
			break
		}
		n, addr, err := s.pc.ReadFrom(b[:])
		if err != nil {
			select {
			case <-s.closing:
			default:
				s.ReadErr = err
			}
			return
		}
		var nilB []byte
		s.reads <- read{append(nilB, b[:n:n]...), addr}
	}
}

func (s *Socket) unusedRead(read read) {
	// log.Printf("unused read from %q", read.from.String())
	s.raw.mu.Lock()
	defer s.raw.mu.Unlock()
	if s.raw.closed {
		return
	}
	select {
	case s.raw.unusedReads <- read:
	default:
	}
}

func stringAddr(s string) net.Addr {
	addr, err := net.ResolveUDPAddr("udp", s)
	if err != nil {
		panic(err)
	}
	return addr
}

func (s *Socket) pushBacklog(syn syn) {
	if _, ok := s.backlog[syn]; ok {
		return
	}
	for k := range s.backlog {
		if len(s.backlog) < backlog {
			break
		}
		delete(s.backlog, k)
		// A syn is sent on the remote's recv_id, so this is where we can send
		// the reset.
		s.reset(stringAddr(k.addr), k.seq_nr, k.conn_id)
	}
	s.backlog[syn] = struct{}{}
	s.event.Broadcast()
}

func (s *Socket) dispatcher() {
	for {
		read, ok := <-s.reads
		if !ok {
			return
		}
		if len(read.data) < 20 {
			s.unusedRead(read)
			continue
		}
		b := read.data
		addr := read.from
		var h header
		hEnd, err := h.Unmarshal(b)
		if logLevel >= 1 {
			log.Printf("recvd utp msg: %s", packetDebugString(&h, b[hEnd:]))
		}
		if err != nil || h.Type > stMax || h.Version != 1 {
			s.unusedRead(read)
			continue
		}
		s.mu.Lock()
		c, ok := s.conns[connKey{resolvedAddrStr(addr.String()), func() (recvID uint16) {
			recvID = h.ConnID
			// If a SYN is resent, its connection ID field will be one lower
			// than we expect.
			if h.Type == stSyn {
				recvID++
			}
			return
		}()}]
		s.mu.Unlock()
		if ok {
			if h.Type == stSyn {
				if h.ConnID == c.send_id-2 {
					// This is a SYN for connection that cannot exist locally. The
					// connection the remote wants to establish here with the proposed
					// recv_id, already has an existing connection that was dialled
					// *out* from this socket, which is why the send_id is 1 higher,
					// rather than 1 lower than the recv_id.
					log.Print("resetting conflicting syn")
					s.reset(addr, h.SeqNr, h.ConnID)
					continue
				} else if h.ConnID != c.send_id {
					panic("bad assumption")
				}
			}
			c.deliver(h, b[hEnd:])
			continue
		}
		if h.Type == stSyn {
			if logLevel >= 1 {
				log.Printf("adding SYN to backlog")
			}
			syn := syn{
				seq_nr:  h.SeqNr,
				conn_id: h.ConnID,
				addr:    addr.String(),
			}
			s.mu.Lock()
			s.pushBacklog(syn)
			s.mu.Unlock()
			continue
		} else if h.Type != stReset {
			// This is an unexpected packet. We'll send a reset, but also pass
			// it on.
			// log.Print("resetting unexpected packet")
			// s.reset(addr, h.SeqNr, h.ConnID)
		}
		s.unusedRead(read)
	}
}

// Send a reset in response to a packet with the given header.
func (s *Socket) reset(addr net.Addr, ackNr, connId uint16) {
	go s.pc.WriteTo((&header{
		Type:    stReset,
		Version: 1,
		ConnID:  connId,
		AckNr:   ackNr,
	}).Marshal(), addr)
}

// Attempt to connect to a remote uTP listener, creating a Socket just for
// this connection.
func Dial(addr string) (net.Conn, error) {
	return DialTimeout(addr, 0)
}

// Same as Dial with a timeout parameter.
func DialTimeout(addr string, timeout time.Duration) (nc net.Conn, err error) {
	s, err := NewSocket("udp", ":0")
	if err != nil {
		return
	}
	return s.DialTimeout(addr, timeout)

}

// Return a recv_id that should be free. Handling the case where it isn't is
// deferred to a more appropriate function.
func (s *Socket) newConnID(remoteAddr resolvedAddrStr) (id uint16) {
	// Rather than use math.Rand, which requires generating all the IDs up
	// front and allocating a slice, we do it on the stack, generating the IDs
	// only as required. To do this, we use the fact that the array is
	// default-initialized. IDs that are 0, are actually their index in the
	// array. IDs that are non-zero, are +1 from their intended ID.
	var idsBack [0x10000]int
	ids := idsBack[:]
	for len(ids) != 0 {
		// Pick the next ID from the untried ids.
		i := rand.Intn(len(ids))
		id = uint16(ids[i])
		// If it's zero, then treat it as though the index i was the ID.
		// Otherwise the value we get is the ID+1.
		if id == 0 {
			id = uint16(i)
		} else {
			id--
		}
		// Check there's no connection using this ID for its recv_id...
		_, ok1 := s.conns[connKey{remoteAddr, id}]
		// and if we're connecting to our own Socket, that there isn't a Conn
		// already receiving on what will correspond to our send_id. Note that
		// we just assume that we could be connecting to our own Socket. This
		// will halve the available connection IDs to each distinct remote
		// address. Presumably that's ~0x8000, down from ~0x10000.
		_, ok2 := s.conns[connKey{remoteAddr, id + 1}]
		_, ok4 := s.conns[connKey{remoteAddr, id - 1}]
		if !ok1 && !ok2 && !ok4 {
			return
		}
		// The set of possible IDs is shrinking. The highest one will be lost, so
		// it's moved to the location of the one we just tried.
		ids[i] = len(ids) // Conveniently already +1.
		// And shrink.
		ids = ids[:len(ids)-1]
	}
	return
}

func (s *Socket) newConn(addr net.Addr) (c *Conn) {
	c = &Conn{
		socket:         s.pc,
		remoteAddr:     addr,
		startTimestamp: nowTimestamp(),
		created:        time.Now(),
	}
	c.event.L = &c.mu
	c.connDeadlines.read.setCallback(func() {
		c.mu.Lock()
		c.event.Broadcast()
		c.mu.Unlock()
	})
	c.connDeadlines.write.setCallback(func() {
		c.mu.Lock()
		c.event.Broadcast()
		c.mu.Unlock()
	})
	return
}

func (s *Socket) Dial(addr string) (net.Conn, error) {
	return s.DialTimeout(addr, 0)
}

func (s *Socket) DialTimeout(addr string, timeout time.Duration) (nc net.Conn, err error) {
	netAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return
	}

	s.mu.Lock()
	c := s.newConn(netAddr)
	c.recv_id = s.newConnID(resolvedAddrStr(netAddr.String()))
	c.send_id = c.recv_id + 1
	if logLevel >= 1 {
		log.Printf("dial registering addr: %s", netAddr.String())
	}
	if !s.registerConn(c.recv_id, resolvedAddrStr(netAddr.String()), c) {
		err = errors.New("couldn't register new connection")
		log.Println(c.recv_id, netAddr.String())
		for k, c := range s.conns {
			log.Println(k, c, c.age())
		}
		log.Printf("that's %d connections", len(s.conns))
	}
	s.mu.Unlock()
	if err != nil {
		return
	}

	connErr := make(chan error, 1)
	go func() {
		connErr <- c.connect()
	}()
	var timeoutCh <-chan time.Time
	if timeout != 0 {
		timeoutCh = time.After(timeout)
	}
	select {
	case err = <-connErr:
	case <-timeoutCh:
		c.Close()
		err = errTimeout
	}
	if err == nil {
		nc = c
	}
	return
}

func (c *Conn) wndSize() uint32 {
	if len(c.inbound) > maxUnackedInbound/2 {
		return 0
	}
	var buffered int
	for _, r := range c.inbound {
		buffered += len(r.data)
	}
	buffered += len(c.readBuf)
	if buffered >= recvWindow {
		return 0
	}
	return recvWindow - uint32(buffered)
}

func nowTimestamp() uint32 {
	return uint32(monotime.Monotonic() / time.Microsecond)
}

// Send the given payload with an up to date header.
func (c *Conn) send(_type int, connID uint16, payload []byte, seqNr uint16) (err error) {
	selAck := selectiveAckBitmask(make([]byte, 8))
	for i := 1; i < 65; i++ {
		if len(c.inbound) <= i {
			break
		}
		if c.inbound[i].seen {
			selAck.SetBit(i - 1)
		}
	}
	h := header{
		Type:          _type,
		Version:       1,
		ConnID:        connID,
		SeqNr:         seqNr,
		AckNr:         c.ack_nr,
		WndSize:       c.wndSize(),
		Timestamp:     c.timestamp(),
		TimestampDiff: c.lastTimeDiff,
		// Currently always send an 8 byte selective ack.
		Extensions: []extensionField{{
			Type:  extensionTypeSelectiveAck,
			Bytes: selAck,
		}},
	}
	p := h.Marshal()
	sendBufferPool.Put(p)
	// Extension headers are currently fixed in size.
	if len(p) != maxHeaderSize {
		panic("header has unexpected size")
	}
	if artificialPacketDropChance != 0 {
		if rand.Float64() < artificialPacketDropChance {
			return nil
		}
	}
	p = append(p, payload...)
	if logLevel >= 1 {
		log.Printf("writing utp msg to %s: %s", c.remoteAddr, packetDebugString(&h, payload))
	}
	n1, err := c.socket.WriteTo(p, c.remoteAddr)
	if err != nil {
		return
	}
	if n1 != len(p) {
		panic(n1)
	}
	return
}

func (s *send) resendTimeout() time.Duration {
	return jitter.Duration(3*time.Second, time.Second)
}

func (s *send) timeoutResend() {
	select {
	case <-s.acked:
		return
	default:
	}
	if time.Since(s.started) >= 15*time.Second {
		s.timedOut()
		return
	}
	go s.resend()
	s.mu.Lock()
	s.numResends++
	s.resendTimer.Reset(s.resendTimeout())
	s.mu.Unlock()
}

func (c *Conn) write(_type int, connID uint16, payload []byte, seqNr uint16) (n int, err error) {
	if c.cs == csDestroy {
		err = errors.New("conn being destroyed")
		return
	}
	if len(payload) > maxPayloadSize {
		payload = payload[:maxPayloadSize]
	}
	err = c.send(_type, connID, payload, seqNr)
	if err != nil {
		return
	}
	n = len(payload)
	// State messages aren't acknowledged, so there's nothing to resend.
	if _type != stState {
		// Copy payload so caller to write can continue to use the buffer.
		payload = append(sendBufferPool.Get().([]byte)[:0:minMTU], payload...)
		send := &send{
			acked:       make(chan struct{}),
			payloadSize: uint32(len(payload)),
			started:     time.Now(),
			resend: func() {
				c.mu.Lock()
				c.send(_type, connID, payload, seqNr)
				c.mu.Unlock()
			},
			timedOut: func() {
				c.mu.Lock()
				c.destroy(errAckTimeout)
				c.mu.Unlock()
			},
		}
		send.mu.Lock()
		send.resendTimer = time.AfterFunc(send.resendTimeout(), send.timeoutResend)
		send.mu.Unlock()
		c.unackedSends = append(c.unackedSends, send)
	}
	return
}

func (c *Conn) numUnackedSends() (num int) {
	for _, s := range c.unackedSends {
		select {
		case <-s.acked:
		default:
			num++
		}
	}
	return
}

func (c *Conn) cur_window() (window uint32) {
	for _, s := range c.unackedSends {
		select {
		case <-s.acked:
		default:
			window += s.payloadSize
		}
	}
	return
}

func (c *Conn) sendState() {
	c.write(stState, c.send_id, nil, c.seq_nr)
}

func seqLess(a, b uint16) bool {
	if b < 0x8000 {
		return a < b || a >= b-0x8000
	} else {
		return a < b && a >= b-0x8000
	}
}

// Ack our send with the given sequence number.
func (c *Conn) ack(nr uint16) {
	if !seqLess(c.lastAck, nr) {
		// Already acked.
		return
	}
	i := nr - c.lastAck - 1
	if int(i) >= len(c.unackedSends) {
		log.Printf("got ack ahead of syn (%x > %x)", nr, c.seq_nr-1)
		return
	}
	c.unackedSends[i].Ack()
	for {
		if len(c.unackedSends) == 0 {
			break
		}
		select {
		case <-c.unackedSends[0].acked:
		default:
			// Can't trim unacked sends any further.
			return
		}
		// Trim the front of the unacked sends.
		c.unackedSends = c.unackedSends[1:]
		c.lastAck++
	}
	c.event.Broadcast()
}

func (c *Conn) ackTo(nr uint16) {
	if !seqLess(nr, c.seq_nr) {
		return
	}
	for seqLess(c.lastAck, nr) {
		c.ack(c.lastAck + 1)
	}
}

type selectiveAckBitmask []byte

func (me selectiveAckBitmask) NumBits() int {
	return len(me) * 8
}

func (me selectiveAckBitmask) SetBit(index int) {
	me[index/8] |= 1 << uint(index%8)
}

func (me selectiveAckBitmask) BitIsSet(index int) bool {
	return me[index/8]>>uint(index%8)&1 == 1
}

// Return the send state for the sequence number. Returns nil if there's no
// outstanding send for that sequence number.
func (c *Conn) seqSend(seqNr uint16) *send {
	if !seqLess(c.lastAck, seqNr) {
		// Presumably already acked.
		return nil
	}
	i := int(seqNr - c.lastAck - 1)
	if i >= len(c.unackedSends) {
		// No such send.
		return nil
	}
	return c.unackedSends[i]
}

func (c *Conn) ackSkipped(seqNr uint16) {
	send := c.seqSend(seqNr)
	if send == nil {
		return
	}
	send.mu.Lock()
	defer send.mu.Unlock()
	send.acksSkipped++
	switch send.acksSkipped {
	case 3, 60:
		ackSkippedResends.Add(1)
		go send.resend()
		send.resendTimer.Reset(send.resendTimeout())
	default:
	}
}

func (c *Conn) deliver(h header, payload []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer c.event.Broadcast()
	if h.Type == stSyn {
		if h.ConnID != c.send_id {
			panic(fmt.Sprintf("%d != %d", h.ConnID, c.send_id))
		}
	} else {
		if h.ConnID != c.recv_id {
			panic("erroneous delivery")
		}
	}
	c.peerWndSize = h.WndSize
	c.ackTo(h.AckNr)
	for _, ext := range h.Extensions {
		switch ext.Type {
		case extensionTypeSelectiveAck:
			c.ackSkipped(h.AckNr + 1)
			bitmask := selectiveAckBitmask(ext.Bytes)
			for i := 0; i < bitmask.NumBits(); i++ {
				if bitmask.BitIsSet(i) {
					nr := h.AckNr + 2 + uint16(i)
					// log.Printf("selectively acked %d", nr)
					c.ack(nr)
				} else {
					c.ackSkipped(h.AckNr + 2 + uint16(i))
				}
			}
		}
	}
	if h.Timestamp == 0 {
		c.lastTimeDiff = 0
	} else {
		c.lastTimeDiff = c.timestamp() - h.Timestamp
	}
	// log.Printf("now micros: %d, header timestamp: %d, header diff: %d", c.timestamp(), h.Timestamp, h.TimestampDiff)
	if h.Type == stReset {
		c.destroy(errors.New("peer reset"))
		return
	}
	if c.cs == csSynSent {
		if h.Type != stState {
			return
		}
		c.cs = csConnected
		c.ack_nr = h.SeqNr - 1
		return
	}
	if h.Type == stState {
		return
	}
	if !seqLess(c.ack_nr, h.SeqNr) {
		if h.Type == stSyn {
			c.sendState()
		}
		// Already received this packet.
		return
	}
	inboundIndex := int(h.SeqNr - c.ack_nr - 1)
	if inboundIndex < len(c.inbound) && c.inbound[inboundIndex].seen {
		// Already received this packet.
		return
	}
	// Derived from running in production:
	// grep -oP '(?<=packet out of order, index=)\d+' log | sort -n | uniq -c
	// 64 should correspond to 8 bytes of selective ack.
	if inboundIndex >= maxUnackedInbound {
		// Discard packet too far ahead.
		if missinggo.CryHeard() {
			// I can't tell if this occurs due to bad peers, or something
			// missing in the implementation.
			log.Printf("received packet from %s %d ahead of next seqnr (%x > %x)", c.remoteAddr, inboundIndex, h.SeqNr, c.ack_nr+1)
		}
		return
	}
	// Extend inbound so the new packet has a place.
	for inboundIndex >= len(c.inbound) {
		c.inbound = append(c.inbound, recv{})
	}
	if inboundIndex != 0 {
		// log.Printf("packet out of order, index=%d", inboundIndex)
	}
	c.inbound[inboundIndex] = recv{true, payload}
	// Consume consecutive next packets.
	for len(c.inbound) > 0 && c.inbound[0].seen {
		c.ack_nr++
		c.readBuf = append(c.readBuf, c.inbound[0].data...)
		c.inbound = c.inbound[1:]
	}
	c.sendState()
	if c.cs == csSentFin {
		if !seqLess(h.AckNr, c.seq_nr-1) {
			c.destroy(nil)
		}
	}
	if h.Type == stFin {
		c.destroy(nil)
	}
}

func (c *Conn) waitAck(seq uint16) {
	send := c.seqSend(seq)
	if send == nil {
		return
	}
	c.mu.Unlock()
	defer c.mu.Lock()
	<-send.acked
	return
}

func (c *Conn) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq_nr = 1
	_, err := c.write(stSyn, c.recv_id, nil, c.seq_nr)
	if err != nil {
		return err
	}
	c.cs = csSynSent
	if logLevel >= 2 {
		log.Printf("sent syn")
	}
	c.seq_nr++
	c.waitAck(1)
	if c.err != nil {
		err = c.err
	}
	c.event.Broadcast()
	return err
}

// Returns true if the connection was newly registered, false otherwise.
func (s *Socket) registerConn(recvID uint16, remoteAddr resolvedAddrStr, c *Conn) bool {
	if s.conns == nil {
		s.conns = make(map[connKey]*Conn)
	}
	key := connKey{remoteAddr, recvID}
	if _, ok := s.conns[key]; ok {
		return false
	}
	s.conns[key] = c
	c.detach = func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.conns[key] != c {
			panic("conn changed")
		}
		delete(s.conns, key)
		s.event.Broadcast()
	}
	return true
}

func (s *Socket) nextSyn() (syn syn, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		for k := range s.backlog {
			syn = k
			delete(s.backlog, k)
			ok = true
			return
		}
		select {
		case <-s.closing:
			return
		default:
		}
		s.event.Wait()
	}
}

// Accept and return a new uTP connection.
func (s *Socket) Accept() (c net.Conn, err error) {
	for {
		syn, ok := s.nextSyn()
		if !ok {
			err = errClosed
			return
		}
		s.mu.Lock()
		_c := s.newConn(stringAddr(syn.addr))
		_c.send_id = syn.conn_id
		_c.recv_id = _c.send_id + 1
		_c.seq_nr = uint16(rand.Int())
		_c.lastAck = _c.seq_nr - 1
		_c.ack_nr = syn.seq_nr
		_c.cs = csConnected
		if !s.registerConn(_c.recv_id, resolvedAddrStr(syn.addr), _c) {
			// SYN that triggered this accept duplicates existing connection.
			// Ack again in case the SYN was a resend.
			_c = s.conns[connKey{resolvedAddrStr(syn.addr), _c.recv_id}]
			if _c.send_id != syn.conn_id {
				panic(":|")
			}
			_c.sendState()
			s.mu.Unlock()
			continue
		}
		_c.sendState()
		// _c.seq_nr++
		c = _c
		s.mu.Unlock()
		return
	}
}

// The address we're listening on for new uTP connections.
func (s *Socket) Addr() net.Addr {
	return s.pc.LocalAddr()
}

// Marks the Socket for close. Currently this just axes the underlying OS
// socket.
func (s *Socket) Close() (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.closing:
		return
	default:
	}
	close(s.closing)
	s.raw.Close()
	s.pc.Close()
	s.event.Broadcast()
	return
}

// TODO: Currently does nothing. Should probably "close" the packet connection
// to adher to the net.PacketConn protocol.
func (me *packetConn) Close() (err error) {
	me.mu.Lock()
	defer me.mu.Unlock()
	if me.closed {
		return
	}
	close(me.unusedReads)
	me.closed = true
	return
}

func (s *packetConn) LocalAddr() net.Addr {
	return s.real.LocalAddr()
}

func (s *packetConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	read, ok := <-s.unusedReads
	if !ok {
		err = io.EOF
	}
	n = copy(p, read.data)
	addr = read.from
	return
}

func (s *packetConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return s.real.WriteTo(b, addr)
}

func (c *Conn) finish() {
	if c.cs != csConnected {
		return
	}
	finSeqNr := c.seq_nr
	if _, err := c.write(stFin, c.send_id, nil, finSeqNr); err != nil {
		c.destroy(fmt.Errorf("error sending FIN: %s", err))
		return
	}
	c.seq_nr++ // Spec says set to "eof_pkt".
	c.cs = csSentFin
	go func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.waitAck(finSeqNr)
		c.destroy(nil)
	}()
}

func (c *Conn) destroy(reason error) {
	if c.err != nil && reason != nil {
		log.Printf("duplicate destroy call: %s", reason)
	}
	if c.cs == csDestroy {
		return
	}
	c.cs = csDestroy
	c.err = reason
	c.event.Broadcast()
	c.detach()
	for _, s := range c.unackedSends {
		s.Ack()
	}
}

func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.finish()
	return nil
}

func (c *Conn) LocalAddr() net.Addr {
	return c.socket.LocalAddr()
}

func (c *Conn) Read(b []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for {
		if len(c.readBuf) != 0 {
			break
		}
		if c.cs == csDestroy || c.cs == csGotFin {
			err = c.err
			if err == nil {
				err = io.EOF
			}
			return
		}
		if c.connDeadlines.read.deadlineExceeded() {
			err = errTimeout
			return
		}
		if logLevel >= 2 {
			log.Printf("nothing to read, state=%d", c.cs)
		}
		c.event.Wait()
	}
	// log.Printf("read some data!")
	n = copy(b, c.readBuf)
	c.readBuf = c.readBuf[n:]

	return
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *Conn) String() string {
	return fmt.Sprintf("<UTPConn %s-%s>", c.LocalAddr(), c.RemoteAddr())
}

func (c *Conn) Write(p []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(p) != 0 {
		if c.cs != csConnected {
			err = io.ErrClosedPipe
			return
		}
		for {
			// If we're not in a connected state, we let .write() give an
			// appropriate error, below.
			if c.cs != csConnected {
				break
			}
			// If peerWndSize is 0, we still want to send something, so don't
			// block until we exceed it.
			if c.cur_window() <= c.peerWndSize && len(c.unackedSends) < 64 {
				break
			}
			if c.connDeadlines.write.deadlineExceeded() {
				err = errTimeout
				return
			}
			c.event.Wait()
		}
		var n1 int
		n1, err = c.write(stData, c.send_id, p, c.seq_nr)
		if err != nil {
			return
		}
		c.seq_nr++
		n += n1
		p = p[n1:]
	}
	return
}
