package torrent

import (
	"container/heap"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/bitmap"
	"github.com/anacrolix/missinggo/perf"
	"github.com/anacrolix/missinggo/pubsub"
	"github.com/anacrolix/missinggo/slices"
	"github.com/bradfitz/iter"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/dht"
	"github.com/anacrolix/torrent/metainfo"
	pp "github.com/anacrolix/torrent/peer_protocol"
	"github.com/anacrolix/torrent/storage"
	"github.com/anacrolix/torrent/tracker"
)

func (t *Torrent) chunkIndexSpec(chunkIndex, piece int) chunkSpec {
	return chunkIndexSpec(chunkIndex, t.pieceLength(piece), t.chunkSize)
}

type peersKey struct {
	IPBytes string
	Port    int
}

// Maintains state of torrent within a Client.
type Torrent struct {
	cl *Client

	closed   missinggo.Event
	infoHash metainfo.Hash
	pieces   []piece
	// Values are the piece indices that changed.
	pieceStateChanges *pubsub.PubSub
	// The size of chunks to request from peers over the wire. This is
	// normally 16KiB by convention these days.
	chunkSize pp.Integer
	chunkPool *sync.Pool
	// Total length of the torrent in bytes. Stored because it's not O(1) to
	// get this from the info dict.
	length int64

	// The storage to open when the info dict becomes available.
	storageOpener *storage.Client
	// Storage for torrent data.
	storage *storage.Torrent

	metainfo metainfo.MetaInfo

	// The info dict. nil if we don't have it (yet).
	info *metainfo.Info
	// Active peer connections, running message stream loops.
	conns               []*connection
	maxEstablishedConns int
	// Set of addrs to which we're attempting to connect. Connections are
	// half-open until all handshakes are completed.
	halfOpen map[string]struct{}

	// Reserve of peers to connect to. A peer can be both here and in the
	// active connections if were told about the peer after connecting with
	// them. That encourages us to reconnect to peers that are well known in
	// the swarm.
	peers          map[peersKey]Peer
	wantPeersEvent missinggo.Event
	// An announcer for each tracker URL.
	trackerAnnouncers map[string]*trackerScraper
	// How many times we've initiated a DHT announce. TODO: Move into stats.
	numDHTAnnounces int

	// Name used if the info name isn't available. Should be cleared when the
	// Info does become available.
	displayName string
	// The bencoded bytes of the info dict. This is actively manipulated if
	// the info bytes aren't initially available, and we try to fetch them
	// from peers.
	metadataBytes []byte
	// Each element corresponds to the 16KiB metadata pieces. If true, we have
	// received that piece.
	metadataCompletedChunks []bool

	// Set when .Info is obtained.
	gotMetainfo missinggo.Event

	readers               map[*Reader]struct{}
	readerNowPieces       bitmap.Bitmap
	readerReadaheadPieces bitmap.Bitmap

	// The indexes of pieces we want with normal priority, that aren't
	// currently available.
	pendingPieces bitmap.Bitmap
	// A cache of completed piece indices.
	completedPieces bitmap.Bitmap

	// A pool of piece priorities []int for assignment to new connections.
	// These "inclinations" are used to give connections preference for
	// different pieces.
	connPieceInclinationPool sync.Pool
	// Torrent-level statistics.
	stats TorrentStats
}

func (t *Torrent) setChunkSize(size pp.Integer) {
	t.chunkSize = size
	t.chunkPool = &sync.Pool{
		New: func() interface{} {
			return make([]byte, size)
		},
	}
}

func (t *Torrent) setDisplayName(dn string) {
	if t.haveInfo() {
		return
	}
	t.displayName = dn
}

func (t *Torrent) pieceComplete(piece int) bool {
	return t.completedPieces.Get(piece)
}

func (t *Torrent) pieceCompleteUncached(piece int) bool {
	return t.pieces[piece].Storage().GetIsComplete()
}

func (t *Torrent) numConnsUnchoked() (num int) {
	for _, c := range t.conns {
		if !c.PeerChoked {
			num++
		}
	}
	return
}

// There's a connection to that address already.
func (t *Torrent) addrActive(addr string) bool {
	if _, ok := t.halfOpen[addr]; ok {
		return true
	}
	for _, c := range t.conns {
		if c.remoteAddr().String() == addr {
			return true
		}
	}
	return false
}

func (t *Torrent) worstUnclosedConns() (ret []*connection) {
	ret = make([]*connection, 0, len(t.conns))
	for _, c := range t.conns {
		if !c.closed.IsSet() {
			ret = append(ret, c)
		}
	}
	return
}

func (t *Torrent) addPeer(p Peer) {
	cl := t.cl
	cl.openNewConns(t)
	if len(t.peers) >= torrentPeersHighWater {
		return
	}
	key := peersKey{string(p.IP), p.Port}
	if _, ok := t.peers[key]; ok {
		return
	}
	t.peers[key] = p
	peersAddedBySource.Add(string(p.Source), 1)
	cl.openNewConns(t)

}

