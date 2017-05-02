package slices

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type herp int

func TestCastSliceInterface(t *testing.T) {
	var dest []herp
	MakeInto(&dest, []interface{}{herp(1), herp(2)})
	assert.Len(t, dest, 2)
	assert.EqualValues(t, 1, dest[0])
	assert.EqualValues(t, 2, dest[1])
}

func TestCastSliceInts(t *testing.T) {
	var dest []int
	MakeInto(&dest, []uint32{1, 2})
	assert.Len(t, dest, 2)
	assert.EqualValues(t, 1, dest[0])
	assert.EqualValues(t, 2, dest[1])
}
