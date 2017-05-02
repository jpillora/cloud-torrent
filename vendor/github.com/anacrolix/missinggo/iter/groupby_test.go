package iter

import (
	"testing"

	"github.com/anacrolix/missinggo/slices"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupByKey(t *testing.T) {
	var ks []byte
	gb := GroupBy(StringIterator("AAAABBBCCDAABBB"), nil)
	for gb.Next() {
		ks = append(ks, gb.Value().(Group).Key().(byte))
	}
	t.Log(ks)
	require.EqualValues(t, "ABCDAB", ks)
}

func TestGroupByList(t *testing.T) {
	var gs []string
	gb := GroupBy(StringIterator("AAAABBBCCD"), nil)
	for gb.Next() {
		i := gb.Value().(Iterator)
		var g string
		for i.Next() {
			g += string(i.Value().(byte))
		}
		gs = append(gs, g)
	}
	t.Log(gs)
}

func TestGroupByNiladicKey(t *testing.T) {
	const s = "AAAABBBCCD"
	gb := GroupBy(StringIterator(s), func(interface{}) interface{} { return nil })
	gb.Next()
	var ss []byte
	g := ToSlice(gb.Value().(Iterator))
	slices.MakeInto(&ss, g)
	assert.Equal(t, s, string(ss))
}

func TestNilEqualsNil(t *testing.T) {
	assert.False(t, nil == uniqueKey)
}
