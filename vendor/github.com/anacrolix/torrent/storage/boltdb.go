package storage

import (
	"encoding/binary"
	"path/filepath"

	"github.com/boltdb/bolt"

	"github.com/anacrolix/torrent/metainfo"
)

const (
	// Chosen to match the usual chunk size in a torrent client. This way,
	// most chunk writes are to exactly one full item in bolt DB.
	chunkSize = 1 << 14
)

var (
	// The key for the data bucket.
	data = []byte("data")
	// The key for the completion flag bucket.
	completed = []byte("completed")
	// The value to assigned to pieces that are complete in the completed
	// bucket.
	completedValue = []byte{1}
)

type boltDBClient struct {
	db *bolt.DB
}

type boltDBTorrent struct {
	cl *boltDBClient
	ih metainfo.Hash
}

type boltDBPiece struct {
	db  *bolt.DB
	p   metainfo.Piece
	key [24]byte
}

func NewBoltDB(filePath string) ClientImpl {
	ret := &boltDBClient{}
	var err error
	ret.db, err = bolt.Open(filepath.Join(filePath, "bolt.db"), 0600, nil)
	if err != nil {
		panic(err)
	}
	return ret
}

func (me *boltDBClient) Close() error {
	return me.db.Close()
}

func (me *boltDBClient) OpenTorrent(info *metainfo.Info, infoHash metainfo.Hash) (TorrentImpl, error) {
	return &boltDBTorrent{me, infoHash}, nil
}

func (me *boltDBTorrent) Piece(p metainfo.Piece) PieceImpl {
	ret := &boltDBPiece{p: p, db: me.cl.db}
	copy(ret.key[:], me.ih[:])
	binary.BigEndian.PutUint32(ret.key[20:], uint32(p.Index()))
	return ret
}

func (boltDBTorrent) Close() error { return nil }

func (me *boltDBPiece) GetIsComplete() (complete bool) {
	err := me.db.View(func(tx *bolt.Tx) error {
		cb := tx.Bucket(completed)
		// db := tx.Bucket(data)
		complete =
			cb != nil && len(cb.Get(me.key[:])) != 0
			// db != nil && int64(len(db.Get(me.key[:]))) == me.p.Length()
		return nil
	})
	if err != nil {
		panic(err)
	}
	return
}

func (me *boltDBPiece) MarkComplete() error {
	return me.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(completed)
		if err != nil {
			return err
		}
		return b.Put(me.key[:], completedValue)
	})
}

func (me *boltDBPiece) MarkNotComplete() error {
	return me.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(completed)
		if b == nil {
			return nil
		}
		return b.Delete(me.key[:])
	})
}
func (me *boltDBPiece) ReadAt(b []byte, off int64) (n int, err error) {
	err = me.db.View(func(tx *bolt.Tx) error {
		db := tx.Bucket(data)
		if db == nil {
			return nil
		}
		ci := off / chunkSize
		off %= chunkSize
		for len(b) != 0 {
			ck := me.chunkKey(int(ci))
			_b := db.Get(ck[:])
			if len(_b) != chunkSize {
				break
			}
			n1 := copy(b, _b[off:])
			off = 0
			ci++
			b = b[n1:]
			n += n1
		}
		return nil
	})
	return
}

func (me *boltDBPiece) chunkKey(index int) (ret [26]byte) {
	copy(ret[:], me.key[:])
	binary.BigEndian.PutUint16(ret[24:], uint16(index))
	return
}

func (me *boltDBPiece) WriteAt(b []byte, off int64) (n int, err error) {
	err = me.db.Update(func(tx *bolt.Tx) error {
		db, err := tx.CreateBucketIfNotExists(data)
		if err != nil {
			return err
		}
		ci := off / chunkSize
		off %= chunkSize
		for len(b) != 0 {
			_b := make([]byte, chunkSize)
			ck := me.chunkKey(int(ci))
			copy(_b, db.Get(ck[:]))
			n1 := copy(_b[off:], b)
			db.Put(ck[:], _b)
			if n1 > len(b) {
				break
			}
			b = b[n1:]
			off = 0
			ci++
			n += n1
		}
		return nil
	})
	return
}
