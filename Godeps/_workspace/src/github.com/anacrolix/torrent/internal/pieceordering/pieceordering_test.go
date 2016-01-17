package pieceordering

import (
	"sort"
	"testing"

	"github.com/bradfitz/iter"
	"github.com/stretchr/testify/assert"
)

func instanceSlice(i *Instance) (sl []int) {
	for e := i.First(); e != nil; e = e.Next() {
		sl = append(sl, e.Piece())
	}
	return
}

func sameContents(a, b []int) bool {
	if len(a) != len(b) {
		panic("y u pass different length slices")
	}
	sort.IntSlice(a).Sort()
	sort.IntSlice(b).Sort()
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func checkOrder(t testing.TB, i *Instance, ppp ...[]int) {
	fatal := func() {
		t.Fatalf("have %v, expected %v", instanceSlice(i), ppp)
	}
	e := i.First()
	for _, pp := range ppp {
		var pp_ []int
		for len(pp_) != len(pp) {
			pp_ = append(pp_, e.Piece())
			e = e.Next()
		}
		if !sameContents(pp, pp_) {
			fatal()
		}
	}
	if e != nil {
		fatal()
	}
}

func testPieceOrdering(t testing.TB) {
	i := New()
	assert.True(t, i.Empty())
	i.SetPiece(0, 1)
	assert.False(t, i.Empty())
	i.SetPiece(1, 0)
	checkOrder(t, i, []int{1, 0})
	i.SetPiece(1, 2)
	checkOrder(t, i, []int{0, 1})
	i.DeletePiece(1)
	checkOrder(t, i, []int{0})
	i.DeletePiece(2)
	i.DeletePiece(1)
	checkOrder(t, i, []int{0})
	i.DeletePiece(0)
	assert.True(t, i.Empty())
	checkOrder(t, i, nil)
	i.SetPiece(2, 1)
	assert.False(t, i.Empty())
	i.SetPiece(1, 1)
	i.SetPiece(3, 1)
	checkOrder(t, i, []int{3, 1, 2})
	// Move a piece that isn't the youngest in a key.
	i.SetPiece(1, -1)
	checkOrder(t, i, []int{1}, []int{3, 2})
	i.DeletePiece(2)
	i.DeletePiece(3)
	i.DeletePiece(1)
	assert.True(t, i.Empty())
	checkOrder(t, i, nil)
	// Deleting pieces that aren't present.
	i.DeletePiece(2)
	i.DeletePiece(3)
	i.DeletePiece(1)
	assert.True(t, i.Empty())
	checkOrder(t, i, nil)
}

func TestPieceOrdering(t *testing.T) {
	testPieceOrdering(t)
}

func BenchmarkPieceOrdering(b *testing.B) {
	for range iter.N(b.N) {
		testPieceOrdering(b)
	}
}

func BenchmarkIteration(b *testing.B) {
	for range iter.N(b.N) {
		i := New()
		for p := range iter.N(500) {
			i.SetPiece(p, p)
		}
		for e := i.First(); e != nil; e = e.Next() {
		}
	}
}
