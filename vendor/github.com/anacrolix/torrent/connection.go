package torrent

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent/mse"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/bitmap"
	"github.com/anacrolix/missinggo/iter"
	"github.com/anacrolix/missinggo/prioritybitmap"

	"github.com/anacrolix/torrent/bencode"
	pp "github.com/anacrolix/torrent/peer_protocol"
)

type peerSource string

const (
	peerSourceTracker         = "T" // It's the default.
	peerSourceIncoming        = "I"
	peerSourceDHTGetPeers     = "Hg"
	peerSourceDHTAnnouncePeer = "Ha"
	peerSourcePEX             = "X"
)

// Maintains the state of a connection with a peer.
type connection struct {
	t *Torrent
	// The actual Conn, used for closing, and setting socket options.
	conn net.Conn
	// The Reader and Writer for this Conn, with hooks installed for stats,
	// limiting, deadlines etc.
	w io.Writer
	r io.Reader
	// True if the connection is operating over MSE obfuscation.
	headerEncrypted bool
	cryptoMethod    uint32
	Discovery       peerSource
	uTP             bool
	closed          missinggo.Event

	stats                  ConnStats
	UnwantedChunksReceived int
	UsefulChunksReceived   int
	chunksSent             int
	goodPiecesDirtied      int
	badPiecesDirtied       int

	lastMessageReceived     time.Time
	completedHandshake      time.Time
	lastUsefulChunkReceived time.Time
	lastChunkSent           time.Time

	// Stuff controlled by the local peer.
	Interested       bool
	Choked           bool
	requests         map[request]struct{}
	requestsLowWater int
	// Indexed by metadata piece, set to true if posted and pending a
	// response.
	metadataRequests []bool
	sentHaves        []bool

	// Stuff controlled by the remote peer.
	PeerID             [20]byte
	PeerInterested     bool
	PeerChoked         bool
	PeerRequests       map[request]struct{}
	PeerExtensionBytes peerExtensionBytes
	// The pieces the peer has claimed to have.
	peerPieces bitmap.Bitmap
	// The peer has everything. This can occur due to a special message, when
	// we may not even know the number of pieces in the torrent yet.
	peerHasAll bool
	// The highest possible number of pieces the torrent could have based on
	// communication with the peer. Generally only useful until we have the
	// torrent info.
	peerMinPieces int
	// Pieces we've accepted chunks for from the peer.
	peerTouchedPieces map[int]struct{}

	PeerMaxRequests  int // Maximum pending requests the peer allows.
	PeerExtensionIDs map[string]byte
	PeerClientName   string

	pieceInclination  []int
	pieceRequestOrder prioritybitmap.PriorityBitmap

	postedBuffer bytes.Buffer
	uploadTimer  *time.Timer
	writerCond   sync.Cond
}

func (cn *connection) mu() sync.Locker {
	return &cn.t.cl.mu
}

func (cn *connection) remoteAddr() net.Addr {
	return cn.conn.RemoteAddr()
}

func (cn *connection) localAddr() net.Addr {
	return cn.conn.LocalAddr()
}

func (cn *connection) supportsExtension(ext string) bool {
	_, ok := cn.PeerExtensionIDs[ext]
	return ok
}

// The best guess at number of pieces in the torrent for this peer.
func (cn *connection) bestPeerNumPieces() int {
	if cn.t.haveInfo() {
		return cn.t.numPieces()
	}
	return cn.peerMinPieces
}

func (cn *connection) completedString() string {
	return fmt.Sprintf("%d/%d", cn.peerPieces.Len(), cn.bestPeerNumPieces())
}

// Correct the PeerPieces slice length. Return false if the existing slice is
// invalid, such as by receiving badly sized BITFIELD, or invalid HAVE
// messages.
func (cn *connection) setNumPieces(num int) error {
	cn.peerPieces.RemoveRange(num, -1)
	cn.peerPiecesChanged()
	return nil
}

func eventAgeString(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return fmt.Sprintf("%.2fs ago", time.Now().Sub(t).Seconds())
}

