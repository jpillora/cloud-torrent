package storage

import (
	"log"

	"github.com/anacrolix/torrent/metainfo"
)

// Implementations track the completion of pieces.
type pieceCompletion interface {
	Get(metainfo.PieceKey) (bool, error)
	Set(metainfo.PieceKey, bool) error
	Close() error
}

func pieceCompletionForDir(dir string) (ret pieceCompletion) {
	ret, err := newBoltPieceCompletion(dir)
	if err != nil {
		log.Printf("couldn't open piece completion db in %q: %s", dir, err)
		ret = new(mapPieceCompletion)
	}
	return
}
