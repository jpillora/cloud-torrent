package torrent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTorrentOffsetRequest(t *testing.T) {
	check := func(tl, ps, off int64, expected request, ok bool) {
		req, _ok := torrentOffsetRequest(tl, ps, defaultChunkSize, off)
		assert.Equal(t, _ok, ok)
		assert.Equal(t, req, expected)
	}
	check(13, 5, 0, newRequest(0, 0, 5), true)
	check(13, 5, 3, newRequest(0, 0, 5), true)
	check(13, 5, 11, newRequest(2, 0, 3), true)
	check(13, 5, 13, request{}, false)
}