func (cn *connection) connectionFlags() (ret string) {
	c := func(b byte) {
		ret += string([]byte{b})
	}
	if cn.cryptoMethod == mse.CryptoMethodRC4 {
		c('E')
	} else if cn.headerEncrypted {
		c('e')
	}
	ret += string(cn.Discovery)
	if cn.uTP {
		c('T')
	}
	return
}

// Inspired by https://trac.transmissionbt.com/wiki/PeerStatusText
func (cn *connection) statusFlags() (ret string) {
	c := func(b byte) {
		ret += string([]byte{b})
	}
	if cn.Interested {
		c('i')
	}
	if cn.Choked {
		c('c')
	}
	c('-')
	ret += cn.connectionFlags()
	c('-')
	if cn.PeerInterested {
		c('i')
	}
	if cn.PeerChoked {
		c('c')
	}
	return
}

func (cn *connection) String() string {
	var buf bytes.Buffer
	cn.WriteStatus(&buf, nil)
	return buf.String()
}

func (cn *connection) WriteStatus(w io.Writer, t *Torrent) {
	// \t isn't preserved in <pre> blocks?
	fmt.Fprintf(w, "%+q: %s-%s\n", cn.PeerID, cn.localAddr(), cn.remoteAddr())
	fmt.Fprintf(w, "    last msg: %s, connected: %s, last useful chunk: %s\n",
		eventAgeString(cn.lastMessageReceived),
		eventAgeString(cn.completedHandshake),
		eventAgeString(cn.lastUsefulChunkReceived))
	fmt.Fprintf(w,
		"    %s completed, %d pieces touched, good chunks: %d/%d-%d reqq: %d-%d, flags: %s\n",
		cn.completedString(),
		len(cn.peerTouchedPieces),
		cn.UsefulChunksReceived,
		cn.UnwantedChunksReceived+cn.UsefulChunksReceived,
		cn.chunksSent,
		cn.numLocalRequests(),
		len(cn.PeerRequests),
		cn.statusFlags(),
	)
	roi := cn.pieceRequestOrderIter()
	fmt.Fprintf(w, "    next pieces: %v%s\n",
		iter.ToSlice(iter.Head(10, roi)),
		func() string {
			if cn.shouldRequestWithoutBias() {
				return " (fastest)"
			} else {
				return ""
			}
		}())
}

func (cn *connection) Close() {
	if !cn.closed.Set() {
		return
	}
	cn.discardPieceInclination()
	cn.pieceRequestOrder.Clear()
	if cn.conn != nil {
		go func() {
			// TODO: This call blocks sometimes, why? Maybe it was the Go utp
			// implementation?
			err := cn.conn.Close()
			if err != nil {
				log.Printf("error closing connection net.Conn: %s", err)
			}
		}()
	}
}

func (cn *connection) PeerHasPiece(piece int) bool {
	return cn.peerHasAll || cn.peerPieces.Contains(piece)
}

func (cn *connection) Post(msg pp.Message) {
	messageTypesPosted.Add(strconv.FormatInt(int64(msg.Type), 10), 1)
	cn.postedBuffer.Write(msg.MustMarshalBinary())
	cn.tickleWriter()
}

func (cn *connection) requestMetadataPiece(index int) {
	eID := cn.PeerExtensionIDs["ut_metadata"]
	if eID == 0 {
		return
	}
	if index < len(cn.metadataRequests) && cn.metadataRequests[index] {
		return
	}
	cn.Post(pp.Message{
		Type:       pp.Extended,
		ExtendedID: eID,
		ExtendedPayload: func() []byte {
			b, err := bencode.Marshal(map[string]int{
				"msg_type": pp.RequestMetadataExtensionMsgType,
				"piece":    index,
			})
			if err != nil {
				panic(err)
			}
			return b
		}(),
	})
	for index >= len(cn.metadataRequests) {
		cn.metadataRequests = append(cn.metadataRequests, false)
	}
	cn.metadataRequests[index] = true
}

