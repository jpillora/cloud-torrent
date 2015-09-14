package torrent

import (
	"container/heap"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/anacrolix/missinggo"
	"github.com/bradfitz/iter"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/data"
	"github.com/anacrolix/torrent/metainfo"
	pp "github.com/anacrolix/torrent/peer_protocol"
	"github.com/anacrolix/torrent/tracker"
)

func (t *torrent) pieceNumPendingBytes(index int) (count pp.Integer) {
	if t.pieceComplete(index) {
		return 0
	}
	piece := t.Pieces[index]
	pieceLength := t.pieceLength(index)
	if !piece.EverHashed {
		return pieceLength
	}
	for i, pending := range piece.PendingChunkSpecs {
		if pending {
			count += chunkIndexSpec(i, pieceLength, t.chunkSize).Length
		}
	}
	return
}

type peersKey struct {
	IPBytes string
	Port    int
}

// Data maintains per-piece persistent state.
type StatefulData interface {
	data.Data
	// We believe the piece data will pass a hash check.
	PieceCompleted(index int) error
	// Returns true if the piece is complete.
	PieceComplete(index int) bool
}

// Is not aware of Client. Maintains state of torrent for with-in a Client.
type torrent struct {
	stateMu sync.Mutex
	closing chan struct{}

	// Closed when no more network activity is desired. This includes
	// announcing, and communicating with peers.
	ceasingNetworking chan struct{}

	InfoHash  InfoHash
	Pieces    []*piece
	chunkSize pp.Integer
	// Chunks that are wanted before all others. This is for
	// responsive/streaming readers that want to unblock ASAP.
	urgent map[request]struct{}
	// Total length of the torrent in bytes. Stored because it's not O(1) to
	// get this from the info dict.
	length int64

	data StatefulData

	// The info dict. Nil if we don't have it (yet).
	Info *metainfo.Info
	// Active peer connections, running message stream loops.
	Conns []*connection
	// Set of addrs to which we're attempting to connect. Connections are
	// half-open until all handshakes are completed.
	HalfOpen map[string]struct{}

	// Reserve of peers to connect to. A peer can be both here and in the
	// active connections if were told about the peer after connecting with
	// them. That encourages us to reconnect to peers that are well known.
	Peers     map[peersKey]Peer
	wantPeers sync.Cond

	// BEP 12 Multitracker Metadata Extension. The tracker.Client instances
	// mirror their respective URLs from the announce-list metainfo key.
	Trackers [][]tracker.Client
	// Name used if the info name isn't available.
	DisplayName string
	// The bencoded bytes of the info dict.
	MetaData []byte
	// Each element corresponds to the 16KiB metadata pieces. If true, we have
	// received that piece.
	metadataHave []bool

	// Closed when .Info is set.
	gotMetainfo chan struct{}
}

func (t *torrent) pieceComplete(piece int) bool {
	// TODO: This is called when setting metadata, and before storage is
	// assigned, which doesn't seem right.
	return t.data != nil && t.data.PieceComplete(piece)
}

func (t *torrent) numConnsUnchoked() (num int) {
	for _, c := range t.Conns {
		if !c.PeerChoked {
			num++
		}
	}
	return
}

// There's a connection to that address already.
func (t *torrent) addrActive(addr string) bool {
	if _, ok := t.HalfOpen[addr]; ok {
		return true
	}
	for _, c := range t.Conns {
		if c.remoteAddr().String() == addr {
			return true
		}
	}
	return false
}

func (t *torrent) worstConns(cl *Client) (wcs *worstConns) {
	wcs = &worstConns{
		c:  make([]*connection, 0, len(t.Conns)),
		t:  t,
		cl: cl,
	}
	for _, c := range t.Conns {
		select {
		case <-c.closing:
		default:
			wcs.c = append(wcs.c, c)
		}
	}
	return
}

func (t *torrent) ceaseNetworking() {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	select {
	case <-t.ceasingNetworking:
		return
	default:
	}
	close(t.ceasingNetworking)
	for _, c := range t.Conns {
		c.Close()
	}
}

func (t *torrent) addPeer(p Peer) {
	t.Peers[peersKey{string(p.IP), p.Port}] = p
}

func (t *torrent) invalidateMetadata() {
	t.MetaData = nil
	t.metadataHave = nil
	t.Info = nil
}

func (t *torrent) saveMetadataPiece(index int, data []byte) {
	if t.haveInfo() {
		return
	}
	if index >= len(t.metadataHave) {
		log.Printf("%s: ignoring metadata piece %d", t, index)
		return
	}
	copy(t.MetaData[(1<<14)*index:], data)
	t.metadataHave[index] = true
}

