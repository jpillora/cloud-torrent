package torrent

import (
	"errors"
	"io"
	"log"
	"os"
	"sync"

	"github.com/anacrolix/missinggo"
	"golang.org/x/net/context"
)

// Piece range by piece index, [begin, end).
type pieceRange struct {
	begin, end int
}

// Accesses Torrent data via a Client. Reads block until the data is
// available. Seeks and readahead also drive Client behaviour.
type Reader struct {
	t          *Torrent
	responsive bool
	// Ensure operations that change the position are exclusive, like Read()
	// and Seek().
	opMu sync.Mutex

	// Required when modifying pos and readahead, or reading them without
	// opMu.
	mu        sync.Locker
	pos       int64
	readahead int64
	// The cached piece range this reader wants downloaded. The zero value
	// corresponds to nothing. We cache this so that changes can be detected,
	// and bubbled up to the Torrent only as required.
	pieces pieceRange
}

var _ io.ReadCloser = &Reader{}

// Don't wait for pieces to complete and be verified. Read calls return as
// soon as they can when the underlying chunks become available.
func (r *Reader) SetResponsive() {
	r.responsive = true
}

// Configure the number of bytes ahead of a read that should also be
// prioritized in preparation for further reads.
func (r *Reader) SetReadahead(readahead int64) {
	r.mu.Lock()
	r.readahead = readahead
	r.mu.Unlock()
	r.t.cl.mu.Lock()
	defer r.t.cl.mu.Unlock()
	r.posChanged()
}

func (r *Reader) readable(off int64) (ret bool) {
	if r.t.closed.IsSet() {
		return true
	}
	req, ok := r.t.offsetRequest(off)
	if !ok {
		panic(off)
	}
	if r.responsive {
		return r.t.haveChunk(req)
	}
	return r.t.pieceComplete(int(req.Index))
}

// How many bytes are available to read. Max is the most we could require.
func (r *Reader) available(off, max int64) (ret int64) {
	for max > 0 {
		req, ok := r.t.offsetRequest(off)
		if !ok {
			break
		}
		if !r.t.haveChunk(req) {
			break
		}
		len1 := int64(req.Length) - (off - r.t.requestOffset(req))
		max -= len1
		ret += len1
		off += len1
	}
	// Ensure that ret hasn't exceeded our original max.
	if max < 0 {
		ret += max
	}
	return
}

func (r *Reader) tickleClient() {
	r.t.readersChanged()
}

func (r *Reader) waitReadable(off int64) {
	// We may have been sent back here because we were told we could read but
	// it failed.
	r.tickleClient()
	r.t.cl.event.Wait()
}

// Calculates the pieces this reader wants downloaded, ignoring the cached
// value at r.pieces.
func (r *Reader) piecesUncached() (ret pieceRange) {
	ra := r.readahead
	if ra < 1 {
		ra = 1
	}
	ret.begin, ret.end = r.t.byteRegionPieces(r.pos, ra)
	return
}

func (r *Reader) Read(b []byte) (n int, err error) {
	return r.ReadContext(b, context.Background())
}

func (r *Reader) ReadContext(b []byte, ctx context.Context) (n int, err error) {
	// This is set under the Client lock if the Context is canceled.
	var ctxErr error
	if ctx.Done() != nil {
		ctx, cancel := context.WithCancel(ctx)
		// Abort the goroutine when the function returns.
		defer cancel()
		go func() {
			<-ctx.Done()
			r.t.cl.mu.Lock()
			ctxErr = ctx.Err()
			r.t.cl.event.Broadcast()
			r.t.cl.mu.Unlock()
		}()
	}
	// Hmmm, if a Read gets stuck, this means you can't change position for
	// other purposes. That seems reasonable, but unusual.
	r.opMu.Lock()
	defer r.opMu.Unlock()
	for len(b) != 0 {
		var n1 int
		n1, err = r.readOnceAt(b, r.pos, &ctxErr)
		if n1 == 0 {
			if err == nil {
				panic("expected error")
			}
			break
		}
		b = b[n1:]
		n += n1
		r.mu.Lock()
		r.pos += int64(n1)
		r.posChanged()
		r.mu.Unlock()
	}
	if r.pos >= r.t.length {
		err = io.EOF
	} else if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return
}

// Wait until some data should be available to read. Tickles the client if it
// isn't. Returns how much should be readable without blocking.
func (r *Reader) waitAvailable(pos, wanted int64, ctxErr *error) (avail int64) {
	r.t.cl.mu.Lock()
	defer r.t.cl.mu.Unlock()
	for !r.readable(pos) && *ctxErr == nil {
		r.waitReadable(pos)
	}
	return r.available(pos, wanted)
}

// Performs at most one successful read to torrent storage.
func (r *Reader) readOnceAt(b []byte, pos int64, ctxErr *error) (n int, err error) {
	if pos >= r.t.length {
		err = io.EOF
		return
	}
	for {
		avail := r.waitAvailable(pos, int64(len(b)), ctxErr)
		if avail == 0 {
			if r.t.closed.IsSet() {
				err = errors.New("torrent closed")
				return
			}
			if *ctxErr != nil {
				err = *ctxErr
				return
			}
		}
		b1 := b[:avail]
		pi := int(pos / r.t.Info().PieceLength)
		ip := r.t.Info().Piece(pi)
		po := pos % r.t.Info().PieceLength
		missinggo.LimitLen(&b1, ip.Length()-po)
		n, err = r.t.readAt(b1, pos)
		if n != 0 {
			err = nil
			return
		}
		log.Printf("error reading torrent %q piece %d offset %d, %d bytes: %s", r.t, pi, po, len(b1), err)
		r.t.cl.mu.Lock()
		r.t.updateAllPieceCompletions()
		r.t.updatePiecePriorities()
		r.t.cl.mu.Unlock()
	}
}

func (r *Reader) Close() error {
	r.t.deleteReader(r)
	r.t = nil
	return nil
}

func (r *Reader) posChanged() {
	p := r.piecesUncached()
	if p == r.pieces {
		return
	}
	r.pieces = p
	r.t.readersChanged()
}

func (r *Reader) Seek(off int64, whence int) (ret int64, err error) {
	r.opMu.Lock()
	defer r.opMu.Unlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	switch whence {
	case os.SEEK_SET:
		r.pos = off
	case os.SEEK_CUR:
		r.pos += off
	case os.SEEK_END:
		r.pos = r.t.info.TotalLength() + off
	default:
		err = errors.New("bad whence")
	}
	ret = r.pos

	r.posChanged()
	return
}

func (r *Reader) Torrent() *Torrent {
	return r.t
}