func (cn *connection) requestedMetadataPiece(index int) bool {
	return index < len(cn.metadataRequests) && cn.metadataRequests[index]
}

// The actual value to use as the maximum outbound requests.
func (cn *connection) nominalMaxRequests() (ret int) {
	ret = cn.PeerMaxRequests
	if ret > 64 {
		ret = 64
	}
	return
}

// Returns true if an unsatisfied request was canceled.
func (cn *connection) PeerCancel(r request) bool {
	if cn.PeerRequests == nil {
		return false
	}
	if _, ok := cn.PeerRequests[r]; !ok {
		return false
	}
	delete(cn.PeerRequests, r)
	return true
}

func (cn *connection) Choke(msg func(pp.Message) bool) bool {
	if cn.Choked {
		return true
	}
	cn.PeerRequests = nil
	cn.Choked = true
	return msg(pp.Message{
		Type: pp.Choke,
	})
}

func (cn *connection) Unchoke(msg func(pp.Message) bool) bool {
	if !cn.Choked {
		return true
	}
	cn.Choked = false
	return msg(pp.Message{
		Type: pp.Unchoke,
	})
}

func (cn *connection) SetInterested(interested bool, msg func(pp.Message) bool) bool {
	if cn.Interested == interested {
		return true
	}
	cn.Interested = interested
	// log.Printf("%p: setting interest: %v", cn, interested)
	return msg(pp.Message{
		Type: func() pp.MessageType {
			if interested {
				return pp.Interested
			} else {
				return pp.NotInterested
			}
		}(),
	})
}

func (cn *connection) fillWriteBuffer(msg func(pp.Message) bool) {
	numFillBuffers.Add(1)
	cancel, new, i := cn.desiredRequestState()
	if !cn.SetInterested(i, msg) {
		return
	}
	if cancel && len(cn.requests) != 0 {
		fillBufferSentCancels.Add(1)
		for r := range cn.requests {
			cn.deleteRequest(r)
			// log.Printf("%p: cancelling request: %v", cn, r)
			if !msg(makeCancelMessage(r)) {
				return
			}
		}
	}
	if len(new) != 0 {
		fillBufferSentRequests.Add(1)
		for _, r := range new {
			if cn.requests == nil {
				cn.requests = make(map[request]struct{}, cn.nominalMaxRequests())
			}
			cn.requests[r] = struct{}{}
			// log.Printf("%p: requesting %v", cn, r)
			if !msg(pp.Message{
				Type:   pp.Request,
				Index:  r.Index,
				Begin:  r.Begin,
				Length: r.Length,
			}) {
				return
			}
		}
		// If we didn't completely top up the requests, we shouldn't mark the
		// low water, since we'll want to top up the requests as soon as we
		// have more write buffer space.
		cn.requestsLowWater = len(cn.requests) / 2
	}
	cn.upload(msg)
}

// Writes buffers to the socket from the write channel.
func (cn *connection) writer(keepAliveTimeout time.Duration) {
	var (
		buf       bytes.Buffer
		lastWrite time.Time = time.Now()
	)
	var keepAliveTimer *time.Timer
	keepAliveTimer = time.AfterFunc(keepAliveTimeout, func() {
		cn.mu().Lock()
		defer cn.mu().Unlock()
		if time.Since(lastWrite) >= keepAliveTimeout {
			cn.tickleWriter()
		}
		keepAliveTimer.Reset(keepAliveTimeout)
	})
	cn.mu().Lock()
	defer cn.mu().Unlock()
	defer cn.Close()
	defer keepAliveTimer.Stop()
	for {
		buf.Write(cn.postedBuffer.Bytes())
		cn.postedBuffer.Reset()
		if buf.Len() == 0 {
			cn.fillWriteBuffer(func(msg pp.Message) bool {
				cn.wroteMsg(&msg)
				buf.Write(msg.MustMarshalBinary())
				return buf.Len() < 1<<16
			})
		}
		if buf.Len() == 0 && time.Since(lastWrite) >= keepAliveTimeout {
			buf.Write(pp.Message{Keepalive: true}.MustMarshalBinary())
			postedKeepalives.Add(1)
		}
		if buf.Len() == 0 {
			cn.writerCond.Wait()
			continue
		}
		cn.mu().Unlock()
		// log.Printf("writing %d bytes", buf.Len())
		n, err := cn.w.Write(buf.Bytes())
		cn.mu().Lock()
		if n != 0 {
			lastWrite = time.Now()
			keepAliveTimer.Reset(keepAliveTimeout)
		}
		if err != nil {
			return
		}
		if n != buf.Len() {
			panic("short write")
		}
		buf.Reset()
	}
}

