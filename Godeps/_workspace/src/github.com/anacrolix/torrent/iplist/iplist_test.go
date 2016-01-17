package iplist

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/anacrolix/missinggo"
	"github.com/bradfitz/iter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// Note the shared description "eff". The overlapping ranges at 1.2.8.2
	// will cause problems. Don't overlap your ranges.
	sample = `
# List distributed by iblocklist.com

a:1.2.4.0-1.2.4.255
b:1.2.8.0-1.2.8.255
eff:1.2.8.2-1.2.8.2
something:more detail:86.59.95.195-86.59.95.195
eff:127.0.0.0-127.0.0.1`
	packedSample []byte
)

func init() {
	var buf bytes.Buffer
	list, err := NewFromReader(strings.NewReader(sample))
	if err != nil {
		panic(err)
	}
	err = list.WritePacked(&buf)
	if err != nil {
		panic(err)
	}
	packedSample = buf.Bytes()
}

func TestIPv4RangeLen(t *testing.T) {
	ranges, _ := sampleRanges(t)
	for i := range iter.N(3) {
		if len(ranges[i].First) != 4 {
			t.FailNow()
		}
		if len(ranges[i].Last) != 4 {
			t.FailNow()
		}
	}
}

func sampleRanges(tb testing.TB) (ranges []Range, err error) {
	scanner := bufio.NewScanner(strings.NewReader(sample))
	for scanner.Scan() {
		r, ok, err := ParseBlocklistP2PLine(scanner.Bytes())
		if err != nil {
			tb.Fatal(err)
		}
		if ok {
			ranges = append(ranges, r)
		}
	}
	err = scanner.Err()
	return
}

func BenchmarkParseP2pBlocklist(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sampleRanges(b)
	}
}

func connRemoteAddrIP(network, laddr string, dialHost string) net.IP {
	l, err := net.Listen(network, laddr)
	if err != nil {
		panic(err)
	}
	go func() {
		c, err := net.Dial(network, net.JoinHostPort(dialHost, fmt.Sprintf("%d", missinggo.AddrPort(l.Addr()))))
		if err != nil {
			panic(err)
		}
		defer c.Close()
	}()
	c, err := l.Accept()
	if err != nil {
		panic(err)
	}
	defer c.Close()
	ret := missinggo.AddrIP(c.RemoteAddr())
	return ret
}

func lookupOk(r Range, ok bool) bool {
	return ok
}

func TestBadIP(t *testing.T) {
	for _, iplist := range []Ranger{
		New(nil),
		NewFromPacked([]byte("\x00\x00\x00\x00\x00\x00\x00\x00")),
	} {
		assert.False(t, lookupOk(iplist.Lookup(net.IP(make([]byte, 4)))), "%v", iplist)
		assert.False(t, lookupOk(iplist.Lookup(net.IP(make([]byte, 16)))))
		r, ok := iplist.Lookup(nil)
		assert.True(t, ok)
		assert.Equal(t, r.Description, "bad IP")
		assert.True(t, lookupOk(iplist.Lookup(net.IP(make([]byte, 5)))))
	}
}

func testLookuperSimple(t *testing.T, iplist Ranger) {
	for _, _case := range []struct {
		IP   string
		Hit  bool
		Desc string
	}{
		{"1.2.3.255", false, ""},
		{"1.2.8.0", true, "b"},
		{"1.2.4.255", true, "a"},
		// Try to roll over to the next octet on the parse. Note the final
		// octet is overbounds. In the next case.
		{"1.2.7.256", true, "bad IP"},
		{"1.2.8.1", true, "b"},
		{"1.2.8.2", true, "eff"},
	} {
		ip := net.ParseIP(_case.IP)
		r, ok := iplist.Lookup(ip)
		assert.Equal(t, _case.Hit, ok, "%s", _case)
		if !_case.Hit {
			continue
		}
		assert.Equal(t, _case.Desc, r.Description, "%T", iplist)
	}
}

func TestSimple(t *testing.T) {
	ranges, err := sampleRanges(t)
	require.NoError(t, err)
	require.Len(t, ranges, 5)
	iplist := New(ranges)
	testLookuperSimple(t, iplist)
	packed := NewFromPacked(packedSample)
	testLookuperSimple(t, packed)
}
