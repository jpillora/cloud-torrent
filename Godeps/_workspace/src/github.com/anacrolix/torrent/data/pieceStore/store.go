package pieceStore

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	"github.com/bradfitz/iter"

	"github.com/anacrolix/torrent/data/pieceStore/dataBackend"
	"github.com/anacrolix/torrent/metainfo"
)

type store struct {
	db dataBackend.I

	mu sync.Mutex
	// The cached completion state for pieces.
	completion map[[20]byte]bool
	lastError  time.Time
}

func (me *store) completedPiecePath(p metainfo.Piece) string {
	return path.Join("completed", hex.EncodeToString(p.Hash()))
}

func (me *store) incompletePiecePath(p metainfo.Piece) string {
	return path.Join(
		"incomplete",
		strconv.FormatInt(int64(os.Getpid()), 10),
		hex.EncodeToString(p.Hash()))
}

func (me *store) OpenTorrentData(info *metainfo.Info) (ret *data) {
	ret = &data{info, me}
	for i := range iter.N(info.NumPieces()) {
		go ret.PieceComplete(i)
	}
	return
}

func New(db dataBackend.I) *store {
	s := &store{
		db: db,
	}
	return s
}

// Turns 40 byte hex string into its equivalent binary byte array.
func hexStringPieceHashArray(s string) (ret [20]byte, ok bool) {
	if len(s) != 40 {
		return
	}
	n, err := hex.Decode(ret[:], []byte(s))
	if err != nil {
		return
	}
	if n != 20 {
		panic(n)
	}
	ok = true
	return
}

func sliceToPieceHashArray(b []byte) (ret [20]byte) {
	n := copy(ret[:], b)
	if n != 20 {
		panic(n)
	}
	return
}

func pieceHashArray(p metainfo.Piece) [20]byte {
	return sliceToPieceHashArray(p.Hash())
}

func (me *store) completionKnown(p metainfo.Piece) bool {
	me.mu.Lock()
	_, ok := me.completion[pieceHashArray(p)]
	me.mu.Unlock()
	return ok
}

func (me *store) isComplete(p metainfo.Piece) bool {
	me.mu.Lock()
	ret, _ := me.completion[pieceHashArray(p)]
	me.mu.Unlock()
	return ret
}

func (me *store) setCompletion(p metainfo.Piece, complete bool) {
	me.mu.Lock()
	if me.completion == nil {
		me.completion = make(map[[20]byte]bool)
	}
	me.completion[pieceHashArray(p)] = complete
	me.mu.Unlock()
}

func (me *store) pieceComplete(p metainfo.Piece) bool {
	if me.completionKnown(p) {
		return me.isComplete(p)
	}
	// Prevent a errors from stalling the caller.
	if !me.lastError.IsZero() && time.Since(me.lastError) < time.Second {
		return false
	}
	length, err := me.db.GetLength(me.completedPiecePath(p))
	if err == dataBackend.ErrNotFound {
		me.setCompletion(p, false)
		return false
	}
	if err != nil {
		me.lastError = time.Now()
		log.Printf("%+v", err)
		return false
	}
	complete := length == p.Length()
	if !complete {
		log.Printf("completed piece %x has wrong length: %d", p.Hash(), length)
	}
	me.setCompletion(p, complete)
	return complete
}

func (me *store) pieceWriteAt(p metainfo.Piece, b []byte, off int64) (n int, err error) {
	if me.pieceComplete(p) {
		err = errors.New("already have piece")
		return
	}
	f, err := me.db.Open(me.incompletePiecePath(p), os.O_WRONLY|os.O_CREATE)
	if err != nil {
		err = fmt.Errorf("error opening %q: %s", me.incompletePiecePath(p), err)
		return
	}
	defer func() {
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
	}()
	_, err = f.Seek(off, os.SEEK_SET)
	if err != nil {
		return
	}
	n, err = f.Write(b)
	return
}

func (me *store) forgetCompletions() {
	me.mu.Lock()
	me.completion = nil
	me.mu.Unlock()
}

func (me *store) getPieceRange(p metainfo.Piece, off, n int64) (ret io.ReadCloser, err error) {
	rc, err := me.db.OpenSection(me.completedPiecePath(p), off, n)
	if err == dataBackend.ErrNotFound {
		if me.isComplete(p) {
			me.forgetCompletions()
		}
		me.setCompletion(p, false)
		rc, err = me.db.OpenSection(me.incompletePiecePath(p), off, n)
	}
	if err == dataBackend.ErrNotFound {
		err = io.ErrUnexpectedEOF
		return
	}
	if err != nil {
		return
	}
	// Wrap up the response body so that the request slot is released when the
	// response body is closed.
	ret = rc
	return
}

func (me *store) pieceReadAt(p metainfo.Piece, b []byte, off int64) (n int, err error) {
	rc, err := me.getPieceRange(p, off, int64(len(b)))
	if err != nil {
		return
	}
	defer rc.Close()
	n, err = io.ReadFull(rc, b)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return
}

func (me *store) removePath(path string) (err error) {
	err = me.db.Delete(path)
	return
}

// Remove the completed piece if it exists, and mark the piece not completed.
// Mustn't fail.
func (me *store) deleteCompleted(p metainfo.Piece) {
	if err := me.removePath(me.completedPiecePath(p)); err != nil {
		panic(err)
	}
	me.setCompletion(p, false)
}

func (me *store) hashCopyFile(from, to string, n int64) (hash []byte, err error) {
	src, err := me.db.OpenSection(from, 0, n)
	if err != nil {
		return
	}
	defer src.Close()
	hasher := sha1.New()
	tee := io.TeeReader(src, hasher)
	dest, err := me.db.Open(to, os.O_WRONLY|os.O_CREATE)
	if err != nil {
		return
	}
	defer dest.Close()
	_, err = io.Copy(dest, tee)
	if err != nil {
		return
	}
	hash = hasher.Sum(nil)
	return
}

func (me *store) PieceCompleted(p metainfo.Piece) (err error) {
	hash, err := me.hashCopyFile(me.incompletePiecePath(p), me.completedPiecePath(p), p.Length())
	if err == nil && !bytes.Equal(hash, p.Hash()) {
		err = errors.New("piece incomplete")
	}
	if err != nil {
		me.deleteCompleted(p)
		return
	}
	me.removePath(me.incompletePiecePath(p))
	me.setCompletion(p, true)
	return
}