func (cn *connection) Have(piece int) {
	for piece >= len(cn.sentHaves) {
		cn.sentHaves = append(cn.sentHaves, false)
	}
	if cn.sentHaves[piece] {
		return
	}
	cn.Post(pp.Message{
		Type:  pp.Have,
		Index: pp.Integer(piece),
	})
	cn.sentHaves[piece] = true
}

func (cn *connection) Bitfield(haves []bool) {
	if cn.sentHaves != nil {
		panic("bitfield must be first have-related message sent")
	}
	cn.Post(pp.Message{
		Type:     pp.Bitfield,
		Bitfield: haves,
	})
	// Make a copy of haves, as that's read when the message is marshalled
	// without the lock. Also it obviously shouldn't change in the Msg due to
	// changes in .sentHaves.
	cn.sentHaves = append([]bool(nil), haves...)
}

func nextRequestState(
	networkingEnabled bool,
	currentRequests map[request]struct{},
	peerChoking bool,
	requestPieces iter.Func,
	pendingChunks func(piece int, f func(chunkSpec) bool) bool,
	requestsLowWater int,
	requestsHighWater int,
) (
	cancelExisting bool, // Cancel all our pending requests
	newRequests []request, // Chunks to request that we currently aren't
	interested bool, // Whether we should indicate interest, even if we don't request anything
) {
	if !networkingEnabled {
		return true, nil, false
	}
	if len(currentRequests) > requestsLowWater {
		return false, nil, true
	}
	requestPieces(func(_piece interface{}) bool {
		interested = true
		if peerChoking {
			return false
		}
		piece := _piece.(int)
		return pendingChunks(piece, func(cs chunkSpec) bool {
			r := request{pp.Integer(piece), cs}
			if _, ok := currentRequests[r]; !ok {
				if newRequests == nil {
					newRequests = make([]request, 0, requestsHighWater-len(currentRequests))
				}
				newRequests = append(newRequests, r)
			}
			return len(currentRequests)+len(newRequests) < requestsHighWater
		})
	})
	return
}

func (cn *connection) updateRequests() {
	cn.tickleWriter()
}

func iterBitmapsDistinct(skip bitmap.Bitmap, bms ...bitmap.Bitmap) iter.Func {
	return func(cb iter.Callback) {
		for _, bm := range bms {
			if !iter.All(func(i interface{}) bool {
				skip.Add(i.(int))
				return cb(i)
			}, bitmap.Sub(bm, skip).Iter) {
				return
			}
		}
	}
}

func (cn *connection) unbiasedPieceRequestOrder() iter.Func {
	now, readahead := cn.t.readerPiecePriorities()
	return iterBitmapsDistinct(cn.t.completedPieces.Copy(), now, readahead, cn.t.pendingPieces)
}

// The connection should download highest priority pieces first, without any
// inclination toward avoiding wastage. Generally we might do this if there's
// a single connection, or this is the fastest connection, and we have active
// readers that signal an ordering preference. It's conceivable that the best
// connection should do this, since it's least likely to waste our time if
// assigned to the highest priority pieces, and assigning more than one this
// role would cause significant wasted bandwidth.
func (cn *connection) shouldRequestWithoutBias() bool {
	if cn.t.requestStrategy != 2 {
		return false
	}
	if len(cn.t.readers) == 0 {
		return false
	}
	if len(cn.t.conns) == 1 {
		return true
	}
	if cn == cn.t.fastestConn {
		return true
	}
	return false
}

