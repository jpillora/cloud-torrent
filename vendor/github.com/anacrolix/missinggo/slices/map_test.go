package slices

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFromMap(t *testing.T) {
	sl := FromMap(map[string]int{"two": 2, "one": 1})
	assert.Len(t, sl, 2)
	Sort(sl, func(left, right MapItem) bool {
		return left.Key.(string) < right.Key.(string)
	})
	assert.EqualValues(t, []MapItem{{"one", 1}, {"two", 2}}, sl)
}