func (t *Torrent) invalidateMetadata() {
	for i := range t.metadataCompletedChunks {
		t.metadataCompletedChunks[i] = false
	}
	t.info = nil
}

func (t *Torrent) saveMetadataPiece(index int, data []byte) {
	if t.haveInfo() {
		return
	}
	if index >= len(t.metadataCompletedChunks) {
		log.Printf("%s: ignoring metadata piece %d", t, index)
		return
	}
	copy(t.metadataBytes[(1<<14)*index:], data)
	t.metadataCompletedChunks[index] = true
}

func (t *Torrent) metadataPieceCount() int {
	return (len(t.metadataBytes) + (1 << 14) - 1) / (1 << 14)
}

func (t *Torrent) haveMetadataPiece(piece int) bool {
	if t.haveInfo() {
		return (1<<14)*piece < len(t.metadataBytes)
	} else {
		return piece < len(t.metadataCompletedChunks) && t.metadataCompletedChunks[piece]
	}
}

func (t *Torrent) metadataSizeKnown() bool {
	return t.metadataBytes != nil
}

func (t *Torrent) metadataSize() int {
	return len(t.metadataBytes)
}

func infoPieceHashes(info *metainfo.Info) (ret []string) {
	for i := 0; i < len(info.Pieces); i += sha1.Size {
		ret = append(ret, string(info.Pieces[i:i+sha1.Size]))
	}
	return
}

func (t *Torrent) makePieces() {
	hashes := infoPieceHashes(t.info)
	t.pieces = make([]piece, len(hashes))
	for i, hash := range hashes {
		piece := &t.pieces[i]
		piece.t = t
		piece.index = i
		piece.noPendingWrites.L = &piece.pendingWritesMutex
		missinggo.CopyExact(piece.Hash[:], hash)
	}
}

// Called when metadata for a torrent becomes available.
func (t *Torrent) setInfoBytes(b []byte) error {
	if t.haveInfo() {
		return nil
	}
	if metainfo.HashBytes(b) != t.infoHash {
		return errors.New("info bytes have wrong hash")
	}
	var info metainfo.Info
	err := bencode.Unmarshal(b, &info)
	if err != nil {
		return fmt.Errorf("error unmarshalling info bytes: %s", err)
	}
	err = validateInfo(&info)
	if err != nil {
		return fmt.Errorf("bad info: %s", err)
	}
	defer t.updateWantPeersEvent()
	t.info = &info
	t.displayName = "" // Save a few bytes lol.
	t.cl.event.Broadcast()
	t.gotMetainfo.Set()
	t.storage, err = t.storageOpener.OpenTorrent(t.info, t.infoHash)
	if err != nil {
		return fmt.Errorf("error opening torrent storage: %s", err)
	}
	t.length = 0
	for _, f := range t.info.UpvertedFiles() {
		t.length += f.Length
	}
	t.metadataBytes = b
	t.metadataCompletedChunks = nil
	t.makePieces()
	for _, conn := range t.conns {
		if err := conn.setNumPieces(t.numPieces()); err != nil {
			log.Printf("closing connection: %s", err)
			conn.Close()
		}
	}
	for i := range t.pieces {
		t.updatePieceCompletion(i)
		t.pieces[i].QueuedForHash = true
	}
	go func() {
		for i := range t.pieces {
			t.verifyPiece(i)
		}
	}()
	return nil
}

func (t *Torrent) verifyPiece(piece int) {
	t.cl.verifyPiece(t, piece)
}

func (t *Torrent) haveAllMetadataPieces() bool {
	if t.haveInfo() {
		return true
	}
	if t.metadataCompletedChunks == nil {
		return false
	}
	for _, have := range t.metadataCompletedChunks {
		if !have {
			return false
		}
	}
	return true
}

// TODO: Propagate errors to disconnect peer.
func (t *Torrent) setMetadataSize(bytes int64) (err error) {
	if t.haveInfo() {
		// We already know the correct metadata size.
		return
	}
	if bytes <= 0 || bytes > 10000000 { // 10MB, pulled from my ass.
		return errors.New("bad size")
	}
	if t.metadataBytes != nil && len(t.metadataBytes) == int(bytes) {
		return
	}
	t.metadataBytes = make([]byte, bytes)
	t.metadataCompletedChunks = make([]bool, (bytes+(1<<14)-1)/(1<<14))
	for _, c := range t.conns {
		c.requestPendingMetadata()
	}
	return
}

// The current working name for the torrent. Either the name in the info dict,
// or a display name given such as by the dn value in a magnet link, or "".
func (t *Torrent) name() string {
	if t.haveInfo() {
		return t.info.Name
	}
	return t.displayName
}

