// Package iplist handles the P2P Plaintext Format described by
// https://en.wikipedia.org/wiki/PeerGuardian#P2P_plaintext_format.
package iplist

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
)

type IPList struct {
	ranges []Range
}

type Range struct {
	First, Last net.IP
	Description string
}

func (r *Range) String() string {
	return fmt.Sprintf("%s-%s (%s)", r.First, r.Last, r.Description)
}

// Create a new IP list. The given ranges must already sorted by the lower
// bound IP in each range. Behaviour is undefined for lists of overlapping
// ranges.
func New(initSorted []Range) *IPList {
	return &IPList{
		ranges: initSorted,
	}
}

func (me *IPList) NumRanges() int {
	if me == nil {
		return 0
	}
	return len(me.ranges)
}

// Return the range the given IP is in. Returns nil if no range is found.
func (me *IPList) Lookup(ip net.IP) (r *Range) {
	if me == nil {
		return nil
	}
	// TODO: Perhaps all addresses should be converted to IPv6, if the future
	// of IP is to always be backwards compatible. But this will cost 4x the
	// memory for IPv4 addresses?
	v4 := ip.To4()
	if v4 != nil {
		r = me.lookup(v4)
		if r != nil {
			return
		}
	}
	v6 := ip.To16()
	if v6 != nil {
		return me.lookup(v6)
	}
	if v4 == nil && v6 == nil {
		return &Range{
			Description: fmt.Sprintf("unsupported IP: %s", ip),
		}
	}
	return nil
}

// Return the range the given IP is in. Returns nil if no range is found.
func (me *IPList) lookup(ip net.IP) (r *Range) {
	// Find the index of the first range for which the following range exceeds
	// it.
	i := sort.Search(len(me.ranges), func(i int) bool {
		if i+1 >= len(me.ranges) {
			return true
		}
		return bytes.Compare(ip, me.ranges[i+1].First) < 0
	})
	if i == len(me.ranges) {
		return
	}
	r = &me.ranges[i]
	if bytes.Compare(ip, r.First) < 0 || bytes.Compare(ip, r.Last) > 0 {
		r = nil
	}
	return
}

func minifyIP(ip *net.IP) {
	v4 := ip.To4()
	if v4 != nil {
		*ip = append(make([]byte, 0, 4), v4...)
	}
}

// Parse a line of the PeerGuardian Text Lists (P2P) Format. Returns !ok but
// no error if a line doesn't contain a range but isn't erroneous, such as
// comment and blank lines.
func ParseBlocklistP2PLine(l []byte) (r Range, ok bool, err error) {
	l = bytes.TrimSpace(l)
	if len(l) == 0 || bytes.HasPrefix(l, []byte("#")) {
		return
	}
	// TODO: Check this when IPv6 blocklists are available.
	colon := bytes.LastIndexAny(l, ":")
	if colon == -1 {
		err = errors.New("missing colon")
		return
	}
	hyphen := bytes.IndexByte(l[colon+1:], '-')
	if hyphen == -1 {
		err = errors.New("missing hyphen")
		return
	}
	hyphen += colon + 1
	r.Description = string(l[:colon])
	r.First = net.ParseIP(string(l[colon+1 : hyphen]))
	minifyIP(&r.First)
	r.Last = net.ParseIP(string(l[hyphen+1:]))
	minifyIP(&r.Last)
	if r.First == nil || r.Last == nil || len(r.First) != len(r.Last) {
		err = errors.New("bad IP range")
		return
	}
	ok = true
	return
}

// Creates an IPList from a line-delimited P2P Plaintext file.
func NewFromReader(f io.Reader) (ret *IPList, err error) {
	var ranges []Range
	// There's a lot of similar descriptions, so we maintain a pool and reuse
	// them to reduce memory overhead.
	uniqStrs := make(map[string]string)
	scanner := bufio.NewScanner(f)
	lineNum := 1
	for scanner.Scan() {
		r, ok, lineErr := ParseBlocklistP2PLine(scanner.Bytes())
		if lineErr != nil {
			err = fmt.Errorf("error parsing line %d: %s", lineNum, lineErr)
			return
		}
		lineNum++
		if !ok {
			continue
		}
		if s, ok := uniqStrs[r.Description]; ok {
			r.Description = s
		} else {
			uniqStrs[r.Description] = r.Description
		}
		ranges = append(ranges, r)
	}
	err = scanner.Err()
	if err != nil {
		return
	}
	ret = New(ranges)
	return
}
