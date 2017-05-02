package torrent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"

	"github.com/anacrolix/torrent/internal/testutil"
)

func TestReaderReadContext(t *testing.T) {
	cl, err := NewClient(&TestingConfig)
	require.NoError(t, err)
	defer cl.Close()
	tt, err := cl.AddTorrent(testutil.GreetingMetaInfo())
	require.NoError(t, err)
	defer tt.Drop()
	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Millisecond))
	r := tt.NewReader()
	defer r.Close()
	_, err = r.ReadContext(make([]byte, 1), ctx)
	require.EqualValues(t, context.DeadlineExceeded, err)
}
