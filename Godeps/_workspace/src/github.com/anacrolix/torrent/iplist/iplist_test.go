package iplist

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/anacrolix/missinggo"
	"github.com/bradfitz/iter"
)

var sample = `
# List distributed by iblocklist.com

a:1.2.4.0-1.2.4.255
b:1.2.8.0-1.2.8.255
something:more detail:86.59.95.195-86.59.95.195`

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

func TestBadIP(t *testing.T) {
	iplist := New(nil)
	if iplist.Lookup(net.IP(make([]byte, 4))) != nil {
		t.FailNow()
	}
	if iplist.Lookup(net.IP(make([]byte, 16))) != nil {
		t.FailNow()
	}
	if iplist.Lookup(nil) == nil {
		t.FailNow()
	}
	if iplist.Lookup(net.IP(make([]byte, 5))) == nil {
		t.FailNow()
	}
}

func TestSimple(t *testing.T) {
	ranges, err := sampleRanges(t)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranges) != 3 {
		t.Fatalf("expected 3 ranges but got %d", len(ranges))
	}
	iplist := New(ranges)
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
		{"1.2.7.256", true, "unsupported IP: <nil>"},
		{"1.2.8.254", true, "b"},
	} {
		ip := net.ParseIP(_case.IP)
		r := iplist.Lookup(ip)
		if !_case.Hit {
			if r != nil {
				t.Fatalf("got hit when none was expected: %s", ip)
			}
			continue
		}
		if r == nil {
			t.Fatalf("expected hit for %q", _case.IP)
		}
		if r.Description != _case.Desc {
			t.Fatalf("%q != %q", r.Description, _case.Desc)
		}
	}
}
