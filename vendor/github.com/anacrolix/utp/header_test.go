package utp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectiveAckBitmaskBytesLen(t *testing.T) {
	for _, _case := range []struct {
		BitIndex    int
		ExpectedLen int
	}{
		{0, 4},
		{31, 4},
		{32, 8},
	} {
		var selAck selectiveAckBitmask
		selAck.SetBit(_case.BitIndex)
		assert.EqualValues(t, _case.ExpectedLen, len(selAck.Bytes))
	}
}
