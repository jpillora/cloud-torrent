// +build cgo

package storage

import (
	"database/sql"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"

	"github.com/anacrolix/torrent/metainfo"
)

type sqlitePieceCompletion struct {
	db *sql.DB
}

func newSqlitePieceCompletion(dir string) (ret *sqlitePieceCompletion, err error) {
	p := filepath.Join(dir, ".torrent.db")
	db, err := sql.Open("sqlite3", p)
	if err != nil {
		return
	}
	_, err = db.Exec(`create table if not exists completed(infohash, "index", unique(infohash, "index") on conflict ignore)`)
	if err != nil {
		db.Close()
		return
	}
	ret = &sqlitePieceCompletion{db}
	return
}

func (me *sqlitePieceCompletion) Get(pk metainfo.PieceKey) (ret bool, err error) {
	row := me.db.QueryRow(`select exists(select * from completed where infohash=? and "index"=?)`, pk.InfoHash.HexString(), pk.Index)
	err = row.Scan(&ret)
	return
}

func (me *sqlitePieceCompletion) Set(pk metainfo.PieceKey, b bool) (err error) {
	if b {
		_, err = me.db.Exec(`insert into completed (infohash, "index") values (?, ?)`, pk.InfoHash.HexString(), pk.Index)
	} else {
		_, err = me.db.Exec(`delete from completed where infohash=? and "index"=?`, pk.InfoHash.HexString(), pk.Index)
	}
	return
}

func (me *sqlitePieceCompletion) Close() {
	me.db.Close()
}
