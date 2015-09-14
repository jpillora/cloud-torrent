package jsonpatch

import (
	//	"fmt"
	"github.com/stretchr/testify/assert"
	"sort"
	"testing"
)

var complexBase = `{"a":100, "b":[{"c1":"hello", "d1":"foo"},{"c2":"hello2", "d2":"foo2"} ], "e":{"f":200, "g":"h", "i":"j"}}`
var complexA = `{"a":100, "b":[{"c1":"goodbye", "d1":"foo"},{"c2":"hello2", "d2":"foo2"} ], "e":{"f":200, "g":"h", "i":"j"}}`
var complexB = `{"a":100, "b":[{"c1":"hello", "d1":"foo"},{"c2":"hello2", "d2":"foo2"} ], "e":{"f":100, "g":"h", "i":"j"}}`
var complexC = `{"a":100, "b":[{"c1":"hello", "d1":"foo"},{"c2":"hello2", "d2":"foo2"} ], "e":{"f":200, "g":"h", "i":"j"}, "k":[{"l":"m"}, {"l":"o"}]}`
var complexD = `{"a":100, "b":[{"c1":"hello", "d1":"foo"},{"c2":"hello2", "d2":"foo2"}, {"c3":"hello3", "d3":"foo3"} ], "e":{"f":200, "g":"h", "i":"j"}}`
var complexE = `{"a":100, "b":[{"c1":"hello", "d1":"foo"},{"c2":"hello2", "d2":"foo2"} ], "e":{"f":200, "g":"h", "i":"j"}}`

func TestComplexSame(t *testing.T) {
	patch, e := CreatePatch([]byte(complexBase), []byte(complexBase))
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 0, "they should be equal")
}
func TestComplexOneStringReplaceInArray(t *testing.T) {
	patch, e := CreatePatch([]byte(complexBase), []byte(complexA))
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 1, "they should be equal")
	change := patch[0]
	assert.Equal(t, change.Operation, "replace", "they should be equal")
	assert.Equal(t, change.Path, "/b/0/c1", "they should be equal")
	assert.Equal(t, change.Value, "goodbye", "they should be equal")
}

func TestComplexOneIntReplace(t *testing.T) {
	patch, e := CreatePatch([]byte(complexBase), []byte(complexB))
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 1, "they should be equal")
	change := patch[0]
	assert.Equal(t, change.Operation, "replace", "they should be equal")
	assert.Equal(t, change.Path, "/e/f", "they should be equal")
	assert.Equal(t, change.Value, 100, "they should be equal")
}

func TestComplexOneAdd(t *testing.T) {
	patch, e := CreatePatch([]byte(complexBase), []byte(complexC))
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 1, "they should be equal")
	change := patch[0]
	assert.Equal(t, change.Operation, "add", "they should be equal")
	assert.Equal(t, change.Path, "/k", "they should be equal")
	a := make(map[string]interface{})
	b := make(map[string]interface{})
	a["l"] = "m"
	b["l"] = "o"
	expected := []interface{}{a, b}
	assert.Equal(t, change.Value, expected, "they should be equal")
}

func TestComplexOneAddToArray(t *testing.T) {
	patch, e := CreatePatch([]byte(complexBase), []byte(complexC))
	assert.NoError(t, e)
	assert.Equal(t, len(patch), 1, "they should be equal")
	change := patch[0]
	assert.Equal(t, change.Operation, "add", "they should be equal")
	assert.Equal(t, change.Path, "/k", "they should be equal")
	a := make(map[string]interface{})
	b := make(map[string]interface{})
	a["l"] = "m"
	b["l"] = "o"
	expected := []interface{}{a, b}
	assert.Equal(t, change.Value, expected, "they should be equal")
}

func TestComplexVsEmpty(t *testing.T) {
	patch, e := CreatePatch([]byte(complexBase), []byte(empty))
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
	assert.Equal(t, change.Path, "/e", "they should be equal")
}
