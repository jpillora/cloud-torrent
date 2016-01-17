package iplist

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The active ingredients in the sample P2P blocklist file contents `sample`,
// for reference:
//
// a:1.2.4.0-1.2.4.255
// b:1.2.8.0-1.2.8.255
// eff:1.2.8.2-1.2.8.2
// something:more detail:86.59.95.195-86.59.95.195
// eff:127.0.0.0-127.0.0.1`

func TestWritePacked(t *testing.T) {
	l, err := NewFromReader(strings.NewReader(sample))
	require.NoError(t, err)
	var buf bytes.Buffer
	err = l.WritePacked(&buf)
	require.NoError(t, err)
	require.Equal(t,
		"\x05\x00\x00\x00\x00\x00\x00\x00"+
			"\x01\x02\x04\x00\x01\x02\x04\xff"+"\x00\x00\x00\x00\x00\x00\x00\x00"+"\x01\x00\x00\x00"+
			"\x01\x02\x08\x00\x01\x02\x08\xff"+"\x01\x00\x00\x00\x00\x00\x00\x00"+"\x01\x00\x00\x00"+
			"\x01\x02\x08\x02\x01\x02\x08\x02"+"\x02\x00\x00\x00\x00\x00\x00\x00"+"\x03\x00\x00\x00"+
			"\x56\x3b\x5f\xc3\x56\x3b\x5f\xc3"+"\x05\x00\x00\x00\x00\x00\x00\x00"+"\x15\x00\x00\x00"+
			"\x7f\x00\x00\x00\x7f\x00\x00\x01"+"\x02\x00\x00\x00\x00\x00\x00\x00"+"\x03\x00\x00\x00"+
			"abeffsomething:more detail",
		buf.String())
}