func (cn *connection) pieceRequestOrderIter() iter.Func {
	if cn.shouldRequestWithoutBias() {
		return cn.unbiasedPieceRequestOrder()
	} else {
		return cn.pieceRequestOrder.Iter
	}
}

func (cn *connection) desiredRequestState() (bool, []request, bool) {
	return nextRequestState(
		cn.t.networkingEnabled,
		cn.requests,
		cn.PeerChoked,
		cn.pieceRequestOrderIter(),
		func(piece int, f func(chunkSpec) bool) bool {
			return undirtiedChunks(piece, cn.t, f)
		},
		cn.requestsLowWater,
		cn.nominalMaxRequests(),
	)
}

func undirtiedChunks(piece int, t *Torrent, f func(chunkSpec) bool) bool {
	chunkIndices := t.pieces[piece].undirtiedChunkIndices().ToSortedSlice()
	return iter.ForPerm(len(chunkIndices), func(i int) bool {
		return f(t.chunkIndexSpec(chunkIndices[i], piece))
	})
}

// check callers updaterequests
func (cn *connection) stopRequestingPiece(piece int) bool {
	return cn.pieceRequestOrder.Remove(piece)
}

// This is distinct from Torrent piece priority, which is the user's
// preference. Connection piece priority is specific to a connection,
// pseudorandomly avoids connections always requesting the same pieces and
// thus wasting effort.
func (cn *connection) updatePiecePriority(piece int) bool {
	tpp := cn.t.piecePriority(piece)
	if !cn.PeerHasPiece(piece) {
		tpp = PiecePriorityNone
	}
	if tpp == PiecePriorityNone {
		return cn.stopRequestingPiece(piece)
	}
	prio := cn.getPieceInclination()[piece]
	switch cn.t.requestStrategy {
	case 1:
		switch tpp {
		case PiecePriorityNormal:
		case PiecePriorityReadahead:
			prio -= cn.t.numPieces()
		case PiecePriorityNext, PiecePriorityNow:
			prio -= 2 * cn.t.numPieces()
		default:
			panic(tpp)
		}
		prio += piece / 3
	default:
	}
	return cn.pieceRequestOrder.Set(piece, prio)
}

func (cn *connection) getPieceInclination() []int {
	if cn.pieceInclination == nil {
		cn.pieceInclination = cn.t.getConnPieceInclination()
	}
	return cn.pieceInclination
}

func (cn *connection) discardPieceInclination() {
	if cn.pieceInclination == nil {
		return
	}
	cn.t.putPieceInclination(cn.pieceInclination)
	cn.pieceInclination = nil
}

func (cn *connection) peerPiecesChanged() {
	if cn.t.haveInfo() {
		prioritiesChanged := false
		for i := range iter.N(cn.t.numPieces()) {
			if cn.updatePiecePriority(i) {
				prioritiesChanged = true
			}
		}
		if prioritiesChanged {
			cn.updateRequests()
		}
	}
}

func (cn *connection) raisePeerMinPieces(newMin int) {
	if newMin > cn.peerMinPieces {
		cn.peerMinPieces = newMin
	}
}

func (cn *connection) peerSentHave(piece int) error {
	if cn.t.haveInfo() && piece >= cn.t.numPieces() {
		return errors.New("invalid piece")
	}
	if cn.PeerHasPiece(piece) {
		return nil
	}
	cn.raisePeerMinPieces(piece + 1)
	cn.peerPieces.Set(piece, true)
	if cn.updatePiecePriority(piece) {
		cn.updateRequests()
	}
	return nil
}

