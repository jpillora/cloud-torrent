package blob

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/anacrolix/missinggo"

	dataPkg "github.com/anacrolix/torrent/data"
	"github.com/anacrolix/torrent/metainfo"
)

const (
	filePerm = 0640
	dirPerm  = 0750
)

type store struct {
	baseDir  string
	capacity int64

	mu        sync.Mutex
	completed map[[20]byte]struct{}
}

func (me *store) OpenTorrent(info *metainfo.Info) dataPkg.Data {
	return &data{info, me}
}

type StoreOption func(*store)

func Capacity(bytes int64) StoreOption {
	return func(s *store) {
		s.capacity = bytes
	}
}

func NewStore(baseDir string, opt ...StoreOption) dataPkg.Store {
	s := &store{baseDir, -1, sync.Mutex{}, nil}
	for _, o := range opt {
		o(s)
	}
	s.initCompleted()
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

func (me *store) initCompleted() {
	fis, err := me.readCompletedDir()
	if err != nil {
		panic(err)
	}
	me.mu.Lock()
	me.completed = make(map[[20]byte]struct{}, len(fis))
	for _, fi := range fis {
		binHash, ok := hexStringPieceHashArray(fi.Name())
		if !ok {
			continue
		}
		me.completed[binHash] = struct{}{}
	}
	me.mu.Unlock()
}

func (me *store) completePieceDirPath() string {
	return filepath.Join(me.baseDir, "complete")
}

func (me *store) path(p metainfo.Piece, completed bool) string {
	return filepath.Join(me.baseDir, func() string {
		if completed {
			return "complete"
		} else {
			return "incomplete"
		}
	}(), fmt.Sprintf("%x", p.Hash()))
}

func sliceToPieceHashArray(b []byte) (ret [20]byte) {
	n := copy(ret[:], b)
	if n != 20 {
		panic(n)
	}
	return
}

func (me *store) pieceComplete(p metainfo.Piece) bool {
	me.mu.Lock()
	defer me.mu.Unlock()
	_, ok := me.completed[sliceToPieceHashArray(p.Hash())]
	return ok
}

func (me *store) pieceWrite(p metainfo.Piece) (f *os.File) {
	if me.pieceComplete(p) {
		return
	}
	name := me.path(p, false)
	os.MkdirAll(filepath.Dir(name), dirPerm)
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY, filePerm)
	if err != nil {
		panic(err)
	}
	return
}

// Returns the file for the given piece, if it exists. It could be completed,
// or incomplete.
func (me *store) pieceRead(p metainfo.Piece) (f *os.File) {
	f, err := os.Open(me.path(p, true))
	if err == nil {
		return
	}
	if !os.IsNotExist(err) {
		panic(err)
	}
	// Mark the file not completed, in case we thought it was. TODO: Trigger
	// an asynchronous initCompleted to reinitialize the entire completed map
	// as there are likely other files missing.
	me.mu.Lock()
	delete(me.completed, sliceToPieceHashArray(p.Hash()))
	me.mu.Unlock()
	f, err = os.Open(me.path(p, false))
	if err == nil {
		return
	}
	if !os.IsNotExist(err) {
		panic(err)
	}
	return
}

func (me *store) readCompletedDir() (fis []os.FileInfo, err error) {
	f, err := os.Open(me.completePieceDirPath())
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return
	}
	fis, err = f.Readdir(-1)
	f.Close()
	return
}

func (me *store) removeCompleted(name string) (err error) {
	err = os.Remove(filepath.Join(me.completePieceDirPath(), name))
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return err
	}
	binHash, ok := hexStringPieceHashArray(name)
	if ok {
		me.mu.Lock()
		delete(me.completed, binHash)
		me.mu.Unlock()
	}
	return
}

type fileInfoSorter struct {
	fis []os.FileInfo
}

func (me fileInfoSorter) Len() int {
	return len(me.fis)
}

func lastTime(fi os.FileInfo) (ret time.Time) {
	ret = fi.ModTime()
	atime := missinggo.FileInfoAccessTime(fi)
	if atime.After(ret) {
		ret = atime
	}
	return
}

func (me fileInfoSorter) Less(i, j int) bool {
	return lastTime(me.fis[i]).Before(lastTime(me.fis[j]))
}

func (me fileInfoSorter) Swap(i, j int) {
	me.fis[i], me.fis[j] = me.fis[j], me.fis[i]
}

func sortFileInfos(fis []os.FileInfo) {
	sorter := fileInfoSorter{fis}
	sort.Sort(sorter)
}

func (me *store) makeSpace(space int64) error {
	if me.capacity < 0 {
		return nil
	}
	if space > me.capacity {
		return errors.New("space requested exceeds capacity")
	}
	fis, err := me.readCompletedDir()
	if err != nil {
		return err
	}
	var size int64
	for _, fi := range fis {
		size += fi.Size()
	}
	sortFileInfos(fis)
	for size > me.capacity-space {
		me.removeCompleted(fis[0].Name())
		size -= fis[0].Size()
		fis = fis[1:]
	}
	return nil
}

func (me *store) PieceCompleted(p metainfo.Piece) (err error) {
	err = me.makeSpace(p.Length())
	if err != nil {
		return
	}
	var (
		incompletePiecePath = me.path(p, false)
		completedPiecePath  = me.path(p, true)
	)
	fSrc, err := os.Open(incompletePiecePath)
	if err != nil {
		return
	}
	defer fSrc.Close()
	os.MkdirAll(filepath.Dir(completedPiecePath), dirPerm)
	fDst, err := os.OpenFile(completedPiecePath, os.O_EXCL|os.O_CREATE|os.O_WRONLY, filePerm)
	if err != nil {
		return
	}
	defer fDst.Close()
	hasher := sha1.New()
	r := io.TeeReader(io.LimitReader(fSrc, p.Length()), hasher)
	_, err = io.Copy(fDst, r)
	if err != nil {
		return
	}
	if !bytes.Equal(hasher.Sum(nil), p.Hash()) {
		err = errors.New("piece incomplete")
		os.Remove(completedPiecePath)
		return
	}
	os.Remove(incompletePiecePath)
	me.mu.Lock()
	me.completed[sliceToPieceHashArray(p.Hash())] = struct{}{}
	me.mu.Unlock()
	return
}
