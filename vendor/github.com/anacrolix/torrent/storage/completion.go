package storage

import (
	"log"

	"github.com/anacrolix/torrent/metainfo"
)

// Implementations track the completion of pieces. It must be concurrent-safe.
type PieceCompletion interface {
	Get(metainfo.PieceKey) (bool, error)
	Set(metainfo.PieceKey, bool) error
	Close() error
}

func pieceCompletionForDir(dir string) (ret PieceCompletion) {
	ret, err := NewBoltPieceCompletion(dir)
	if err != nil {
		log.Printf("couldn't open piece completion db in %q: %s", dir, err)
		ret = NewMapPieceCompletion()
	}
	return
}