func (cn *connection) peerSentBitfield(bf []bool) error {
	cn.peerHasAll = false
	if len(bf)%8 != 0 {
		panic("expected bitfield length divisible by 8")
	}
	// We know that the last byte means that at most the last 7 bits are
	// wasted.
	cn.raisePeerMinPieces(len(bf) - 7)
	if cn.t.haveInfo() && len(bf) > cn.t.numPieces() {
		// Ignore known excess pieces.
		bf = bf[:cn.t.numPieces()]
	}
	for i, have := range bf {
		if have {
			cn.raisePeerMinPieces(i + 1)
		}
		cn.peerPieces.Set(i, have)
	}
	cn.peerPiecesChanged()
	return nil
}

func (cn *connection) peerSentHaveAll() error {
	cn.peerHasAll = true
	cn.peerPieces.Clear()
	cn.peerPiecesChanged()
	return nil
}

func (cn *connection) peerSentHaveNone() error {
	cn.peerPieces.Clear()
	cn.peerHasAll = false
	cn.peerPiecesChanged()
	return nil
}

func (c *connection) requestPendingMetadata() {
	if c.t.haveInfo() {
		return
	}
	if c.PeerExtensionIDs["ut_metadata"] == 0 {
		// Peer doesn't support this.
		return
	}
	// Request metadata pieces that we don't have in a random order.
	var pending []int
	for index := 0; index < c.t.metadataPieceCount(); index++ {
		if !c.t.haveMetadataPiece(index) && !c.requestedMetadataPiece(index) {
			pending = append(pending, index)
		}
	}
	for _, i := range rand.Perm(len(pending)) {
		c.requestMetadataPiece(pending[i])
	}
}

func (cn *connection) wroteMsg(msg *pp.Message) {
	messageTypesSent.Add(strconv.FormatInt(int64(msg.Type), 10), 1)
	cn.stats.wroteMsg(msg)
	cn.t.stats.wroteMsg(msg)
}

func (cn *connection) readMsg(msg *pp.Message) {
	cn.stats.readMsg(msg)
	cn.t.stats.readMsg(msg)
}

func (cn *connection) wroteBytes(n int64) {
	cn.stats.wroteBytes(n)
	if cn.t != nil {
		cn.t.stats.wroteBytes(n)
	}
}

func (cn *connection) readBytes(n int64) {
	cn.stats.readBytes(n)
	if cn.t != nil {
		cn.t.stats.readBytes(n)
	}
}

// Returns whether the connection is currently useful to us. We're seeding and
// they want data, we don't have metainfo and they can provide it, etc.
func (c *connection) useful() bool {
	t := c.t
	if c.closed.IsSet() {
		return false
	}
	if !t.haveInfo() {
		return c.supportsExtension("ut_metadata")
	}
	if t.seeding() {
		return c.PeerInterested
	}
	return c.peerHasWantedPieces()
}

func (c *connection) lastHelpful() (ret time.Time) {
	ret = c.lastUsefulChunkReceived
	if c.t.seeding() && c.lastChunkSent.After(ret) {
		ret = c.lastChunkSent
	}
	return
}