func (t *Torrent) pieceState(index int) (ret PieceState) {
	p := &t.pieces[index]
	ret.Priority = t.piecePriority(index)
	if t.pieceComplete(index) {
		ret.Complete = true
	}
	if p.QueuedForHash || p.Hashing {
		ret.Checking = true
	}
	if !ret.Complete && t.piecePartiallyDownloaded(index) {
		ret.Partial = true
	}
	return
}

func (t *Torrent) metadataPieceSize(piece int) int {
	return metadataPieceSize(len(t.metadataBytes), piece)
}

func (t *Torrent) newMetadataExtensionMessage(c *connection, msgType int, piece int, data []byte) pp.Message {
	d := map[string]int{
		"msg_type": msgType,
		"piece":    piece,
	}
	if data != nil {
		d["total_size"] = len(t.metadataBytes)
	}
	p, err := bencode.Marshal(d)
	if err != nil {
		panic(err)
	}
	return pp.Message{
		Type:            pp.Extended,
		ExtendedID:      c.PeerExtensionIDs["ut_metadata"],
		ExtendedPayload: append(p, data...),
	}
}

func (t *Torrent) pieceStateRuns() (ret []PieceStateRun) {
	rle := missinggo.NewRunLengthEncoder(func(el interface{}, count uint64) {
		ret = append(ret, PieceStateRun{
			PieceState: el.(PieceState),
			Length:     int(count),
		})
	})
	for index := range t.pieces {
		rle.Append(t.pieceState(index), 1)
	}
	rle.Flush()
	return
}

// Produces a small string representing a PieceStateRun.
func pieceStateRunStatusChars(psr PieceStateRun) (ret string) {
	ret = fmt.Sprintf("%d", psr.Length)
	ret += func() string {
		switch psr.Priority {
		case PiecePriorityNext:
			return "N"
		case PiecePriorityNormal:
			return "."
		case PiecePriorityReadahead:
			return "R"
		case PiecePriorityNow:
			return "!"
		default:
			return ""
		}
	}()
	if psr.Checking {
		ret += "H"
	}
	if psr.Partial {
		ret += "P"
	}
	if psr.Complete {
		ret += "C"
	}
	return
}

func (t *Torrent) writeStatus(w io.Writer) {
	fmt.Fprintf(w, "Infohash: %x\n", t.infoHash)
	fmt.Fprintf(w, "Metadata length: %d\n", t.metadataSize())
	if !t.haveInfo() {
		fmt.Fprintf(w, "Metadata have: ")
		for _, h := range t.metadataCompletedChunks {
			fmt.Fprintf(w, "%c", func() rune {
				if h {
					return 'H'
				} else {
					return '.'
				}
			}())
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "Piece length: %s\n", func() string {
		if t.haveInfo() {
			return fmt.Sprint(t.usualPieceSize())
		} else {
			return "?"
		}
	}())
	if t.Info() != nil {
		fmt.Fprintf(w, "Num Pieces: %d\n", t.numPieces())
		fmt.Fprint(w, "Piece States:")
		for _, psr := range t.PieceStateRuns() {
			w.Write([]byte(" "))
			w.Write([]byte(pieceStateRunStatusChars(psr)))
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "Reader Pieces:")
	t.forReaderOffsetPieces(func(begin, end int) (again bool) {
		fmt.Fprintf(w, " %d:%d", begin, end)
		return true
	})
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Trackers:\n")
	func() {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "    URL\tNext announce\tLast announce\n")
		for _, ta := range slices.Sort(slices.FromMapElems(t.trackerAnnouncers), func(l, r *trackerScraper) bool {
			return l.url < r.url
		}).([]*trackerScraper) {
			fmt.Fprintf(tw, "    %s\n", ta.statusLine())
		}
		tw.Flush()
	}()

	fmt.Fprintf(w, "DHT Announces: %d\n", t.numDHTAnnounces)

	fmt.Fprintf(w, "Pending peers: %d\n", len(t.peers))
	fmt.Fprintf(w, "Half open: %d\n", len(t.halfOpen))
	fmt.Fprintf(w, "Active peers: %d\n", len(t.conns))
	slices.Sort(t.conns, worseConn)
	for i, c := range t.conns {
		fmt.Fprintf(w, "%2d. ", i+1)
		c.WriteStatus(w, t)
	}
}

func (t *Torrent) haveInfo() bool {
	return t.info != nil
}

// TODO: Include URIs that weren't converted to tracker clients.
func (t *Torrent) announceList() (al [][]string) {
	return t.metainfo.AnnounceList
}

// Returns a run-time generated MetaInfo that includes the info bytes and
// announce-list as currently known to the client.
func (t *Torrent) newMetaInfo() metainfo.MetaInfo {
	return metainfo.MetaInfo{
		CreationDate: time.Now().Unix(),
		Comment:      "dynamic metainfo from client",
		CreatedBy:    "go.torrent",
		AnnounceList: t.announceList(),
		InfoBytes:    t.metadataBytes,
	}
}

func (t *Torrent) BytesMissing() int64 {
	t.mu().RLock()
	defer t.mu().RUnlock()
	return t.bytesLeft()
}

func (t *Torrent) bytesLeft() (left int64) {
	for i := 0; i < t.numPieces(); i++ {
		left += int64(t.pieces[i].bytesLeft())
	}
	return
}

// Bytes left to give in tracker announces.
func (t *Torrent) bytesLeftAnnounce() uint64 {
	if t.haveInfo() {
		return uint64(t.bytesLeft())
	} else {
		return math.MaxUint64
	}
}

func (t *Torrent) piecePartiallyDownloaded(piece int) bool {
	if t.pieceComplete(piece) {
		return false
	}
	if t.pieceAllDirty(piece) {
		return false
	}
	return t.pieces[piece].hasDirtyChunks()
}

func (t *Torrent) usualPieceSize() int {
	return int(t.info.PieceLength)
}

func (t *Torrent) lastPieceSize() int {
	return int(t.pieceLength(t.numPieces() - 1))
}

func (t *Torrent) numPieces() int {
	return t.info.NumPieces()
}

func (t *Torrent) numPiecesCompleted() (num int) {
	return t.completedPieces.Len()
}

func (t *Torrent) close() (err error) {
	t.closed.Set()
	if t.storage != nil {
		t.storage.Close()
	}
	for _, conn := range t.conns {
		conn.Close()
	}
	t.pieceStateChanges.Close()
	t.updateWantPeersEvent()
	return
}

func (t *Torrent) requestOffset(r request) int64 {
	return torrentRequestOffset(t.length, int64(t.usualPieceSize()), r)
}

// Return the request that would include the given offset into the torrent
// data. Returns !ok if there is no such request.
func (t *Torrent) offsetRequest(off int64) (req request, ok bool) {
	return torrentOffsetRequest(t.length, t.info.PieceLength, int64(t.chunkSize), off)
}

func (t *Torrent) writeChunk(piece int, begin int64, data []byte) (err error) {
	tr := perf.NewTimer()

	n, err := t.pieces[piece].Storage().WriteAt(data, begin)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		tr.Stop("write chunk")
	}
	return
}

