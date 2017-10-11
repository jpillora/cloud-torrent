package torrent

import (
	"fmt"
	"io"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/time/rate"
)

type rateLimitedReader struct {
	l *rate.Limiter
	r io.Reader
}

func (me rateLimitedReader) Read(b []byte) (n int, err error) {
	// Wait until we can read at all.
	if err := me.l.WaitN(context.Background(), 1); err != nil {
		panic(err)
	}
	// Limit the read to within the burst.
	if me.l.Limit() != rate.Inf && len(b) > me.l.Burst() {
		b = b[:me.l.Burst()]
	}
	n, err = me.r.Read(b)
	// Pay the piper.
	if !me.l.ReserveN(time.Now(), n-1).OK() {
		panic(fmt.Sprintf("burst exceeded?: %d", n-1))
	}
	return
}