// Processes incoming bittorrent messages. The client lock is held upon entry
// and exit. Returning will end the connection.
func (c *connection) mainReadLoop() error {
	t := c.t
	cl := t.cl

	decoder := pp.Decoder{
		R:         bufio.NewReaderSize(c.r, 1<<17),
		MaxLength: 256 * 1024,
		Pool:      t.chunkPool,
	}
	for {
		var (
			msg pp.Message
			err error
		)
		func() {
			cl.mu.Unlock()
			defer cl.mu.Lock()
			err = decoder.Decode(&msg)
		}()
		if cl.closed.IsSet() || c.closed.IsSet() || err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		c.readMsg(&msg)
		c.lastMessageReceived = time.Now()
		if msg.Keepalive {
			receivedKeepalives.Add(1)
			continue
		}
		messageTypesReceived.Add(strconv.FormatInt(int64(msg.Type), 10), 1)
		switch msg.Type {
		case pp.Choke:
			c.PeerChoked = true
			c.requests = nil
			// We can then reset our interest.
			c.updateRequests()
		case pp.Reject:
			if c.deleteRequest(newRequest(msg.Index, msg.Begin, msg.Length)) {
				c.updateRequests()
			}
		case pp.Unchoke:
			c.PeerChoked = false
			c.tickleWriter()
		case pp.Interested:
			c.PeerInterested = true
			c.tickleWriter()
		case pp.NotInterested:
			c.PeerInterested = false
			c.PeerRequests = nil
		case pp.Have:
			err = c.peerSentHave(int(msg.Index))
		case pp.Request:
			if c.Choked {
				break
			}
			if len(c.PeerRequests) >= maxRequests {
				break
			}
			if c.PeerRequests == nil {
				c.PeerRequests = make(map[request]struct{}, maxRequests)
			}
			c.PeerRequests[newRequest(msg.Index, msg.Begin, msg.Length)] = struct{}{}
			c.tickleWriter()
		case pp.Cancel:
			req := newRequest(msg.Index, msg.Begin, msg.Length)
			if !c.PeerCancel(req) {
				unexpectedCancels.Add(1)
			}
		case pp.Bitfield:
			err = c.peerSentBitfield(msg.Bitfield)
		case pp.HaveAll:
			err = c.peerSentHaveAll()
		case pp.HaveNone:
			err = c.peerSentHaveNone()
		case pp.Piece:
			c.receiveChunk(&msg)
			if len(msg.Piece) == int(t.chunkSize) {
				t.chunkPool.Put(msg.Piece)
			}
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
						err = t.setMetadataSize(metadata_size)
						if err != nil {
							err = fmt.Errorf("error setting metadata size to %d", metadata_size)
							break
						}
					}
				}
				if _, ok := c.PeerExtensionIDs["ut_metadata"]; ok {
					c.requestPendingMetadata()
				}
			case metadataExtendedId:
				err = cl.gotMetadataExtensionMsg(msg.ExtendedPayload, t, c)
				if err != nil {
					err = fmt.Errorf("error handling metadata extension message: %s", err)
				}
			case pexExtendedId:
				if cl.config.DisablePEX {
					break
				}
				var pexMsg peerExchangeMessage
				err = bencode.Unmarshal(msg.ExtendedPayload, &pexMsg)
				if err != nil {
					err = fmt.Errorf("error unmarshalling PEX message: %s", err)
					break
				}
				go func() {
					cl.mu.Lock()
					t.addPeers(func() (ret []Peer) {
						for i, cp := range pexMsg.Added {
							p := Peer{
								IP:     make([]byte, 4),
								Port:   cp.Port,
								Source: peerSourcePEX,
							}
							if i < len(pexMsg.AddedFlags) && pexMsg.AddedFlags[i]&0x01 != 0 {
								p.SupportsEncryption = true
							}
							missinggo.CopyExact(p.IP, cp.IP[:])
							ret = append(ret, p)
						}
						return
					}())
					cl.mu.Unlock()
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
			if cl.dHT == nil {
				break
			}
			pingAddr, err := net.ResolveUDPAddr("", c.remoteAddr().String())
			if err != nil {
				panic(err)
			}
			if msg.Port != 0 {
				pingAddr.Port = int(msg.Port)
			}
			go cl.dHT.Ping(pingAddr, nil)
		default:
			err = fmt.Errorf("received unknown message type: %#v", msg.Type)
		}
		if err != nil {
			return err
		}
	}
}

// Set both the Reader and Writer for the connection from a single ReadWriter.
func (cn *connection) setRW(rw io.ReadWriter) {
	cn.r = rw
	cn.w = rw
}

// Returns the Reader and Writer as a combined ReadWriter.
func (cn *connection) rw() io.ReadWriter {
	return struct {
		io.Reader
		io.Writer
	}{cn.r, cn.w}
}

