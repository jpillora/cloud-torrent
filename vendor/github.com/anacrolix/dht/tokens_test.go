package dht

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTokenServer(t *testing.T) {
	addr1 := NewAddr(&net.UDPAddr{
		IP: []byte{1, 2, 3, 4},
	})
	addr2 := NewAddr(&net.UDPAddr{
		IP: []byte{1, 2, 3, 3},
	})
	ts := tokenServer{
		secret:           []byte("42"),
		interval:         5 * time.Minute,
		maxIntervalDelta: 2,
	}
	tok := ts.CreateToken(addr1)
	assert.Len(t, tok, 20)
	assert.True(t, ts.ValidToken(tok, addr1))
	assert.False(t, ts.ValidToken(tok[1:], addr1))
	assert.False(t, ts.ValidToken(tok, addr2))
	func() {
		ts0 := ts
		ts0.secret = nil
		assert.False(t, ts0.ValidToken(tok, addr1))
	}()
	now := time.Now()
	setTime := func(t time.Time) {
		ts.timeNow = func() time.Time {
			return t
		}
	}
	setTime(now)
	tok = ts.CreateToken(addr1)
	assert.True(t, ts.ValidToken(tok, addr1))
	setTime(time.Time{})
	assert.False(t, ts.ValidToken(tok, addr1))
	setTime(now.Add(-5 * time.Minute))
	assert.False(t, ts.ValidToken(tok, addr1))
	setTime(now)
	assert.True(t, ts.ValidToken(tok, addr1))
	setTime(now.Add(5 * time.Minute))
	assert.True(t, ts.ValidToken(tok, addr1))
	setTime(now.Add(2 * 5 * time.Minute))
	assert.True(t, ts.ValidToken(tok, addr1))
	setTime(now.Add(3 * 5 * time.Minute))
	assert.False(t, ts.ValidToken(tok, addr1))
}