func (t *torrent) metadataPieceCount() int {
	return (len(t.MetaData) + (1 << 14) - 1) / (1 << 14)
}

func (t *torrent) haveMetadataPiece(piece int) bool {
	if t.haveInfo() {
		return (1<<14)*piece < len(t.MetaData)
	} else {
		return piece < len(t.metadataHave) && t.metadataHave[piece]
	}
}

func (t *torrent) metadataSizeKnown() bool {
	return t.MetaData != nil
}

func (t *torrent) metadataSize() int {
	return len(t.MetaData)
}

func infoPieceHashes(info *metainfo.Info) (ret []string) {
	for i := 0; i < len(info.Pieces); i += 20 {
		ret = append(ret, string(info.Pieces[i:i+20]))
	}
	return
}

// Called when metadata for a torrent becomes available.
func (t *torrent) setMetadata(md *metainfo.Info, infoBytes []byte, eventLocker sync.Locker) (err error) {
	err = validateInfo(md)
	if err != nil {
		err = fmt.Errorf("bad info: %s", err)
		return
	}
	t.Info = md
	t.length = 0
	for _, f := range t.Info.UpvertedFiles() {
		t.length += f.Length
	}
	t.MetaData = infoBytes
	t.metadataHave = nil
	for _, hash := range infoPieceHashes(md) {
		piece := &piece{}
		piece.Event.L = eventLocker
		piece.noPendingWrites.L = &piece.pendingWritesMutex
		missinggo.CopyExact(piece.Hash[:], hash)
		t.Pieces = append(t.Pieces, piece)
	}
	for _, conn := range t.Conns {
		t.initRequestOrdering(conn)
		if err := conn.setNumPieces(t.numPieces()); err != nil {
			log.Printf("closing connection: %s", err)
			conn.Close()
		}
	}
	return
}

func (t *torrent) setStorage(td data.Data) (err error) {
	if c, ok := t.data.(io.Closer); ok {
		c.Close()
	}
	if sd, ok := td.(StatefulData); ok {
		t.data = sd
	} else {
		t.data = &statelessDataWrapper{td, make([]bool, t.Info.NumPieces())}
	}
	return
}

func (t *torrent) haveAllMetadataPieces() bool {
	if t.haveInfo() {
		return true
	}
	if t.metadataHave == nil {
		return false
	}
	for _, have := range t.metadataHave {
		if !have {
			return false
		}
	}
	return true
}

func (t *torrent) setMetadataSize(bytes int64, cl *Client) {
	if t.haveInfo() {
		// We already know the correct metadata size.
		return
	}
	if bytes <= 0 || bytes > 10000000 { // 10MB, pulled from my ass.
		log.Printf("received bad metadata size: %d", bytes)
		return
	}
	if t.MetaData != nil && len(t.MetaData) == int(bytes) {
		return
	}
	t.MetaData = make([]byte, bytes)
	t.metadataHave = make([]bool, (bytes+(1<<14)-1)/(1<<14))
	for _, c := range t.Conns {
		cl.requestPendingMetadata(t, c)
	}

}

// The current working name for the torrent. Either the name in the info dict,
// or a display name given such as by the dn value in a magnet link, or "".
func (t *torrent) Name() string {
	if t.haveInfo() {
		return t.Info.Name
	}
	return t.DisplayName
}

