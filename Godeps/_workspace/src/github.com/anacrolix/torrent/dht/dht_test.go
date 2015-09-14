package dht

import (
	"encoding/hex"
	"math/big"
	"math/rand"
	"net"
	"testing"

	"github.com/anacrolix/missinggo"
	"github.com/stretchr/testify/assert"
)

func TestSetNilBigInt(t *testing.T) {
	i := new(big.Int)
	i.SetBytes(make([]byte, 2))
}

func TestMarshalCompactNodeInfo(t *testing.T) {
	cni := NodeInfo{
		ID: [20]byte{'a', 'b', 'c'},
	}
	addr, err := net.ResolveUDPAddr("udp4", "1.2.3.4:5")
	if err != nil {
		t.Fatal(err)
	}
	cni.Addr = newDHTAddr(addr)
	var b [CompactNodeInfoLen]byte
	cni.PutCompact(b[:])
	if err != nil {
		t.Fatal(err)
	}
	var bb [26]byte
	copy(bb[:], []byte("abc"))
	copy(bb[20:], []byte("\x01\x02\x03\x04\x00\x05"))
	if b != bb {
		t.FailNow()
	}
}

func recoverPanicOrDie(t *testing.T, f func()) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
	}()
	f()
}

const zeroID = "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"

var testIDs []nodeID

func init() {
	for _, s := range []string{
		zeroID,
		"\x03" + zeroID[1:],
		"\x03" + zeroID[1:18] + "\x55\xf0",
		"\x55" + zeroID[1:17] + "\xff\x55\x0f",
		"\x54" + zeroID[1:18] + "\x50\x0f",
		"",
	} {
		testIDs = append(testIDs, nodeIDFromString(s))
	}
}

func TestDistances(t *testing.T) {
	expectBitcount := func(i big.Int, count int) {
		if bitCount(i) != count {
			t.Fatalf("expected bitcount of %d: got %d", count, bitCount(i))
		}
	}
	expectBitcount(testIDs[3].Distance(&testIDs[0]), 4+8+4+4)
	expectBitcount(testIDs[3].Distance(&testIDs[1]), 4+8+4+4)
	expectBitcount(testIDs[3].Distance(&testIDs[2]), 4+8+8)
	for i := 0; i < 5; i++ {
		dist := testIDs[i].Distance(&testIDs[5])
		if dist.Cmp(&maxDistance) != 0 {
			t.Fatal("expected max distance for comparison with unset node id")
		}
	}
}

func TestMaxDistanceString(t *testing.T) {
	if string(maxDistance.Bytes()) != "\x01"+zeroID {
		t.FailNow()
	}
}

func TestClosestNodes(t *testing.T) {
	cn := newKClosestNodesSelector(2, testIDs[3])
	for _, i := range rand.Perm(len(testIDs)) {
		cn.Push(testIDs[i])
	}
	if len(cn.IDs()) != 2 {
		t.FailNow()
	}
	m := map[string]bool{}
	for _, id := range cn.IDs() {
		m[id.ByteString()] = true
	}
	if !m[testIDs[3].ByteString()] || !m[testIDs[4].ByteString()] {
		t.FailNow()
	}
}

func TestUnmarshalGetPeersResponse(t *testing.T) {
	gpr := Msg{
		"r": map[string]interface{}{
			"values": []interface{}{"\x01\x02\x03\x04\x05\x06", "\x07\x08\x09\x0a\x0b\x0c"},
			"nodes":  "\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x02\x03\x04\x05\x06\x07\x08\x09\x02\x03\x04\x05\x06\x07" + "\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x02\x03\x04\x05\x06\x07\x08\x09\x02\x03\x04\x05\x06\x07",
		},
	}
	assert.EqualValues(t, 2, len(gpr.Values()))
	assert.EqualValues(t, 2, len(gpr.Nodes()))
}

func TestDHTDefaultConfig(t *testing.T) {
	s, err := NewServer(nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
}

func TestPing(t *testing.T) {
	srv, err := NewServer(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	srv0, err := NewServer(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv0.Close()
	tn, err := srv.Ping(&net.UDPAddr{
		IP:   []byte{127, 0, 0, 1},
		Port: srv0.Addr().(*net.UDPAddr).Port,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tn.Close()
	ok := make(chan bool)
	tn.SetResponseHandler(func(msg Msg) {
		ok <- msg.SenderID() == srv0.ID()
	})
	if !<-ok {
		t.FailNow()
	}
}

func TestDHTSec(t *testing.T) {
	for _, case_ := range []struct {
		ipStr     string
		nodeIDHex string
		valid     bool
	}{
		// These 5 are from the spec example. They are all valid.
		{"124.31.75.21", "5fbfbff10c5d6a4ec8a88e4c6ab4c28b95eee401", true},
		{"21.75.31.124", "5a3ce9c14e7a08645677bbd1cfe7d8f956d53256", true},
		{"65.23.51.170", "a5d43220bc8f112a3d426c84764f8c2a1150e616", true},
		{"84.124.73.14", "1b0321dd1bb1fe518101ceef99462b947a01ff41", true},
		{"43.213.53.83", "e56f6cbf5b7c4be0237986d5243b87aa6d51305a", true},
		// spec[0] with one of the rand() bytes changed. Valid.
		{"124.31.75.21", "5fbfbff10c5d7a4ec8a88e4c6ab4c28b95eee401", true},
		// spec[1] with the 21st leading bit changed. Not Valid.
		{"21.75.31.124", "5a3ce1c14e7a08645677bbd1cfe7d8f956d53256", false},
		// spec[2] with the 22nd leading bit changed. Valid.
		{"65.23.51.170", "a5d43620bc8f112a3d426c84764f8c2a1150e616", true},
		// spec[3] with the 4th last bit changed. Valid.
		{"84.124.73.14", "1b0321dd1bb1fe518101ceef99462b947a01fe01", true},
		// spec[4] with the 3rd last bit changed. Not valid.
		{"43.213.53.83", "e56f6cbf5b7c4be0237986d5243b87aa6d51303e", false},
	} {
		ip := net.ParseIP(case_.ipStr)
		id, err := hex.DecodeString(case_.nodeIDHex)
		if err != nil {
			t.Fatal(err)
		}
		secure := NodeIdSecure(string(id), ip)
		if secure != case_.valid {
			t.Fatalf("case failed: %v", case_)
		}
		if !secure {
			SecureNodeId(id, ip)
			if !NodeIdSecure(string(id), ip) {
				t.Fatal("failed to secure node id")
			}
		}
	}
}

func TestServerDefaultNodeIdSecure(t *testing.T) {
	s, err := NewServer(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !NodeIdSecure(s.ID(), missinggo.AddrIP(s.Addr())) {
		t.Fatal("not secure")
	}
}
