package jsonpatch

import (
	"github.com/stretchr/testify/assert"
	"sort"
	"testing"
)

var simpleA = `{"a":100, "b":200, "c":"hello"}`
var simpleB = `{"a":100, "b":200, "c":"goodbye"}`
var simpleC = `{"a":100, "b":100, "c":"hello"}`
var simpleD = `{"a":100, "b":200, "c":"hello", "d":"foo"}`
var simpleE = `{"a":100, "b":200}`
var simplef = `{"a":100, "b":100, "d":"foo"}`
var empty = `{}`

func TestSame(t *testing.T) {
	patch, e := CreatePatch([]byte(simpleA), []byte(simpleA))
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 0, "they should be equal")
}

func TestOneStringReplace(t *testing.T) {
	patch, e := CreatePatch([]byte(simpleA), []byte(simpleB))
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 1, "they should be equal")
	change := patch[0]
	assert.Equal(t, change.Operation, "replace", "they should be equal")
	assert.Equal(t, change.Path, "/c", "they should be equal")
	assert.Equal(t, change.Value, "goodbye", "they should be equal")
}

func TestOneIntReplace(t *testing.T) {
	patch, e := CreatePatch([]byte(simpleA), []byte(simpleC))
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 1, "they should be equal")
	change := patch[0]
	assert.Equal(t, change.Operation, "replace", "they should be equal")
	assert.Equal(t, change.Path, "/b", "they should be equal")
	var expected float64 = 100
	assert.Equal(t, change.Value, expected, "they should be equal")
}

func TestOneAdd(t *testing.T) {
	patch, e := CreatePatch([]byte(simpleA), []byte(simpleD))
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 1, "they should be equal")
	change := patch[0]
	assert.Equal(t, change.Operation, "add", "they should be equal")
	assert.Equal(t, change.Path, "/d", "they should be equal")
	assert.Equal(t, change.Value, "foo", "they should be equal")
}

func TestOneRemove(t *testing.T) {
	patch, e := CreatePatch([]byte(simpleA), []byte(simpleE))
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 1, "they should be equal")
	change := patch[0]
	assert.Equal(t, change.Operation, "remove", "they should be equal")
	assert.Equal(t, change.Path, "/c", "they should be equal")
	assert.Equal(t, change.Value, nil, "they should be equal")
}

func TestVsEmpty(t *testing.T) {
	patch, e := CreatePatch([]byte(simpleA), []byte(empty))
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 3, "they should be equal")
	sort.Sort(ByPath(patch))
	change := patch[0]
	assert.Equal(t, change.Operation, "remove", "they should be equal")
	assert.Equal(t, change.Path, "/a", "they should be equal")

	change = patch[1]
	assert.Equal(t, change.Operation, "remove", "they should be equal")
	assert.Equal(t, change.Path, "/b", "they should be equal")

	change = patch[2]
	assert.Equal(t, change.Operation, "remove", "they should be equal")
	assert.Equal(t, change.Path, "/c", "they should be equal")
}