func (t *Torrent) bitfield() (bf []bool) {
	bf = make([]bool, t.numPieces())
	t.completedPieces.IterTyped(func(piece int) (again bool) {
		bf[piece] = true
		return true
	})
	return
}

func (t *Torrent) validOutgoingRequest(r request) bool {
	if r.Index >= pp.Integer(t.info.NumPieces()) {
		return false
	}
	if r.Begin%t.chunkSize != 0 {
		return false
	}
	if r.Length > t.chunkSize {
		return false
	}
	pieceLength := t.pieceLength(int(r.Index))
	if r.Begin+r.Length > pieceLength {
		return false
	}
	return r.Length == t.chunkSize || r.Begin+r.Length == pieceLength
}

func (t *Torrent) pieceChunks(piece int) (css []chunkSpec) {
	css = make([]chunkSpec, 0, (t.pieceLength(piece)+t.chunkSize-1)/t.chunkSize)
	var cs chunkSpec
	for left := t.pieceLength(piece); left != 0; left -= cs.Length {
		cs.Length = left
		if cs.Length > t.chunkSize {
			cs.Length = t.chunkSize
		}
		css = append(css, cs)
		cs.Begin += cs.Length
	}
	return
}

func (t *Torrent) pieceNumChunks(piece int) int {
	return int((t.pieceLength(piece) + t.chunkSize - 1) / t.chunkSize)
}

func (t *Torrent) pendAllChunkSpecs(pieceIndex int) {
	t.pieces[pieceIndex].DirtyChunks.Clear()
}

type Peer struct {
	Id     [20]byte
	IP     net.IP
	Port   int
	Source peerSource
	// Peer is known to support encryption.
	SupportsEncryption bool
}

func (t *Torrent) pieceLength(piece int) (len_ pp.Integer) {
	if piece < 0 || piece >= t.info.NumPieces() {
		return
	}
	if piece == t.numPieces()-1 {
		len_ = pp.Integer(t.length % t.info.PieceLength)
	}
	if len_ == 0 {
		len_ = pp.Integer(t.info.PieceLength)
	}
	return
}

func (t *Torrent) hashPiece(piece int) (ret metainfo.Hash) {
	hash := pieceHash.New()
	p := &t.pieces[piece]
	p.waitNoPendingWrites()
	ip := t.info.Piece(piece)
	pl := ip.Length()
	n, err := io.Copy(hash, io.NewSectionReader(t.pieces[piece].Storage(), 0, pl))
	if n == pl {
		missinggo.CopyExact(&ret, hash.Sum(nil))
		return
	}
	if err != io.ErrUnexpectedEOF && !os.IsNotExist(err) {
		log.Printf("unexpected error hashing piece with %T: %s", t.storage, err)
	}
	return
}