// Handle a received chunk from a peer.
func (c *connection) receiveChunk(msg *pp.Message) {
	t := c.t
	cl := t.cl
	chunksReceived.Add(1)

	req := newRequest(msg.Index, msg.Begin, pp.Integer(len(msg.Piece)))

	// Request has been satisfied.
	if c.deleteRequest(req) {
		c.updateRequests()
	} else {
		unexpectedChunksReceived.Add(1)
	}

	// Do we actually want this chunk?
	if !t.wantPiece(req) {
		unwantedChunksReceived.Add(1)
		c.UnwantedChunksReceived++
		return
	}

	index := int(req.Index)
	piece := &t.pieces[index]

	c.UsefulChunksReceived++
	c.lastUsefulChunkReceived = time.Now()
	if t.fastestConn != c {
		// log.Printf("setting fastest connection %p", c)
	}
	t.fastestConn = c

	// Need to record that it hasn't been written yet, before we attempt to do
	// anything with it.
	piece.incrementPendingWrites()
	// Record that we have the chunk.
	piece.unpendChunkIndex(chunkIndex(req.chunkSpec, t.chunkSize))

	// Cancel pending requests for this chunk.
	for c := range t.conns {
		c.postCancel(req)
	}

	cl.mu.Unlock()
	// Write the chunk out. Note that the upper bound on chunk writing
	// concurrency will be the number of connections.
	err := t.writeChunk(int(msg.Index), int64(msg.Begin), msg.Piece)
	cl.mu.Lock()

	piece.decrementPendingWrites()

	if err != nil {
		log.Printf("%s (%x): error writing chunk %v: %s", t, t.infoHash, req, err)
		t.pendRequest(req)
		t.updatePieceCompletion(int(msg.Index))
		return
	}

	// It's important that the piece is potentially queued before we check if
	// the piece is still wanted, because if it is queued, it won't be wanted.
	if t.pieceAllDirty(index) {
		t.queuePieceCheck(int(req.Index))
	}

	if c.peerTouchedPieces == nil {
		c.peerTouchedPieces = make(map[int]struct{})
	}
	c.peerTouchedPieces[index] = struct{}{}

	cl.event.Broadcast()
	t.publishPieceChange(int(req.Index))
	return
}

// Also handles choking and unchoking of the remote peer.
func (c *connection) upload(msg func(pp.Message) bool) bool {
	t := c.t
	cl := t.cl
	if cl.config.NoUpload {
		return true
	}
	if !c.PeerInterested {
		return true
	}
	seeding := t.seeding()
	if !seeding && !c.peerHasWantedPieces() {
		// There's no reason to upload to this peer.
		return true
	}
	// Breaking or completing this loop means we don't want to upload to the
	// peer anymore, and we choke them.
another:
	for seeding || c.chunksSent < c.UsefulChunksReceived+6 {
		// We want to upload to the peer.
		if !c.Unchoke(msg) {
			return false
		}
		for r := range c.PeerRequests {
			res := cl.uploadLimit.ReserveN(time.Now(), int(r.Length))
			if !res.OK() {
				panic(res)
			}
			delay := res.Delay()
			if delay > 0 {
				res.Cancel()
				if c.uploadTimer == nil {
					c.uploadTimer = time.AfterFunc(delay, c.writerCond.Broadcast)
				} else {
					c.uploadTimer.Reset(delay)
				}
				// Hard to say what to return here.
				return true
			}
			more, err := cl.sendChunk(t, c, r, msg)
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
			if !more {
				return false
			}
			goto another
		}
		return true
	}
	return c.Choke(msg)
}

func (cn *connection) Drop() {
	cn.t.dropConnection(cn)
}

func (cn *connection) netGoodPiecesDirtied() int {
	return cn.goodPiecesDirtied - cn.badPiecesDirtied
}

func (c *connection) peerHasWantedPieces() bool {
	return !c.pieceRequestOrder.IsEmpty()
}

func (c *connection) numLocalRequests() int {
	return len(c.requests)
}

func (c *connection) deleteRequest(r request) bool {
	if _, ok := c.requests[r]; !ok {
		return false
	}
	delete(c.requests, r)
	return true
}
func (c *connection) tickleWriter() {
	c.writerCond.Broadcast()
}

func (c *connection) postCancel(r request) bool {
	if !c.deleteRequest(r) {
		return false
	}
	c.Post(makeCancelMessage(r))
	return true
}
