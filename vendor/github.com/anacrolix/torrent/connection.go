package torrent

import (
	"bufio"
	"bytes"
	"container/list"
	"errors"
	"expvar"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/bitmap"
	"github.com/anacrolix/missinggo/itertools"
	"github.com/anacrolix/missinggo/prioritybitmap"
	"github.com/bradfitz/iter"

	"github.com/anacrolix/torrent/bencode"
	pp "github.com/anacrolix/torrent/peer_protocol"
)

var optimizedCancels = expvar.NewInt("optimizedCancels")

type peerSource byte

const (
	peerSourceTracker  = '\x00' // It's the default.
	peerSourceIncoming = 'I'
	peerSourceDHT      = 'H'
	peerSourcePEX      = 'X'
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
	encrypted bool
	Discovery peerSource
	uTP       bool
	closed    missinggo.Event

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
	Requests         map[request]struct{}
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

	outgoingUnbufferedMessages         *list.List
	outgoingUnbufferedMessagesNotEmpty missinggo.Event
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
	if cn.encrypted {
		c('E')
	}
	if cn.Discovery != 0 {
		c(byte(cn.Discovery))
	}
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
		len(cn.Requests),
		len(cn.PeerRequests),
		cn.statusFlags(),
	)
}

func (cn *connection) Close() {
	cn.closed.Set()
	cn.discardPieceInclination()
	cn.pieceRequestOrder.Clear()
	if cn.conn != nil {
		// TODO: This call blocks sometimes, why?
		go cn.conn.Close()
	}
}

func (cn *connection) PeerHasPiece(piece int) bool {
	return cn.peerHasAll || cn.peerPieces.Contains(piece)
}

func (cn *connection) Post(msg pp.Message) {
	switch msg.Type {
	case pp.Cancel:
		for e := cn.outgoingUnbufferedMessages.Back(); e != nil; e = e.Prev() {
			elemMsg := e.Value.(pp.Message)
			if elemMsg.Type == pp.Request && elemMsg.Index == msg.Index && elemMsg.Begin == msg.Begin && elemMsg.Length == msg.Length {
				cn.outgoingUnbufferedMessages.Remove(e)
				optimizedCancels.Add(1)
				return
			}
		}
	}
	if cn.outgoingUnbufferedMessages == nil {
		cn.outgoingUnbufferedMessages = list.New()
	}
	cn.outgoingUnbufferedMessages.PushBack(msg)
	cn.outgoingUnbufferedMessagesNotEmpty.Set()
	postedMessageTypes.Add(strconv.FormatInt(int64(msg.Type), 10), 1)
}