func (t *Torrent) haveAllPieces() bool {
	if !t.haveInfo() {
		return false
	}
	return t.completedPieces.Len() == t.numPieces()
}

func (t *Torrent) haveAnyPieces() bool {
	for i := range t.pieces {
		if t.pieceComplete(i) {
			return true
		}
	}
	return false
}

func (t *Torrent) havePiece(index int) bool {
	return t.haveInfo() && t.pieceComplete(index)
}

func (t *Torrent) haveChunk(r request) (ret bool) {
	// defer func() {
	// 	log.Println("have chunk", r, ret)
	// }()
	if !t.haveInfo() {
		return false
	}
	if t.pieceComplete(int(r.Index)) {
		return true
	}
	p := &t.pieces[r.Index]
	return !p.pendingChunk(r.chunkSpec, t.chunkSize)
}

func chunkIndex(cs chunkSpec, chunkSize pp.Integer) int {
	return int(cs.Begin / chunkSize)
}

func (t *Torrent) wantPiece(r request) bool {
	if !t.wantPieceIndex(int(r.Index)) {
		return false
	}
	if t.pieces[r.Index].pendingChunk(r.chunkSpec, t.chunkSize) {
		return true
	}
	// TODO: What about pieces that were wanted, but aren't now, and aren't
	// completed either? That used to be done here.
	return false
}

func (t *Torrent) wantPieceIndex(index int) bool {
	if !t.haveInfo() {
		return false
	}
	p := &t.pieces[index]
	if p.QueuedForHash {
		return false
	}
	if p.Hashing {
		return false
	}
	if t.pieceComplete(index) {
		return false
	}
	if t.pendingPieces.Contains(index) {
		return true
	}
	return !t.forReaderOffsetPieces(func(begin, end int) bool {
		return index < begin || index >= end
	})
}

func (t *Torrent) forNeededPieces(f func(piece int) (more bool)) (all bool) {
	return t.forReaderOffsetPieces(func(begin, end int) (more bool) {
		for i := begin; begin < end; i++ {
			if !f(i) {
				return false
			}
		}
		return true
	})
}

func (t *Torrent) connHasWantedPieces(c *connection) bool {
	return !c.pieceRequestOrder.IsEmpty()
}

func (t *Torrent) extentPieces(off, _len int64) (pieces []int) {
	for i := off / int64(t.usualPieceSize()); i*int64(t.usualPieceSize()) < off+_len; i++ {
		pieces = append(pieces, int(i))
	}
	return
}

// The worst connection is one that hasn't been sent, or sent anything useful
// for the longest. A bad connection is one that usually sends us unwanted
// pieces, or has been in worser half of the established connections for more
// than a minute.
func (t *Torrent) worstBadConn() *connection {
	wcs := slices.HeapInterface(t.worstUnclosedConns(), worseConn)
	for wcs.Len() != 0 {
		c := heap.Pop(wcs).(*connection)
		if c.UnwantedChunksReceived >= 6 && c.UnwantedChunksReceived > c.UsefulChunksReceived {
			return c
		}
		if wcs.Len() >= (t.maxEstablishedConns+1)/2 {
			// Give connections 1 minute to prove themselves.
			if time.Since(c.completedHandshake) > time.Minute {
				return c
			}
		}
	}
	return nil
}

type PieceStateChange struct {
	Index int
	PieceState
}

func (t *Torrent) publishPieceChange(piece int) {
	cur := t.pieceState(piece)
	p := &t.pieces[piece]
	if cur != p.PublicPieceState {
		p.PublicPieceState = cur
		t.pieceStateChanges.Publish(PieceStateChange{
			piece,
			cur,
		})
	}
}

func (t *Torrent) pieceNumPendingChunks(piece int) int {
	if t.pieceComplete(piece) {
		return 0
	}
	return t.pieceNumChunks(piece) - t.pieces[piece].numDirtyChunks()
}

func (t *Torrent) pieceAllDirty(piece int) bool {
	return t.pieces[piece].DirtyChunks.Len() == t.pieceNumChunks(piece)
}

func (t *Torrent) forUrgentPieces(f func(piece int) (again bool)) (all bool) {
	return t.forReaderOffsetPieces(func(begin, end int) (again bool) {
		if begin < end {
			if !f(begin) {
				return false
			}
		}
		return true
	})
}

func (t *Torrent) readersChanged() {
	t.readerNowPieces, t.readerReadaheadPieces = t.readerPiecePriorities()
	t.updatePiecePriorities()
}

func (t *Torrent) maybeNewConns() {
	// Tickle the accept routine.
	t.cl.event.Broadcast()
	t.openNewConns()
}

