package torrent

import (
	"testing"
	"time"

	"github.com/bradfitz/iter"
	"github.com/stretchr/testify/assert"

	"github.com/anacrolix/torrent/internal/pieceordering"
	"github.com/anacrolix/torrent/peer_protocol"
)

func TestCancelRequestOptimized(t *testing.T) {
	c := &connection{
		PeerMaxRequests: 1,
		PeerPieces:      []bool{false, true},
		post:            make(chan peer_protocol.Message),
		writeCh:         make(chan []byte),
	}
	if len(c.Requests) != 0 {
		t.FailNow()
	}
	// Keepalive timeout of 0 works because I'm just that good.
	go c.writeOptimizer(0 * time.Millisecond)
	c.Request(newRequest(1, 2, 3))
	if len(c.Requests) != 1 {
		t.Fatal("request was not posted")
	}
	// Posting this message should removing the pending Request.
	if !c.Cancel(newRequest(1, 2, 3)) {
		t.Fatal("request was not found")
	}
	// Check that the write optimization has filtered out the Request message.
	for _, b := range []string{
		// The initial request triggers an Interested message.
		"\x00\x00\x00\x01\x02",
		// Let a keep-alive through to verify there were no pending messages.
		"\x00\x00\x00\x00",
	} {
		bb := string(<-c.writeCh)
		if b != bb {
			t.Fatalf("received message %q is not expected: %q", bb, b)
		}
	}
	close(c.post)
	// Drain the write channel until it closes.
	for b := range c.writeCh {
		bs := string(b)
		if bs != "\x00\x00\x00\x00" {
			t.Fatal("got unexpected non-keepalive")
		}
	}
}

func pieceOrderingAsSlice(po *pieceordering.Instance) (ret []int) {
	for e := po.First(); e != nil; e = e.Next() {
		ret = append(ret, e.Piece())
	}
	return
}

func testRequestOrder(expected []int, ro *pieceordering.Instance, t *testing.T) {
	assert.EqualValues(t, pieceOrderingAsSlice(ro), expected)
}

// Tests the request ordering based on a connections priorities.
func TestPieceRequestOrder(t *testing.T) {
	c := connection{
		pieceRequestOrder: pieceordering.New(),
		piecePriorities:   []int{1, 4, 0, 3, 2},
	}
	testRequestOrder(nil, c.pieceRequestOrder, t)
	c.pendPiece(2, PiecePriorityNone, nil)
	testRequestOrder(nil, c.pieceRequestOrder, t)
	c.pendPiece(1, PiecePriorityNormal, nil)
	c.pendPiece(2, PiecePriorityNormal, nil)
	testRequestOrder([]int{2, 1}, c.pieceRequestOrder, t)
	c.pendPiece(0, PiecePriorityNormal, nil)
	testRequestOrder([]int{2, 0, 1}, c.pieceRequestOrder, t)
	c.pendPiece(1, PiecePriorityReadahead, nil)
	testRequestOrder([]int{1, 2, 0}, c.pieceRequestOrder, t)
	c.pendPiece(4, PiecePriorityNow, nil)
	// now(4), r(1), normal(0, 2)
	testRequestOrder([]int{4, 1, 2, 0}, c.pieceRequestOrder, t)
	c.pendPiece(2, PiecePriorityReadahead, nil)
	// N(4), R(1, 2), N(0)
	testRequestOrder([]int{4, 2, 1, 0}, c.pieceRequestOrder, t)
	c.pendPiece(1, PiecePriorityNow, nil)
	// now(4, 1), readahead(2), normal(0)
	// in the same order, the keys will be: -15+6, -15+12, -5, 1
	// so we test that a very low priority (for this connection), "now"
	// piece has been placed after a readahead piece.
	testRequestOrder([]int{4, 2, 1, 0}, c.pieceRequestOrder, t)
	// Note this intentially sets to None a piece that's not in the order.
	for i := range iter.N(5) {
		c.pendPiece(i, PiecePriorityNone, nil)
	}
	testRequestOrder(nil, c.pieceRequestOrder, t)
}
