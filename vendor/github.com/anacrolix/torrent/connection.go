package torrent

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/dht"
	"github.com/anacrolix/log"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/bitmap"
	"github.com/anacrolix/missinggo/iter"
	"github.com/anacrolix/missinggo/prioritybitmap"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/mse"
	pp "github.com/anacrolix/torrent/peer_protocol"
)

type peerSource string

const (
	peerSourceTracker         = "Tr"
	peerSourceIncoming        = "I"
	peerSourceDHTGetPeers     = "Hg" // Peers we found by searching a DHT.
	peerSourceDHTAnnouncePeer = "Ha" // Peers that were announced to us by a DHT.
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
	cryptoMethod    mse.CryptoMethod
	Discovery       peerSource
	closed          missinggo.Event

	stats ConnStats

	lastMessageReceived     time.Time
	completedHandshake      time.Time
	lastUsefulChunkReceived time.Time
	lastChunkSent           time.Time

	// Stuff controlled by the local peer.
	Interested           bool
	lastBecameInterested time.Time
	priorInterest        time.Duration

	Choked           bool
	requests         map[request]struct{}
	requestsLowWater int
	// Indexed by metadata piece, set to true if posted and pending a
	// response.
	metadataRequests []bool
	sentHaves        bitmap.Bitmap

	// Stuff controlled by the remote peer.
	PeerID             PeerID
	PeerInterested     bool
	PeerChoked         bool
	PeerRequests       map[request]struct{}
	PeerExtensionBytes peerExtensionBytes
	// The pieces the peer has claimed to have.
	peerPieces bitmap.Bitmap
	// The peer has everything. This can occur due to a special message, when
	// we may not even know the number of pieces in the torrent yet.
	peerSentHaveAll bool
	// The highest possible number of pieces the torrent could have based on
	// communication with the peer. Generally only useful until we have the
	// torrent info.
	peerMinPieces int
	// Pieces we've accepted chunks for from the peer.
	peerTouchedPieces map[int]struct{}
	peerAllowedFast   bitmap.Bitmap

	PeerMaxRequests  int // Maximum pending requests the peer allows.
	PeerExtensionIDs map[string]byte
	PeerClientName   string

	pieceInclination  []int
	pieceRequestOrder prioritybitmap.PriorityBitmap

	writeBuffer *bytes.Buffer
	uploadTimer *time.Timer
	writerCond  sync.Cond
}

func (cn *connection) cumInterest() time.Duration {
	ret := cn.priorInterest
	if cn.Interested {
		ret += time.Since(cn.lastBecameInterested)
	}
	return ret
}

func (cn *connection) peerHasAllPieces() (all bool, known bool) {
	if cn.peerSentHaveAll {
		return true, true
	}
	if !cn.t.haveInfo() {
		return false, false
	}
	return bitmap.Flip(cn.peerPieces, 0, cn.t.numPieces()).IsEmpty(), true
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
	have := cn.peerPieces.Len()
	if cn.peerSentHaveAll {
		have = cn.bestPeerNumPieces()
	}
	return fmt.Sprintf("%d/%d", have, cn.bestPeerNumPieces())
}

// Correct the PeerPieces slice length. Return false if the existing slice is
// invalid, such as by receiving badly sized BITFIELD, or invalid HAVE
// messages.
func (cn *connection) setNumPieces(num int) error {
	cn.peerPieces.RemoveRange(num, bitmap.ToEnd)
	cn.peerPiecesChanged()
	return nil
}

func eventAgeString(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return fmt.Sprintf("%.2fs ago", time.Since(t).Seconds())
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
	if cn.utp() {
		c('U')
	}
	return
}

