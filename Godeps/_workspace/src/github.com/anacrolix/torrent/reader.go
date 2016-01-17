package torrent

import (
	"errors"
	"io"
	"os"
)

// Accesses torrent data via a client.
type Reader struct {
	t          *Torrent
	pos        int64
	responsive bool
	readahead  int64
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
	r.readahead = readahead
}

func (r *Reader) raisePriorities(off int64, n int) {
	if r.responsive {
		r.t.cl.addUrgentRequests(r.t.torrent, off, n)
	}
	if !r.responsive || r.readahead != 0 {
		r.t.cl.readRaisePiecePriorities(r.t.torrent, off, int64(n)+r.readahead)
	}
}

func (r *Reader) readable(off int64) (ret bool) {
	// log.Println("readable", off)
	// defer func() {
	// 	log.Println("readable", ret)
	// }()
	if r.t.torrent.isClosed() {
		return true
	}
	req, ok := r.t.torrent.offsetRequest(off)
	if !ok {
		panic(off)
	}
	if r.responsive {
		return r.t.torrent.haveChunk(req)
	}
	return r.t.torrent.pieceComplete(int(req.Index))
}

// How many bytes are available to read. Max is the most we could require.
func (r *Reader) available(off, max int64) (ret int64) {
	for max > 0 {
		req, ok := r.t.torrent.offsetRequest(off)
		if !ok {
			break
		}
		if !r.t.torrent.haveChunk(req) {
			break
		}
		len1 := int64(req.Length) - (off - r.t.torrent.requestOffset(req))
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

func (r *Reader) waitReadable(off int64) {
	r.t.cl.event.Wait()
}

func (r *Reader) ReadAt(b []byte, off int64) (n int, err error) {
	for {
		var n1 int
		n1, err = r.readAt(b, off)
		n += n1
		b = b[n1:]
		off += int64(n1)
		if err != nil || len(b) == 0 || n1 == 0 {
			return
		}
	}
}

func (r *Reader) Read(b []byte) (n int, err error) {
	n, err = r.readAt(b, r.pos)
	r.pos += int64(n)
	return
}

// Must only return EOF at the end of the torrent.
func (r *Reader) readAt(b []byte, pos int64) (n int, err error) {
	// defer func() {
	// 	log.Println(pos, n, err)
	// }()
	maxLen := r.t.torrent.Info.TotalLength() - pos
	if maxLen <= 0 {
		err = io.EOF
		return
	}
	if int64(len(b)) > maxLen {
		b = b[:maxLen]
	}
again:
	r.t.cl.mu.Lock()
	r.raisePriorities(pos, len(b))
	for !r.readable(pos) {
		r.raisePriorities(pos, len(b))
		r.waitReadable(pos)
	}
	avail := r.available(pos, int64(len(b)))
	// log.Println("available", avail)
	r.t.cl.mu.Unlock()
	b1 := b[:avail]
	pi := int(pos / r.t.Info().PieceLength)
	tp := &r.t.torrent.Pieces[pi]
	ip := r.t.Info().Piece(pi)
	po := pos % ip.Length()
	if int64(len(b1)) > ip.Length()-po {
		b1 = b1[:ip.Length()-po]
	}
	tp.pendingWritesMutex.Lock()
	for tp.pendingWrites != 0 {
		tp.noPendingWrites.Wait()
	}
	tp.pendingWritesMutex.Unlock()
	n, err = dataReadAt(r.t.torrent.data, b1, pos)
	if n != 0 {
		err = nil
		return
	}
	if r.t.torrent.isClosed() {
		if err == nil {
			err = errors.New("torrent closed")
		}
		return
	}
	if err == io.ErrUnexpectedEOF {
		goto again
	}
	return
}

func (r *Reader) Close() error {
	r.t = nil
	return nil
}

func (r *Reader) Seek(off int64, whence int) (ret int64, err error) {
	switch whence {
	case os.SEEK_SET:
		r.pos = off
	case os.SEEK_CUR:
		r.pos += off
	case os.SEEK_END:
		r.pos = r.t.torrent.Info.TotalLength() + off
	default:
		err = errors.New("bad whence")
	}
	ret = r.pos
	return
}
