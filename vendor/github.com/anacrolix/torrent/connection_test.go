package torrent

import (
	"container/list"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/missinggo/bitmap"
	"github.com/anacrolix/missinggo/pubsub"
	"github.com/bradfitz/iter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/torrent/metainfo"
	pp "github.com/anacrolix/torrent/peer_protocol"
	"github.com/anacrolix/torrent/storage"
)

func TestCancelRequestOptimized(t *testing.T) {
	r, w := io.Pipe()
	c := &connection{
		PeerMaxRequests: 1,
		peerPieces: func() bitmap.Bitmap {
			var bm bitmap.Bitmap
			bm.Set(1, true)
			return bm
		}(),
		w:    w,
		conn: new(net.TCPConn),
		// For the locks
		t: &Torrent{cl: &Client{}},
	}
	assert.Len(t, c.Requests, 0)
	c.Request(newRequest(1, 2, 3))
	require.Len(t, c.Requests, 1)
	// Posting this message should removing the pending Request.
	require.True(t, c.Cancel(newRequest(1, 2, 3)))
	assert.Len(t, c.Requests, 0)
	// Check that write optimization filters out the Request, due to the
	// Cancel. We should have received an Interested, due to the initial
	// request, and then keep-alives until we close the connection.
	go c.writer(0)
	b := make([]byte, 9)
	n, err := io.ReadFull(r, b)
	require.NoError(t, err)
	require.EqualValues(t, len(b), n)
	require.EqualValues(t, "\x00\x00\x00\x01\x02"+"\x00\x00\x00\x00", string(b))
	time.Sleep(time.Millisecond)
	c.mu().Lock()
	c.Close()
	c.mu().Unlock()
	w.Close()
	b, err = ioutil.ReadAll(r)
	require.NoError(t, err)
	// A single keep-alive will have gone through, as writer would be stuck
	// trying to flush it, and then promptly close.
	require.EqualValues(t, "\x00\x00\x00\x00", string(b))
}

// Ensure that no race exists between sending a bitfield, and a subsequent
// Have that would potentially alter it.
func TestSendBitfieldThenHave(t *testing.T) {
	r, w := io.Pipe()
	c := &connection{
		t: &Torrent{
			cl: &Client{},
		},
		r: r,
		w: w,
		outgoingUnbufferedMessages: list.New(),
	}
	go c.writer(time.Minute)
	c.mu().Lock()
	c.Bitfield([]bool{false, true, false})
	c.mu().Unlock()
	c.mu().Lock()
	c.Have(2)
	c.mu().Unlock()
	b := make([]byte, 15)
	n, err := io.ReadFull(r, b)
	c.mu().Lock()
	// This will cause connection.writer to terminate.
	c.closed.Set()
	c.mu().Unlock()
	require.NoError(t, err)
	require.EqualValues(t, 15, n)
	// Here we see that the bitfield doesn't have piece 2 set, as that should
	// arrive in the following Have message.
	require.EqualValues(t, "\x00\x00\x00\x02\x05@\x00\x00\x00\x05\x04\x00\x00\x00\x02", string(b))
}

type torrentStorage struct {
	writeSem sync.Mutex
}

func (me *torrentStorage) Close() error { return nil }

func (me *torrentStorage) Piece(mp metainfo.Piece) storage.PieceImpl {
	return me
}

func (me *torrentStorage) GetIsComplete() bool {
	return false
}

func (me *torrentStorage) MarkComplete() error {
	return nil
}

func (me *torrentStorage) MarkNotComplete() error {
	return nil
}

func (me *torrentStorage) ReadAt([]byte, int64) (int, error) {
	panic("shouldn't be called")
}

func (me *torrentStorage) WriteAt(b []byte, _ int64) (int, error) {
	if len(b) != defaultChunkSize {
		panic(len(b))
	}
	me.writeSem.Unlock()
	return len(b), nil
}

func BenchmarkConnectionMainReadLoop(b *testing.B) {
	cl := &Client{}
	ts := &torrentStorage{}
	t := &Torrent{
		cl: cl,
		info: &metainfo.Info{
			Pieces:      make([]byte, 20),
			Length:      1 << 20,
			PieceLength: 1 << 20,
		},
		storage:           &storage.Torrent{ts},
		pieceStateChanges: pubsub.NewPubSub(),
	}
	t.setChunkSize(defaultChunkSize)
	t.makePieces()
	t.pendingPieces.Add(0)
	r, w := io.Pipe()
	cn := &connection{
		t: t,
		r: r,
	}
	mrlErr := make(chan error)
	cl.mu.Lock()
	go func() {
		err := cn.mainReadLoop()
		if err != nil {
			mrlErr <- err
		}
		close(mrlErr)
	}()
	msg := pp.Message{
		Type:  pp.Piece,
		Piece: make([]byte, defaultChunkSize),
	}
	wb, err := msg.MarshalBinary()
	require.NoError(b, err)
	b.SetBytes(int64(len(msg.Piece)))
	ts.writeSem.Lock()
	for range iter.N(b.N) {
		cl.mu.Lock()
		t.pieces[0].DirtyChunks.Clear()
		cl.mu.Unlock()
		n, err := w.Write(wb)
		require.NoError(b, err)
		require.EqualValues(b, len(wb), n)
		ts.writeSem.Lock()
	}
	w.Close()
	require.NoError(b, <-mrlErr)
	require.EqualValues(b, b.N, cn.UsefulChunksReceived)
}

func TestConnectionReceiveBadChunkIndex(t *testing.T) {
	cn := connection{
		t: &Torrent{},
	}
	require.False(t, cn.t.haveInfo())
	assert.NotPanics(t, func() { cn.receiveChunk(&pp.Message{}) })
	cn.t.info = &metainfo.Info{}
	require.True(t, cn.t.haveInfo())
	assert.NotPanics(t, func() { cn.receiveChunk(&pp.Message{}) })
}