func (cn *connection) RequestPending(r request) bool {
	_, ok := cn.Requests[r]
	return ok
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

// Returns true if more requests can be sent.
func (cn *connection) Request(chunk request) bool {
	if len(cn.Requests) >= cn.nominalMaxRequests() {
		return false
	}
	if !cn.PeerHasPiece(int(chunk.Index)) {
		return true
	}
	if cn.RequestPending(chunk) {
		return true
	}
	cn.SetInterested(true)
	if cn.PeerChoked {
		return false
	}
	if cn.Requests == nil {
		cn.Requests = make(map[request]struct{}, cn.PeerMaxRequests)
	}
	cn.Requests[chunk] = struct{}{}
	cn.requestsLowWater = len(cn.Requests) / 2
	cn.Post(pp.Message{
		Type:   pp.Request,
		Index:  chunk.Index,
		Begin:  chunk.Begin,
		Length: chunk.Length,
	})
	return true
}

// Returns true if an unsatisfied request was canceled.
func (cn *connection) Cancel(r request) bool {
	if !cn.RequestPending(r) {
		return false
	}
	delete(cn.Requests, r)
	cn.Post(pp.Message{
		Type:   pp.Cancel,
		Index:  r.Index,
		Begin:  r.Begin,
		Length: r.Length,
	})
	return true
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

func (cn *connection) Choke() {
	if cn.Choked {
		return
	}
	cn.Post(pp.Message{
		Type: pp.Choke,
	})
	cn.PeerRequests = nil
	cn.Choked = true
}

func (cn *connection) Unchoke() {
	if !cn.Choked {
		return
	}
	cn.Post(pp.Message{
		Type: pp.Unchoke,
	})
	cn.Choked = false
}

func (cn *connection) SetInterested(interested bool) {
	if cn.Interested == interested {
		return
	}
	cn.Post(pp.Message{
		Type: func() pp.MessageType {
			if interested {
				return pp.Interested
			} else {
				return pp.NotInterested
			}
		}(),
	})
	cn.Interested = interested
}

var (
	// Track connection writer buffer writes and flushes, to determine its
	// efficiency.
	connectionWriterFlush = expvar.NewInt("connectionWriterFlush")
	connectionWriterWrite = expvar.NewInt("connectionWriterWrite")
)

// Writes buffers to the socket from the write channel.
func (cn *connection) writer(keepAliveTimeout time.Duration) {
	defer func() {
		cn.mu().Lock()
		defer cn.mu().Unlock()
		cn.Close()
	}()
	// Reduce write syscalls.
	buf := bufio.NewWriter(cn.w)
	keepAliveTimer := time.NewTimer(keepAliveTimeout)
	for {
		cn.mu().Lock()
		for cn.outgoingUnbufferedMessages != nil && cn.outgoingUnbufferedMessages.Len() != 0 {
			msg := cn.outgoingUnbufferedMessages.Remove(cn.outgoingUnbufferedMessages.Front()).(pp.Message)
			cn.mu().Unlock()
			b, err := msg.MarshalBinary()
			if err != nil {
				panic(err)
			}
			connectionWriterWrite.Add(1)
			n, err := buf.Write(b)
			if err != nil {
				return
			}
			keepAliveTimer.Reset(keepAliveTimeout)
			if n != len(b) {
				panic("short write")
			}
			cn.mu().Lock()
			cn.wroteMsg(&msg)
		}
		cn.outgoingUnbufferedMessagesNotEmpty.Clear()
		cn.mu().Unlock()
		connectionWriterFlush.Add(1)
		if buf.Buffered() != 0 {
			if buf.Flush() != nil {
				return
			}
			keepAliveTimer.Reset(keepAliveTimeout)
		}
		select {
		case <-cn.closed.LockedChan(cn.mu()):
			return
		case <-cn.outgoingUnbufferedMessagesNotEmpty.LockedChan(cn.mu()):
		case <-keepAliveTimer.C:
			cn.mu().Lock()
			cn.Post(pp.Message{Keepalive: true})
			cn.mu().Unlock()
			postedKeepalives.Add(1)
		}
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

func (cn *connection) updateRequests() {
	if !cn.t.haveInfo() {
		return
	}
	if cn.Interested {
		if cn.PeerChoked {
			return
		}
		if len(cn.Requests) > cn.requestsLowWater {
			return
		}
	}
	cn.fillRequests()
	if len(cn.Requests) == 0 && !cn.PeerChoked {
		// So we're not choked, but we don't want anything right now. We may
		// have completed readahead, and the readahead window has not rolled
		// over to the next piece. Better to stay interested in case we're
		// going to want data in the near future.
		cn.SetInterested(!cn.t.haveAllPieces())
	}
}

func (cn *connection) fillRequests() {
	cn.pieceRequestOrder.IterTyped(func(piece int) (more bool) {
		if cn.t.cl.config.Debug && cn.t.havePiece(piece) {
			panic(piece)
		}
		return cn.requestPiecePendingChunks(piece)
	})
}

func (c *connection) requestPiecePendingChunks(piece int) (again bool) {
	if !c.PeerHasPiece(piece) {
		return true
	}
	chunkIndices := c.t.pieces[piece].undirtiedChunkIndices().ToSortedSlice()
	return itertools.ForPerm(len(chunkIndices), func(i int) bool {
		req := request{pp.Integer(piece), c.t.chunkIndexSpec(chunkIndices[i], piece)}
		return c.Request(req)
	})
}

func (cn *connection) stopRequestingPiece(piece int) {
	cn.pieceRequestOrder.Remove(piece)
}

func (cn *connection) updatePiecePriority(piece int) {
	tpp := cn.t.piecePriority(piece)
	if !cn.PeerHasPiece(piece) {
		tpp = PiecePriorityNone
	}
	if tpp == PiecePriorityNone {
		cn.stopRequestingPiece(piece)
		return
	}
	prio := cn.getPieceInclination()[piece]
	switch tpp {
	case PiecePriorityNormal:
	case PiecePriorityReadahead:
		prio -= cn.t.numPieces()
	case PiecePriorityNext, PiecePriorityNow:
		prio -= 2 * cn.t.numPieces()
	default:
		panic(tpp)
	}
	prio += piece / 2
	cn.pieceRequestOrder.Set(piece, prio)
	cn.updateRequests()
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

func (cn *connection) peerHasPieceChanged(piece int) {
	cn.updatePiecePriority(piece)
}

func (cn *connection) peerPiecesChanged() {
	if cn.t.haveInfo() {
		for i := range iter.N(cn.t.numPieces()) {
			cn.peerHasPieceChanged(i)
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
	cn.peerHasPieceChanged(piece)
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
	return t.connHasWantedPieces(c)
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
		R:         bufio.NewReader(c.r),
		MaxLength: 256 * 1024,
		Pool:      t.chunkPool,
	}
	for {
		cl.mu.Unlock()
		var msg pp.Message
		err := decoder.Decode(&msg)
		cl.mu.Lock()
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
		receivedMessageTypes.Add(strconv.FormatInt(int64(msg.Type), 10), 1)
		switch msg.Type {
		case pp.Choke:
			c.PeerChoked = true
			c.Requests = nil
			// We can then reset our interest.
			c.updateRequests()
		case pp.Reject:
			cl.connDeleteRequest(t, c, newRequest(msg.Index, msg.Begin, msg.Length))
			c.updateRequests()
		case pp.Unchoke:
			c.PeerChoked = false
			cl.peerUnchoked(t, c)
		case pp.Interested:
			c.PeerInterested = true
			cl.upload(t, c)
		case pp.NotInterested:
			c.PeerInterested = false
			c.Choke()
		case pp.Have:
			err = c.peerSentHave(int(msg.Index))
		case pp.Request:
			if c.Choked {
				break
			}
			if !c.PeerInterested {
				err = errors.New("peer sent request but isn't interested")
				break
			}
			if !t.havePiece(msg.Index.Int()) {
				// This isn't necessarily them screwing up. We can drop pieces
				// from our storage, and can't communicate this to peers
				// except by reconnecting.
				requestsReceivedForMissingPieces.Add(1)
				err = errors.New("peer requested piece we don't have")
				break
			}
			if c.PeerRequests == nil {
				c.PeerRequests = make(map[request]struct{}, maxRequests)
			}
			c.PeerRequests[newRequest(msg.Index, msg.Begin, msg.Length)] = struct{}{}
			cl.upload(t, c)
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
			cl.downloadedChunk(t, c, &msg)
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
			cl.dHT.Ping(pingAddr)
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