func (t *Torrent) piecePriorityChanged(piece int) {
	for _, c := range t.conns {
		c.updatePiecePriority(piece)
	}
	t.maybeNewConns()
	t.publishPieceChange(piece)
}

func (t *Torrent) updatePiecePriority(piece int) {
	p := &t.pieces[piece]
	newPrio := t.piecePriorityUncached(piece)
	if newPrio == p.priority {
		return
	}
	p.priority = newPrio
	t.piecePriorityChanged(piece)
}

// Update all piece priorities in one hit. This function should have the same
// output as updatePiecePriority, but across all pieces.
func (t *Torrent) updatePiecePriorities() {
	for i := range t.pieces {
		t.updatePiecePriority(i)
	}
}

// Returns the range of pieces [begin, end) that contains the extent of bytes.
func (t *Torrent) byteRegionPieces(off, size int64) (begin, end int) {
	if off >= t.length {
		return
	}
	if off < 0 {
		size += off
		off = 0
	}
	if size <= 0 {
		return
	}
	begin = int(off / t.info.PieceLength)
	end = int((off + size + t.info.PieceLength - 1) / t.info.PieceLength)
	if end > t.info.NumPieces() {
		end = t.info.NumPieces()
	}
	return
}

// Returns true if all iterations complete without breaking. Returns the read
// regions for all readers. The reader regions should not be merged as some
// callers depend on this method to enumerate readers.
func (t *Torrent) forReaderOffsetPieces(f func(begin, end int) (more bool)) (all bool) {
	for r := range t.readers {
		p := r.pieces
		if p.begin >= p.end {
			continue
		}
		if !f(p.begin, p.end) {
			return false
		}
	}
	return true
}

func (t *Torrent) piecePriority(piece int) piecePriority {
	if !t.haveInfo() {
		return PiecePriorityNone
	}
	return t.pieces[piece].priority
}

func (t *Torrent) piecePriorityUncached(piece int) piecePriority {
	if t.pieceComplete(piece) {
		return PiecePriorityNone
	}
	if t.readerNowPieces.Contains(piece) {
		return PiecePriorityNow
	}
	// if t.readerNowPieces.Contains(piece - 1) {
	// 	return PiecePriorityNext
	// }
	if t.readerReadaheadPieces.Contains(piece) {
		return PiecePriorityReadahead
	}
	if t.pendingPieces.Contains(piece) {
		return PiecePriorityNormal
	}
	return PiecePriorityNone
}

func (t *Torrent) pendPiece(piece int) {
	if t.pendingPieces.Contains(piece) {
		return
	}
	if t.havePiece(piece) {
		return
	}
	t.pendingPieces.Add(piece)
	t.updatePiecePriority(piece)
}

func (t *Torrent) getCompletedPieces() (ret bitmap.Bitmap) {
	return t.completedPieces.Copy()
}

func (t *Torrent) unpendPieces(unpend *bitmap.Bitmap) {
	t.pendingPieces.Sub(unpend)
	t.updatePiecePriorities()
}

func (t *Torrent) pendPieceRange(begin, end int) {
	for i := begin; i < end; i++ {
		t.pendPiece(i)
	}
}

func (t *Torrent) unpendPieceRange(begin, end int) {
	var bm bitmap.Bitmap
	bm.AddRange(begin, end)
	t.unpendPieces(&bm)
}

func (t *Torrent) pendRequest(req request) {
	ci := chunkIndex(req.chunkSpec, t.chunkSize)
	t.pieces[req.Index].pendChunkIndex(ci)
}

func (t *Torrent) pieceChanged(piece int) {
	t.cl.pieceChanged(t, piece)
}

func (t *Torrent) openNewConns() {
	t.cl.openNewConns(t)
}

func (t *Torrent) getConnPieceInclination() []int {
	_ret := t.connPieceInclinationPool.Get()
	if _ret == nil {
		pieceInclinationsNew.Add(1)
		return rand.Perm(t.numPieces())
	}
	pieceInclinationsReused.Add(1)
	return _ret.([]int)
}

func (t *Torrent) putPieceInclination(pi []int) {
	t.connPieceInclinationPool.Put(pi)
	pieceInclinationsPut.Add(1)
}

func (t *Torrent) updatePieceCompletion(piece int) {
	pcu := t.pieceCompleteUncached(piece)
	changed := t.completedPieces.Get(piece) != pcu
	t.completedPieces.Set(piece, pcu)
	if changed {
		t.pieceChanged(piece)
	}
}

// Non-blocking read. Client lock is not required.
func (t *Torrent) readAt(b []byte, off int64) (n int, err error) {
	p := &t.pieces[off/t.info.PieceLength]
	p.waitNoPendingWrites()
	return p.Storage().ReadAt(b, off-p.Info().Offset())
}

func (t *Torrent) updateAllPieceCompletions() {
	for i := range iter.N(t.numPieces()) {
		t.updatePieceCompletion(i)
	}
}

