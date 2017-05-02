package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anacrolix/torrent/metainfo"
)

func TestExtentCompleteRequiredLengths(t *testing.T) {
	info := &metainfo.Info{
		Files: []metainfo.FileInfo{
			{Path: []string{"a"}, Length: 2},
			{Path: []string{"b"}, Length: 3},
		},
	}
	assert.Empty(t, extentCompleteRequiredLengths(info, 0, 0))
	assert.EqualValues(t, []metainfo.FileInfo{
		{Path: []string{"a"}, Length: 1},
	}, extentCompleteRequiredLengths(info, 0, 1))
	assert.EqualValues(t, []metainfo.FileInfo{
		{Path: []string{"a"}, Length: 2},
	}, extentCompleteRequiredLengths(info, 0, 2))
	assert.EqualValues(t, []metainfo.FileInfo{
		{Path: []string{"a"}, Length: 2},
		{Path: []string{"b"}, Length: 1},
	}, extentCompleteRequiredLengths(info, 0, 3))
	assert.EqualValues(t, []metainfo.FileInfo{
		{Path: []string{"b"}, Length: 2},
	}, extentCompleteRequiredLengths(info, 2, 2))
	assert.EqualValues(t, []metainfo.FileInfo{
		{Path: []string{"b"}, Length: 3},
	}, extentCompleteRequiredLengths(info, 4, 1))
	assert.Len(t, extentCompleteRequiredLengths(info, 5, 0), 0)
	assert.Panics(t, func() { extentCompleteRequiredLengths(info, 6, 1) })
}
