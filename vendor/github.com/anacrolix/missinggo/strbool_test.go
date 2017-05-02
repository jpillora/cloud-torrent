package missinggo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringTruth(t *testing.T) {
	for _, s := range []string{
		"",
		" ",
		"\n",
		"\x00",
		"0",
	} {
		t.Run(s, func(t *testing.T) {
			assert.False(t, StringTruth(s))
		})
	}
	for _, s := range []string{
		" 1",
		"t",
	} {
		t.Run(s, func(t *testing.T) {
			assert.True(t, StringTruth(s))
		})
	}
}