// Returns an error if the metadata was completed, but couldn't be set for
// some reason. Blame it on the last peer to contribute.
func (t *Torrent) maybeCompleteMetadata() error {
	if t.haveInfo() {
		// Nothing to do.
		return nil
	}
	if !t.haveAllMetadataPieces() {
		// Don't have enough metadata pieces.
		return nil
	}
	err := t.setInfoBytes(t.metadataBytes)
	if err != nil {
		t.invalidateMetadata()
		return fmt.Errorf("error setting info bytes: %s", err)
	}
	if t.cl.config.Debug {
		log.Printf("%s: got metadata from peers", t)
	}
	return nil
}

func (t *Torrent) readerPieces() (ret bitmap.Bitmap) {
	t.forReaderOffsetPieces(func(begin, end int) bool {
		ret.AddRange(begin, end)
		return true
	})
	return
}

func (t *Torrent) readerPiecePriorities() (now, readahead bitmap.Bitmap) {
	t.forReaderOffsetPieces(func(begin, end int) bool {
		if end > begin {
			now.Add(begin)
			readahead.AddRange(begin+1, end)
		}
		return true
	})
	return
}

func (t *Torrent) needData() bool {
	if t.closed.IsSet() {
		return false
	}
	if !t.haveInfo() {
		return true
	}
	if t.pendingPieces.Len() != 0 {
		return true
	}
	// Read as "not all complete".
	return !t.readerPieces().IterTyped(func(piece int) bool {
		return t.pieceComplete(piece)
	})
}

func appendMissingStrings(old, new []string) (ret []string) {
	ret = old
new:
	for _, n := range new {
		for _, o := range old {
			if o == n {
				continue new
			}
		}
		ret = append(ret, n)
	}
	return
}

func appendMissingTrackerTiers(existing [][]string, minNumTiers int) (ret [][]string) {
	ret = existing
	for minNumTiers > len(ret) {
		ret = append(ret, nil)
	}
	return
}

func (t *Torrent) addTrackers(announceList [][]string) {
	fullAnnounceList := &t.metainfo.AnnounceList
	t.metainfo.AnnounceList = appendMissingTrackerTiers(*fullAnnounceList, len(announceList))
	for tierIndex, trackerURLs := range announceList {
		(*fullAnnounceList)[tierIndex] = appendMissingStrings((*fullAnnounceList)[tierIndex], trackerURLs)
	}
	t.startMissingTrackerScrapers()
	t.updateWantPeersEvent()
}

// Don't call this before the info is available.
func (t *Torrent) bytesCompleted() int64 {
	if !t.haveInfo() {
		return 0
	}
	return t.info.TotalLength() - t.bytesLeft()
}

func (t *Torrent) SetInfoBytes(b []byte) (err error) {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	return t.setInfoBytes(b)
}

// Returns true if connection is removed from torrent.Conns.
func (t *Torrent) deleteConnection(c *connection) bool {
	for i0, _c := range t.conns {
		if _c != c {
			continue
		}
		i1 := len(t.conns) - 1
		if i0 != i1 {
			t.conns[i0] = t.conns[i1]
		}
		t.conns = t.conns[:i1]
		return true
	}
	return false
}

func (t *Torrent) dropConnection(c *connection) {
	t.cl.event.Broadcast()
	c.Close()
	if t.deleteConnection(c) {
		t.openNewConns()
	}
}

func (t *Torrent) wantPeers() bool {
	if t.closed.IsSet() {
		return false
	}
	if len(t.peers) > torrentPeersLowWater {
		return false
	}
	return t.needData() || t.seeding()
}

func (t *Torrent) updateWantPeersEvent() {
	if t.wantPeers() {
		t.wantPeersEvent.Set()
	} else {
		t.wantPeersEvent.Clear()
	}
}