func (cn *connection) utp() bool {
	return strings.Contains(cn.remoteAddr().Network(), "utp")
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

// func (cn *connection) String() string {
// 	var buf bytes.Buffer
// 	cn.WriteStatus(&buf, nil)
// 	return buf.String()
// }

func (cn *connection) downloadRate() float64 {
	return float64(cn.stats.BytesReadUsefulData) / cn.cumInterest().Seconds()
}

func (cn *connection) WriteStatus(w io.Writer, t *Torrent) {
	// \t isn't preserved in <pre> blocks?
	fmt.Fprintf(w, "%+-55q %s %s-%s\n", cn.PeerID, cn.PeerExtensionBytes, cn.localAddr(), cn.remoteAddr())
	fmt.Fprintf(w, "    last msg: %s, connected: %s, last helpful: %s, itime: %s\n",
		eventAgeString(cn.lastMessageReceived),
		eventAgeString(cn.completedHandshake),
		eventAgeString(cn.lastHelpful()),
		cn.cumInterest(),
	)
	fmt.Fprintf(w,
		"    %s completed, %d pieces touched, good chunks: %d/%d-%d reqq: (%d,%d,%d]-%d, flags: %s, dr: %.1f KiB/s\n",
		cn.completedString(),
		len(cn.peerTouchedPieces),
		cn.stats.ChunksReadUseful,
		cn.stats.ChunksRead,
		cn.stats.ChunksWritten,
		cn.requestsLowWater,
		cn.numLocalRequests(),
		cn.nominalMaxRequests(),
		len(cn.PeerRequests),
		cn.statusFlags(),
		cn.downloadRate()/(1<<10),
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
	cn.tickleWriter()
	cn.discardPieceInclination()
	cn.pieceRequestOrder.Clear()
	if cn.conn != nil {
		go cn.conn.Close()
	}
}

func (cn *connection) PeerHasPiece(piece int) bool {
	return cn.peerSentHaveAll || cn.peerPieces.Contains(piece)
}

// Writes a message into the write buffer.
func (cn *connection) Post(msg pp.Message) {
	messageTypesPosted.Add(msg.Type.String(), 1)
	// We don't need to track bytes here because a connection.w Writer wrapper
	// takes care of that (although there's some delay between us recording
	// the message, and the connection writer flushing it out.).
	cn.writeBuffer.Write(msg.MustMarshalBinary())
	// Last I checked only Piece messages affect stats, and we don't post
	// those.
	cn.wroteMsg(&msg)
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
	return int(clamp(1, int64(cn.PeerMaxRequests), max(64, cn.stats.ChunksReadUseful-(cn.stats.ChunksRead-cn.stats.ChunksReadUseful))))
}

func (cn *connection) onPeerSentCancel(r request) {
	if _, ok := cn.PeerRequests[r]; !ok {
		torrent.Add("unexpected cancels received", 1)
		return
	}
	if cn.fastEnabled() {
		cn.reject(r)
	} else {
		delete(cn.PeerRequests, r)
	}
}

func (cn *connection) Choke(msg messageWriter) (more bool) {
	if cn.Choked {
		return true
	}
	cn.Choked = true
	more = msg(pp.Message{
		Type: pp.Choke,
	})
	if cn.fastEnabled() {
		for r := range cn.PeerRequests {
			// TODO: Don't reject pieces in allowed fast set.
			cn.reject(r)
		}
	} else {
		cn.PeerRequests = nil
	}
	return
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
	if interested {
		cn.lastBecameInterested = time.Now()
	} else if !cn.lastBecameInterested.IsZero() {
		cn.priorInterest += time.Since(cn.lastBecameInterested)
	}
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

// The function takes a message to be sent, and returns true if more messages
// are okay.
type messageWriter func(pp.Message) bool

// Proxies the messageWriter's response.
func (cn *connection) request(r request, mw messageWriter) bool {
	if cn.requests == nil {
		cn.requests = make(map[request]struct{}, cn.nominalMaxRequests())
	}
	if _, ok := cn.requests[r]; ok {
		panic("chunk already requested")
	}
	if !cn.PeerHasPiece(r.Index.Int()) {
		panic("requesting piece peer doesn't have")
	}
	if _, ok := cn.t.conns[cn]; !ok {
		panic("requesting but not in active conns")
	}
	if cn.closed.IsSet() {
		panic("requesting when connection is closed")
	}
	if cn.PeerChoked {
		if cn.peerAllowedFast.Get(int(r.Index)) {
			torrent.Add("allowed fast requests sent", 1)
		} else {
			panic("requesting while choked and not allowed fast")
		}
	}
	cn.requests[r] = struct{}{}
	cn.t.pendingRequests[r]++
	return mw(pp.Message{
		Type:   pp.Request,
		Index:  r.Index,
		Begin:  r.Begin,
		Length: r.Length,
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
			if !cn.request(r, msg) {
				// If we didn't completely top up the requests, we shouldn't
				// mark the low water, since we'll want to top up the requests
				// as soon as we have more write buffer space.
				return
			}
		}
		cn.requestsLowWater = len(cn.requests) / 2
	}
	cn.upload(msg)
}

// Routine that writes to the peer. Some of what to write is buffered by
// activity elsewhere in the Client, and some is determined locally when the
// connection is writable.
func (cn *connection) writer(keepAliveTimeout time.Duration) {
	var (
		lastWrite      time.Time = time.Now()
		keepAliveTimer *time.Timer
	)
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
	frontBuf := new(bytes.Buffer)
	for {
		if cn.closed.IsSet() {
			return
		}
		if cn.writeBuffer.Len() == 0 {
			cn.fillWriteBuffer(func(msg pp.Message) bool {
				cn.wroteMsg(&msg)
				cn.writeBuffer.Write(msg.MustMarshalBinary())
				return cn.writeBuffer.Len() < 1<<16
			})
		}
		if cn.writeBuffer.Len() == 0 && time.Since(lastWrite) >= keepAliveTimeout {
			cn.writeBuffer.Write(pp.Message{Keepalive: true}.MustMarshalBinary())
			postedKeepalives.Add(1)
		}
		if cn.writeBuffer.Len() == 0 {
			// TODO: Minimize wakeups....
			cn.writerCond.Wait()
			continue
		}
		// Flip the buffers.
		frontBuf, cn.writeBuffer = cn.writeBuffer, frontBuf
		cn.mu().Unlock()
		n, err := cn.w.Write(frontBuf.Bytes())
		cn.mu().Lock()
		if n != 0 {
			lastWrite = time.Now()
			keepAliveTimer.Reset(keepAliveTimeout)
		}
		if err != nil {
			return
		}
		if n != frontBuf.Len() {
			panic("short write")
		}
		frontBuf.Reset()
	}
}

func (cn *connection) Have(piece int) {
	if cn.sentHaves.Get(piece) {
		return
	}
	cn.Post(pp.Message{
		Type:  pp.Have,
		Index: pp.Integer(piece),
	})
	cn.sentHaves.Add(piece)
}

func (cn *connection) PostBitfield() {
	if cn.sentHaves.Len() != 0 {
		panic("bitfield must be first have-related message sent")
	}
	if !cn.t.haveAnyPieces() {
		return
	}
	cn.Post(pp.Message{
		Type:     pp.Bitfield,
		Bitfield: cn.t.bitfield(),
	})
	cn.sentHaves = cn.t.completedPieces.Copy()
}

// Determines interest and requests to send to a connected peer.
func nextRequestState(
	networkingEnabled bool,
	currentRequests map[request]struct{},
	peerChoking bool,
	iterPendingRequests func(f func(request) bool),
	requestsLowWater int,
	requestsHighWater int,
	allowedFast bitmap.Bitmap,
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
	iterPendingRequests(func(r request) bool {
		interested = true
		if peerChoking {
			if allowedFast.IsEmpty() {
				return false
			}
			if !allowedFast.Get(int(r.Index)) {
				return true
			}
		}
		if len(currentRequests)+len(newRequests) >= requestsHighWater {
			return false
		}
		if _, ok := currentRequests[r]; !ok {
			if newRequests == nil {
				newRequests = make([]request, 0, requestsHighWater-len(currentRequests))
			}
			newRequests = append(newRequests, r)
		}
		return true
	})
	return
}

func (cn *connection) updateRequests() {
	// log.Print("update requests")
	cn.tickleWriter()
}

// Emits the indices in the Bitmaps bms in order, never repeating any index.
// skip is mutated during execution, and its initial values will never be
// emitted.
func iterBitmapsDistinct(skip *bitmap.Bitmap, bms ...bitmap.Bitmap) iter.Func {
	return func(cb iter.Callback) {
		for _, bm := range bms {
			if !iter.All(func(i interface{}) bool {
				skip.Add(i.(int))
				return cb(i)
			}, bitmap.Sub(bm, *skip).Iter) {
				return
			}
		}
	}
}

func (cn *connection) unbiasedPieceRequestOrder() iter.Func {
	now, readahead := cn.t.readerPiecePriorities()
	var skip bitmap.Bitmap
	if !cn.peerSentHaveAll {
		// Pieces to skip include pieces the peer doesn't have
		skip = bitmap.Flip(cn.peerPieces, 0, cn.t.numPieces())
	}
	// And pieces that we already have.
	skip.Union(cn.t.completedPieces)
	// Return an iterator over the different priority classes, minus the skip
	// pieces.
	return iter.Chain(
		iterBitmapsDistinct(&skip, now, readahead),
		func(cb iter.Callback) {
			cn.t.pendingPieces.IterTyped(func(piece int) bool {
				if skip.Contains(piece) {
					return true
				}
				more := cb(piece)
				skip.Add(piece)
				return more
			})
		},
	)
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

func (cn *connection) iterPendingRequests(f func(request) bool) {
	cn.pieceRequestOrderIter()(func(_piece interface{}) bool {
		piece := _piece.(int)
		return iterUndirtiedChunks(piece, cn.t, func(cs chunkSpec) bool {
			r := request{pp.Integer(piece), cs}
			// log.Println(r, cn.t.pendingRequests[r], cn.requests)
			// if _, ok := cn.requests[r]; !ok && cn.t.pendingRequests[r] != 0 {
			// 	return true
			// }
			return f(r)
		})
	})
}

func (cn *connection) desiredRequestState() (bool, []request, bool) {
	return nextRequestState(
		cn.t.networkingEnabled,
		cn.requests,
		cn.PeerChoked,
		cn.iterPendingRequests,
		cn.requestsLowWater,
		cn.nominalMaxRequests(),
		cn.peerAllowedFast,
	)
}

func iterUndirtiedChunks(piece int, t *Torrent, f func(chunkSpec) bool) bool {
	chunkIndices := t.pieces[piece].undirtiedChunkIndices().ToSortedSlice()
	// TODO: Use "math/rand".Shuffle >= Go 1.10
	return iter.ForPerm(len(chunkIndices), func(i int) bool {
		return f(t.chunkIndexSpec(chunkIndices[i], piece))
	})
}

// check callers updaterequests
func (cn *connection) stopRequestingPiece(piece int) bool {
	return cn.pieceRequestOrder.Remove(piece)
}

// This is distinct from Torrent piece priority, which is the user's
// preference. Connection piece priority is specific to a connection and is
// used to pseudorandomly avoid connections always requesting the same pieces
// and thus wasting effort.
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
	return cn.pieceRequestOrder.Set(piece, prio) || cn.shouldRequestWithoutBias()
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
	if cn.t.haveInfo() && piece >= cn.t.numPieces() || piece < 0 {
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
	cn.peerSentHaveAll = false
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

func (cn *connection) onPeerSentHaveAll() error {
	cn.peerSentHaveAll = true
	cn.peerPieces.Clear()
	cn.peerPiecesChanged()
	return nil
}

func (cn *connection) peerSentHaveNone() error {
	cn.peerPieces.Clear()
	cn.peerSentHaveAll = false
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
	messageTypesSent.Add(msg.Type.String(), 1)
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

// Returns whether the connection could be useful to us. We're seeding and
// they want data, we don't have metainfo and they can provide it, etc.
func (c *connection) useful() bool {
	t := c.t
	if c.closed.IsSet() {
		return false
	}
	if !t.haveInfo() {
		return c.supportsExtension("ut_metadata")
	}
	if t.seeding() && c.PeerInterested {
		return true
	}
	if c.peerHasWantedPieces() {
		return true
	}
	return false
}

func (c *connection) lastHelpful() (ret time.Time) {
	ret = c.lastUsefulChunkReceived
	if c.t.seeding() && c.lastChunkSent.After(ret) {
		ret = c.lastChunkSent
	}
	return
}

func (c *connection) fastEnabled() bool {
	return c.PeerExtensionBytes.SupportsFast() && c.t.cl.extensionBytes.SupportsFast()
}

func (c *connection) reject(r request) {
	if !c.fastEnabled() {
		panic("fast not enabled")
	}
	c.Post(r.ToMsg(pp.Reject))
	delete(c.PeerRequests, r)
}

func (c *connection) onReadRequest(r request) error {
	requestedChunkLengths.Add(strconv.FormatUint(r.Length.Uint64(), 10), 1)
	if r.Begin+r.Length > c.t.pieceLength(int(r.Index)) {
		torrent.Add("bad requests received", 1)
		return errors.New("bad request")
	}
	if _, ok := c.PeerRequests[r]; ok {
		torrent.Add("duplicate requests received", 1)
		return nil
	}
	if c.Choked {
		torrent.Add("requests received while choking", 1)
		if c.fastEnabled() {
			torrent.Add("requests rejected while choking", 1)
			c.reject(r)
		}
		return nil
	}
	if len(c.PeerRequests) >= maxRequests {
		torrent.Add("requests received while queue full", 1)
		if c.fastEnabled() {
			c.reject(r)
		}
		// BEP 6 says we may close here if we choose.
		return nil
	}
	if !c.t.havePiece(r.Index.Int()) {
		// This isn't necessarily them screwing up. We can drop pieces
		// from our storage, and can't communicate this to peers
		// except by reconnecting.
		requestsReceivedForMissingPieces.Add(1)
		return fmt.Errorf("peer requested piece we don't have: %v", r.Index.Int())
	}
	if c.PeerRequests == nil {
		c.PeerRequests = make(map[request]struct{}, maxRequests)
	}
	c.PeerRequests[r] = struct{}{}
	c.tickleWriter()
	return nil
}

// Processes incoming bittorrent messages. The client lock is held upon entry
// and exit. Returning will end the connection.
func (c *connection) mainReadLoop() (err error) {
	defer func() {
		if err != nil {
			torrent.Add("connection.mainReadLoop returned with error", 1)
		} else {
			torrent.Add("connection.mainReadLoop returned with no error", 1)
		}
	}()
	t := c.t
	cl := t.cl

	decoder := pp.Decoder{
		R:         bufio.NewReaderSize(c.r, 1<<17),
		MaxLength: 256 * 1024,
		Pool:      t.chunkPool,
	}
	for {
		var msg pp.Message
		func() {
			cl.mu.Unlock()
			defer cl.mu.Lock()
			err = decoder.Decode(&msg)
		}()
		if t.closed.IsSet() || c.closed.IsSet() || err == io.EOF {
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
		messageTypesReceived.Add(msg.Type.String(), 1)
		if msg.Type.FastExtension() && !c.fastEnabled() {
			return fmt.Errorf("received fast extension message (type=%v) but extension is disabled", msg.Type)
		}
		switch msg.Type {
		case pp.Choke:
			c.PeerChoked = true
			c.deleteAllRequests()
			// We can then reset our interest.
			c.updateRequests()
		case pp.Reject:
			c.deleteRequest(newRequestFromMessage(&msg))
		case pp.Unchoke:
			c.PeerChoked = false
			c.tickleWriter()
		case pp.Interested:
			c.PeerInterested = true
			c.tickleWriter()
		case pp.NotInterested:
			c.PeerInterested = false
			// TODO: Reject?
			c.PeerRequests = nil
		case pp.Have:
			err = c.peerSentHave(int(msg.Index))
		case pp.Request:
			r := newRequestFromMessage(&msg)
			err = c.onReadRequest(r)
		case pp.Cancel:
			req := newRequestFromMessage(&msg)
			c.onPeerSentCancel(req)
		case pp.Bitfield:
			err = c.peerSentBitfield(msg.Bitfield)
		case pp.HaveAll:
			err = c.onPeerSentHaveAll()
		case pp.HaveNone:
			err = c.peerSentHaveNone()
		case pp.Piece:
			c.receiveChunk(&msg)
			if len(msg.Piece) == int(t.chunkSize) {
				t.chunkPool.Put(&msg.Piece)
			}
		case pp.Extended:
			err = c.onReadExtendedMsg(msg.ExtendedID, msg.ExtendedPayload)
		case pp.Port:
			pingAddr, err := net.ResolveUDPAddr("", c.remoteAddr().String())
			if err != nil {
				panic(err)
			}
			if msg.Port != 0 {
				pingAddr.Port = int(msg.Port)
			}
			cl.eachDhtServer(func(s *dht.Server) {
				go s.Ping(pingAddr, nil)
			})
		case pp.AllowedFast:
			torrent.Add("allowed fasts received", 1)
			log.Fmsg("peer allowed fast: %d", msg.Index).AddValues(c, debugLogValue).Log(c.t.logger)
			c.peerAllowedFast.Add(int(msg.Index))
			c.updateRequests()
		case pp.Suggest:
			torrent.Add("suggests received", 1)
			log.Fmsg("peer suggested piece %d", msg.Index).AddValues(c, msg.Index, debugLogValue).Log(c.t.logger)
			c.updateRequests()
		default:
			err = fmt.Errorf("received unknown message type: %#v", msg.Type)
		}
		if err != nil {
			return err
		}
	}
}

func (c *connection) onReadExtendedMsg(id byte, payload []byte) (err error) {
	defer func() {
		// TODO: Should we still do this?
		if err != nil {
			// These clients use their own extension IDs for outgoing message
			// types, which is incorrect.
			if bytes.HasPrefix(c.PeerID[:], []byte("-SD0100-")) || strings.HasPrefix(string(c.PeerID[:]), "-XL0012-") {
				err = nil
			}
		}
	}()
	t := c.t
	cl := t.cl
	switch id {
	case pp.HandshakeExtendedID:
		// TODO: Create a bencode struct for this.
		var d map[string]interface{}
		err := bencode.Unmarshal(payload, &d)
		if err != nil {
			return fmt.Errorf("error decoding extended message payload: %s", err)
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
		if m, ok := d["m"]; ok {
			mTyped, ok := m.(map[string]interface{})
			if !ok {
				return errors.New("handshake m value is not dict")
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
		}
		metadata_sizeUntyped, ok := d["metadata_size"]
		if ok {
			metadata_size, ok := metadata_sizeUntyped.(int64)
			if !ok {
				log.Printf("bad metadata_size type: %T", metadata_sizeUntyped)
			} else {
				err = t.setMetadataSize(metadata_size)
				if err != nil {
					return fmt.Errorf("error setting metadata size to %d", metadata_size)
				}
			}
		}
		if _, ok := c.PeerExtensionIDs["ut_metadata"]; ok {
			c.requestPendingMetadata()
		}
		return nil
	case metadataExtendedId:
		err := cl.gotMetadataExtensionMsg(payload, t, c)
		if err != nil {
			return fmt.Errorf("error handling metadata extension message: %s", err)
		}
		return nil
	case pexExtendedId:
		if cl.config.DisablePEX {
			// TODO: Maybe close the connection. Check that we're not
			// advertising that we support PEX if it's disabled.
			return nil
		}
		var pexMsg peerExchangeMessage
		err := bencode.Unmarshal(payload, &pexMsg)
		if err != nil {
			return fmt.Errorf("error unmarshalling PEX message: %s", err)
		}
		torrent.Add("pex added6 peers received", int64(len(pexMsg.Added6)))
		t.addPeers(pexMsg.AddedPeers())
		return nil
	default:
		return fmt.Errorf("unexpected extended message ID: %v", id)
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

	req := newRequestFromMessage(msg)

	// Request has been satisfied.
	if c.deleteRequest(req) {
		c.updateRequests()
	} else {
		unexpectedChunksReceived.Add(1)
	}

	if c.PeerChoked {
		torrent.Add("chunks received while choked", 1)
		if c.peerAllowedFast.Get(int(req.Index)) {
			torrent.Add("chunks received due to allowed fast", 1)
		}
	}

	// Do we actually want this chunk?
	if !t.wantPiece(req) {
		unwantedChunksReceived.Add(1)
		c.stats.ChunksReadUnwanted++
		c.t.stats.ChunksReadUnwanted++
		return
	}

	index := int(req.Index)
	piece := &t.pieces[index]

	c.stats.ChunksReadUseful++
	c.t.stats.ChunksReadUseful++
	c.stats.BytesReadUsefulData += int64(len(msg.Piece))
	c.t.stats.BytesReadUsefulData += int64(len(msg.Piece))
	c.lastUsefulChunkReceived = time.Now()
	// if t.fastestConn != c {
	// log.Printf("setting fastest connection %p", c)
	// }
	t.fastestConn = c

	// Need to record that it hasn't been written yet, before we attempt to do
	// anything with it.
	piece.incrementPendingWrites()
	// Record that we have the chunk, so we aren't trying to download it while
	// waiting for it to be written to storage.
	piece.unpendChunkIndex(chunkIndex(req.chunkSpec, t.chunkSize))

	// Cancel pending requests for this chunk.
	for c := range t.conns {
		c.postCancel(req)
	}

	err := func() error {
		cl.mu.Unlock()
		defer cl.mu.Lock()
		// Write the chunk out. Note that the upper bound on chunk writing
		// concurrency will be the number of connections. We write inline with
		// receiving the chunk (with this lock dance), because we want to
		// handle errors synchronously and I haven't thought of a nice way to
		// defer any concurrency to the storage and have that notify the
		// client of errors. TODO: Do that instead.
		return t.writeChunk(int(msg.Index), int64(msg.Begin), msg.Piece)
	}()

	piece.decrementPendingWrites()

	if err != nil {
		log.Printf("%s (%s): error writing chunk %v: %s", t, t.infoHash, req, err)
		t.pendRequest(req)
		t.updatePieceCompletion(int(msg.Index))
		return
	}

	// It's important that the piece is potentially queued before we check if
	// the piece is still wanted, because if it is queued, it won't be wanted.
	if t.pieceAllDirty(index) {
		t.queuePieceCheck(int(req.Index))
		t.pendAllChunkSpecs(index)
	}

	c.onDirtiedPiece(index)

	cl.event.Broadcast()
	t.publishPieceChange(int(req.Index))
}

func (c *connection) onDirtiedPiece(piece int) {
	if c.peerTouchedPieces == nil {
		c.peerTouchedPieces = make(map[int]struct{})
	}
	c.peerTouchedPieces[piece] = struct{}{}
	ds := &c.t.pieces[piece].dirtiers
	if *ds == nil {
		*ds = make(map[*connection]struct{})
	}
	(*ds)[c] = struct{}{}
}

func (c *connection) uploadAllowed() bool {
	if c.t.cl.config.NoUpload {
		return false
	}
	if c.t.seeding() {
		return true
	}
	if !c.peerHasWantedPieces() {
		return false
	}
	// Don't upload more than 100 KiB more than we download.
	if c.stats.BytesWrittenData >= c.stats.BytesReadData+100<<10 {
		return false
	}
	return true
}

func (c *connection) setRetryUploadTimer(delay time.Duration) {
	if c.uploadTimer == nil {
		c.uploadTimer = time.AfterFunc(delay, c.writerCond.Broadcast)
	} else {
		c.uploadTimer.Reset(delay)
	}
}

// Also handles choking and unchoking of the remote peer.
func (c *connection) upload(msg func(pp.Message) bool) bool {
	// Breaking or completing this loop means we don't want to upload to the
	// peer anymore, and we choke them.
another:
	for c.uploadAllowed() {
		// We want to upload to the peer.
		if !c.Unchoke(msg) {
			return false
		}
		for r := range c.PeerRequests {
			res := c.t.cl.uploadLimit.ReserveN(time.Now(), int(r.Length))
			if !res.OK() {
				panic(fmt.Sprintf("upload rate limiter burst size < %d", r.Length))
			}
			delay := res.Delay()
			if delay > 0 {
				res.Cancel()
				c.setRetryUploadTimer(delay)
				// Hard to say what to return here.
				return true
			}
			more, err := c.sendChunk(r, msg)
			if err != nil {
				i := int(r.Index)
				if c.t.pieceComplete(i) {
					c.t.updatePieceCompletion(i)
					if !c.t.pieceComplete(i) {
						// We had the piece, but not anymore.
						break another
					}
				}
				log.Str("error sending chunk to peer").AddValues(c, r, err).Log(c.t.logger)
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

func (cn *connection) netGoodPiecesDirtied() int64 {
	return cn.stats.PiecesDirtiedGood - cn.stats.PiecesDirtiedBad
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
	c.t.pendingRequests[r]--
	c.updateRequests()
	return true
}

func (c *connection) deleteAllRequests() {
	for r := range c.requests {
		c.deleteRequest(r)
	}
	if len(c.requests) != 0 {
		panic(len(c.requests))
	}
	// for c := range c.t.conns {
	// 	c.tickleWriter()
	// }
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

func (c *connection) sendChunk(r request, msg func(pp.Message) bool) (more bool, err error) {
	// Count the chunk being sent, even if it isn't.
	b := make([]byte, r.Length)
	p := c.t.info.Piece(int(r.Index))
	n, err := c.t.readAt(b, p.Offset()+int64(r.Begin))
	if n != len(b) {
		if err == nil {
			panic("expected error")
		}
		return
	} else if err == io.EOF {
		err = nil
	}
	more = msg(pp.Message{
		Type:  pp.Piece,
		Index: r.Index,
		Begin: r.Begin,
		Piece: b,
	})
	c.lastChunkSent = time.Now()
	return
}

func (c *connection) setTorrent(t *Torrent) {
	if c.t != nil {
		panic("connection already associated with a torrent")
	}
	c.t = t
	t.conns[c] = struct{}{}
}
