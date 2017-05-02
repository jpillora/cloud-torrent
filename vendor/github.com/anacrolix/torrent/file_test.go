package torrent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileExclusivePieces(t *testing.T) {
	for _, _case := range []struct {
		off, size, pieceSize int64
		begin, end           int
	}{
		{0, 2, 2, 0, 1},
		{1, 2, 2, 1, 1},
		{1, 4, 2, 1, 2},
	} {
		begin, end := byteRegionExclusivePieces(_case.off, _case.size, _case.pieceSize)
		assert.EqualValues(t, _case.begin, begin)
		assert.EqualValues(t, _case.end, end)
	}
}