func (t *torrent) pieceState(index int) (ret PieceState) {
	p := t.Pieces[index]
	ret.Priority = p.Priority
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

func (t *torrent) metadataPieceSize(piece int) int {
	return metadataPieceSize(len(t.MetaData), piece)
}

func (t *torrent) newMetadataExtensionMessage(c *connection, msgType int, piece int, data []byte) pp.Message {
	d := map[string]int{
		"msg_type": msgType,
		"piece":    piece,
	}
	if data != nil {
		d["total_size"] = len(t.MetaData)
	}
	p, err := bencode.Marshal(d)
	if err != nil {
		panic(err)
	}
	return pp.Message{
		Type:            pp.Extended,
		ExtendedID:      byte(c.PeerExtensionIDs["ut_metadata"]),
		ExtendedPayload: append(p, data...),
	}
}

func (t *torrent) pieceStateRuns() (ret []PieceStateRun) {
	rle := missinggo.NewRunLengthEncoder(func(el interface{}, count uint64) {
		ret = append(ret, PieceStateRun{
			PieceState: el.(PieceState),
			Length:     int(count),
		})
	})
	for index := range t.Pieces {
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

func (t *torrent) writeStatus(w io.Writer, cl *Client) {
	fmt.Fprintf(w, "Infohash: %x\n", t.InfoHash)
	fmt.Fprintf(w, "Metadata length: %d\n", t.metadataSize())
	if !t.haveInfo() {
		fmt.Fprintf(w, "Metadata have: ")
		for _, h := range t.metadataHave {
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
	if t.haveInfo() {
		fmt.Fprint(w, "Pieces:")
		for _, psr := range t.pieceStateRuns() {
			w.Write([]byte(" "))
			w.Write([]byte(pieceStateRunStatusChars(psr)))
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "Urgent:")
	for req := range t.urgent {
		fmt.Fprintf(w, " %v", req)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Trackers: ")
	for _, tier := range t.Trackers {
		for _, tr := range tier {
			fmt.Fprintf(w, "%q ", tr.String())
		}
	}
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Pending peers: %d\n", len(t.Peers))
	fmt.Fprintf(w, "Half open: %d\n", len(t.HalfOpen))
	fmt.Fprintf(w, "Active peers: %d\n", len(t.Conns))
	sort.Sort(&worstConns{
		c:  t.Conns,
		t:  t,
		cl: cl,
	})
	for i, c := range t.Conns {
		fmt.Fprintf(w, "%2d. ", i+1)
		c.WriteStatus(w, t)
	}
}

func (t *torrent) String() string {
	s := t.Name()
	if s == "" {
		s = fmt.Sprintf("%x", t.InfoHash)
	}
	return s
}

func (t *torrent) haveInfo() bool {
	return t != nil && t.Info != nil
}

// TODO: Include URIs that weren't converted to tracker clients.
func (t *torrent) announceList() (al [][]string) {
	for _, tier := range t.Trackers {
		var l []string
		for _, tr := range tier {
			l = append(l, tr.URL())
		}
		al = append(al, l)
	}
	return
}

// Returns a run-time generated MetaInfo that includes the info bytes and
// announce-list as currently known to the client.
func (t *torrent) MetaInfo() *metainfo.MetaInfo {
	if t.MetaData == nil {
		panic("info bytes not set")
	}
	return &metainfo.MetaInfo{
		Info: metainfo.InfoEx{
			Info:  *t.Info,
			Bytes: t.MetaData,
		},
		CreationDate: time.Now().Unix(),
		Comment:      "dynamic metainfo from client",
		CreatedBy:    "go.torrent",
		AnnounceList: t.announceList(),
	}
}

func (t *torrent) bytesLeft() (left int64) {
	if !t.haveInfo() {
		return -1
	}
	for i := 0; i < t.numPieces(); i++ {
		left += int64(t.pieceNumPendingBytes(i))
	}
	return
}

func (t *torrent) piecePartiallyDownloaded(index int) bool {
	pendingBytes := t.pieceNumPendingBytes(index)
	return pendingBytes != 0 && pendingBytes != t.pieceLength(index)
}

func numChunksForPiece(chunkSize int, pieceSize int) int {
	return (pieceSize + chunkSize - 1) / chunkSize
}

func (t *torrent) usualPieceSize() int {
	return int(t.Info.PieceLength)
}

func (t *torrent) lastPieceSize() int {
	return int(t.pieceLength(t.numPieces() - 1))
}

func (t *torrent) numPieces() int {
	return t.Info.NumPieces()
}

func (t *torrent) numPiecesCompleted() (num int) {
	for i := range iter.N(t.Info.NumPieces()) {
		if t.pieceComplete(i) {
			num++
		}
	}
	return
}

func (t *torrent) Length() int64 {
	return t.length
}

func (t *torrent) isClosed() bool {
	select {
	case <-t.closing:
		return true
	default:
		return false
	}
}

func (t *torrent) close() (err error) {
	if t.isClosed() {
		return
	}
	t.ceaseNetworking()
	close(t.closing)
	if c, ok := t.data.(io.Closer); ok {
		c.Close()
	}
	for _, conn := range t.Conns {
		conn.Close()
	}
	return
}

func (t *torrent) requestOffset(r request) int64 {
	return torrentRequestOffset(t.Length(), int64(t.usualPieceSize()), r)
}

// Return the request that would include the given offset into the torrent
// data. Returns !ok if there is no such request.
func (t *torrent) offsetRequest(off int64) (req request, ok bool) {
	return torrentOffsetRequest(t.Length(), t.Info.PieceLength, int64(t.chunkSize), off)
}

func (t *torrent) writeChunk(piece int, begin int64, data []byte) (err error) {
	n, err := t.data.WriteAt(data, int64(piece)*t.Info.PieceLength+begin)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return
}

func (t *torrent) bitfield() (bf []bool) {
	for _, p := range t.Pieces {
		// TODO: Check this logic.
		bf = append(bf, p.EverHashed && p.numPendingChunks() == 0)
	}
	return
}

func (t *torrent) validOutgoingRequest(r request) bool {
	if r.Index >= pp.Integer(t.Info.NumPieces()) {
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

func (t *torrent) pieceChunks(piece int) (css []chunkSpec) {
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

func (t *torrent) pendAllChunkSpecs(pieceIndex int) {
	piece := t.Pieces[pieceIndex]
	if piece.PendingChunkSpecs == nil {
		// Allocate to exact size.
		piece.PendingChunkSpecs = make([]bool, (t.pieceLength(pieceIndex)+t.chunkSize-1)/t.chunkSize)
	}
	// Pend all the chunks.
	pcss := piece.PendingChunkSpecs
	for i := range pcss {
		pcss[i] = true
	}
	return
}

type Peer struct {
	Id     [20]byte
	IP     net.IP
	Port   int
	Source peerSource
	// Peer is known to support encryption.
	SupportsEncryption bool
}

func (t *torrent) pieceLength(piece int) (len_ pp.Integer) {
	if int(piece) == t.numPieces()-1 {
		len_ = pp.Integer(t.Length() % t.Info.PieceLength)
	}
	if len_ == 0 {
		len_ = pp.Integer(t.Info.PieceLength)
	}
	return
}

func (t *torrent) hashPiece(piece pp.Integer) (ps pieceSum) {
	hash := pieceHash.New()
	p := t.Pieces[piece]
	p.pendingWritesMutex.Lock()
	for p.pendingWrites != 0 {
		p.noPendingWrites.Wait()
	}
	p.pendingWritesMutex.Unlock()
	t.data.WriteSectionTo(hash, int64(piece)*t.Info.PieceLength, t.Info.PieceLength)
	missinggo.CopyExact(ps[:], hash.Sum(nil))
	return
}

func (t *torrent) haveAllPieces() bool {
	if !t.haveInfo() {
		return false
	}
	for i := range t.Pieces {
		if !t.pieceComplete(i) {
			return false
		}
	}
	return true
}

func (me *torrent) haveAnyPieces() bool {
	for i := range me.Pieces {
		if me.pieceComplete(i) {
			return true
		}
	}
	return false
}

func (t *torrent) havePiece(index int) bool {
	return t.haveInfo() && t.pieceComplete(index)
}

func (t *torrent) haveChunk(r request) bool {
	if !t.haveInfo() {
		return false
	}
	if t.pieceComplete(int(r.Index)) {
		return true
	}
	p := t.Pieces[r.Index]
	if p.PendingChunkSpecs == nil {
		return false
	}
	return !p.pendingChunk(r.chunkSpec, t.chunkSize)
}

func chunkIndex(cs chunkSpec, chunkSize pp.Integer) int {
	return int(cs.Begin / chunkSize)
}

// TODO: This should probably be called wantPiece.
func (t *torrent) wantChunk(r request) bool {
	if !t.wantPiece(int(r.Index)) {
		return false
	}
	if t.Pieces[r.Index].pendingChunk(r.chunkSpec, t.chunkSize) {
		return true
	}
	_, ok := t.urgent[r]
	return ok
}

func (t *torrent) urgentChunkInPiece(piece int) bool {
	p := pp.Integer(piece)
	for req := range t.urgent {
		if req.Index == p {
			return true
		}
	}
	return false
}

// TODO: This should be called wantPieceIndex.
func (t *torrent) wantPiece(index int) bool {
	if !t.haveInfo() {
		return false
	}
	p := t.Pieces[index]
	if p.QueuedForHash {
		return false
	}
	if p.Hashing {
		return false
	}
	if p.Priority == PiecePriorityNone {
		if !t.urgentChunkInPiece(index) {
			return false
		}
	}
	// Put piece complete check last, since it's the slowest as it can involve
	// calling out into external data stores.
	return !t.pieceComplete(index)
}

func (t *torrent) connHasWantedPieces(c *connection) bool {
	return c.pieceRequestOrder != nil && c.pieceRequestOrder.First() != nil
}

func (t *torrent) extentPieces(off, _len int64) (pieces []int) {
	for i := off / int64(t.usualPieceSize()); i*int64(t.usualPieceSize()) < off+_len; i++ {
		pieces = append(pieces, int(i))
	}
	return
}

func (t *torrent) worstBadConn(cl *Client) *connection {
	wcs := t.worstConns(cl)
	heap.Init(wcs)
	for wcs.Len() != 0 {
		c := heap.Pop(wcs).(*connection)
		if c.UnwantedChunksReceived >= 6 && c.UnwantedChunksReceived > c.UsefulChunksReceived {
			return c
		}
		if wcs.Len() >= (socketsPerTorrent+1)/2 {
			// Give connections 1 minute to prove themselves.
			if time.Since(c.completedHandshake) > time.Minute {
				return c
			}
		}
	}
	return nil
}
