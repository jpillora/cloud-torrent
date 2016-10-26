package storage

import (
	"encoding/binary"
	"path/filepath"
	"time"

	"github.com/boltdb/bolt"

	"github.com/anacrolix/torrent/metainfo"
)

var (
	value = []byte{}
)

type boltPieceCompletion struct {
	db *bolt.DB
}

func newBoltPieceCompletion(dir string) (ret *boltPieceCompletion, err error) {
	p := filepath.Join(dir, ".torrent.bolt.db")
	db, err := bolt.Open(p, 0660, &bolt.Options{
		Timeout: time.Second,
	})
	if err != nil {
		return
	}
	ret = &boltPieceCompletion{db}
	return
}

func (me *boltPieceCompletion) Get(pk metainfo.PieceKey) (ret bool, err error) {
	err = me.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(completed)
		if c == nil {
			return nil
		}
		ih := c.Bucket(pk.InfoHash[:])
		if ih == nil {
			return nil
		}
		var key [4]byte
		binary.BigEndian.PutUint32(key[:], uint32(pk.Index))
		ret = ih.Get(key[:]) != nil
		return nil
	})
	return
}

func (me *boltPieceCompletion) Set(pk metainfo.PieceKey, b bool) error {
	return me.db.Update(func(tx *bolt.Tx) error {
		c, err := tx.CreateBucketIfNotExists(completed)
		if err != nil {
			return err
		}
		ih, err := c.CreateBucketIfNotExists(pk.InfoHash[:])
		if err != nil {
			return err
		}
		var key [4]byte
		binary.BigEndian.PutUint32(key[:], uint32(pk.Index))
		if b {
			return ih.Put(key[:], value)
		} else {
			return ih.Delete(key[:])
		}
	})
}

func (me *boltPieceCompletion) Close() error {
	return me.db.Close()
}