// Returns whether the client should make effort to seed the torrent.
func (t *Torrent) seeding() bool {
	cl := t.cl
	if t.closed.IsSet() {
		return false
	}
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

// Adds and starts tracker scrapers for tracker URLs that aren't already
// running.
func (t *Torrent) startMissingTrackerScrapers() {
	if t.cl.config.DisableTrackers {
		return
	}
	for _, tier := range t.announceList() {
		for _, trackerURL := range tier {
			if _, ok := t.trackerAnnouncers[trackerURL]; ok {
				continue
			}
			newAnnouncer := &trackerScraper{
				url: trackerURL,
				t:   t,
			}
			if t.trackerAnnouncers == nil {
				t.trackerAnnouncers = make(map[string]*trackerScraper)
			}
			t.trackerAnnouncers[trackerURL] = newAnnouncer
			go newAnnouncer.Run()
		}
	}
}

// Returns an AnnounceRequest with fields filled out to defaults and current
// values.
func (t *Torrent) announceRequest() tracker.AnnounceRequest {
	return tracker.AnnounceRequest{
		Event:    tracker.None,
		NumWant:  -1,
		Port:     uint16(t.cl.incomingPeerPort()),
		PeerId:   t.cl.peerID,
		InfoHash: t.infoHash,
		Left:     t.bytesLeftAnnounce(),
	}
}

// Adds peers revealed in an announce until the announce ends, or we have
// enough peers.
func (t *Torrent) consumeDHTAnnounce(pvs <-chan dht.PeersValues) {
	cl := t.cl
	// Count all the unique addresses we got during this announce.
	allAddrs := make(map[string]struct{})
	for {
		select {
		case v, ok := <-pvs:
			if !ok {
				return
			}
			addPeers := make([]Peer, 0, len(v.Peers))
			for _, cp := range v.Peers {
				if cp.Port == 0 {
					// Can't do anything with this.
					continue
				}
				addPeers = append(addPeers, Peer{
					IP:     cp.IP[:],
					Port:   cp.Port,
					Source: peerSourceDHT,
				})
				key := (&net.UDPAddr{
					IP:   cp.IP[:],
					Port: cp.Port,
				}).String()
				allAddrs[key] = struct{}{}
			}
			cl.mu.Lock()
			t.addPeers(addPeers)
			numPeers := len(t.peers)
			cl.mu.Unlock()
			if numPeers >= torrentPeersHighWater {
				return
			}
		case <-t.closed.LockedChan(&cl.mu):
			return
		}
	}
}

func (t *Torrent) announceDHT(impliedPort bool) (err error) {
	cl := t.cl
	ps, err := cl.dHT.Announce(string(t.infoHash[:]), cl.incomingPeerPort(), impliedPort)
	if err != nil {
		return
	}
	t.consumeDHTAnnounce(ps.Peers)
	ps.Close()
	return
}

func (t *Torrent) dhtAnnouncer() {
	cl := t.cl
	for {
		select {
		case <-t.wantPeersEvent.LockedChan(&cl.mu):
		case <-t.closed.LockedChan(&cl.mu):
			return
		}
		err := t.announceDHT(true)
		if err == nil {
			cl.mu.Lock()
			t.numDHTAnnounces++
			cl.mu.Unlock()
		} else {
			log.Printf("error announcing %q to DHT: %s", t, err)
		}
		select {
		case <-t.closed.LockedChan(&cl.mu):
			return
		case <-time.After(5 * time.Minute):
		}
	}
}

func (t *Torrent) addPeers(peers []Peer) {
	for _, p := range peers {
		if t.cl.badPeerIPPort(p.IP, p.Port) {
			continue
		}
		t.addPeer(p)
	}
}

func (t *Torrent) Stats() TorrentStats {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	return t.stats
}

// Returns true if the connection is added.
func (t *Torrent) addConnection(c *connection) bool {
	if t.cl.closed.IsSet() {
		return false
	}
	if !t.wantConns() {
		return false
	}
	for _, c0 := range t.conns {
		if c.PeerID == c0.PeerID {
			// Already connected to a client with that ID.
			duplicateClientConns.Add(1)
			return false
		}
	}
	if len(t.conns) >= t.maxEstablishedConns {
		c := t.worstBadConn()
		if c == nil {
			return false
		}
		if t.cl.config.Debug && missinggo.CryHeard() {
			log.Printf("%s: dropping connection to make room for new one:\n    %s", t, c)
		}
		c.Close()
		t.deleteConnection(c)
	}
	if len(t.conns) >= t.maxEstablishedConns {
		panic(len(t.conns))
	}
	t.conns = append(t.conns, c)
	if c.t != nil {
		panic("connection already associated with a torrent")
	}
	// Reconcile bytes transferred before connection was associated with a
	// torrent.
	t.stats.wroteBytes(c.stats.BytesWritten)
	t.stats.readBytes(c.stats.BytesRead)
	c.t = t
	return true
}

func (t *Torrent) wantConns() bool {
	if t.closed.IsSet() {
		return false
	}
	if !t.seeding() && !t.needData() {
		return false
	}
	if len(t.conns) < t.maxEstablishedConns {
		return true
	}
	return t.worstBadConn() != nil
}

func (t *Torrent) SetMaxEstablishedConns(max int) (oldMax int) {
	t.cl.mu.Lock()
	defer t.cl.mu.Unlock()
	oldMax = t.maxEstablishedConns
	t.maxEstablishedConns = max
	wcs := slices.HeapInterface(append([]*connection(nil), t.conns...), worseConn)
	for len(t.conns) > t.maxEstablishedConns && wcs.Len() > 0 {
		t.dropConnection(wcs.Pop().(*connection))
	}
	t.openNewConns()
	return oldMax
}

func (t *Torrent) mu() missinggo.RWLocker {
	return &t.cl.mu
}
