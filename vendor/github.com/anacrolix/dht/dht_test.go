package dht

import (
	"encoding/hex"
	"log"
	"math/big"
	"math/rand"
	"net"
	"testing"
	"time"

	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/missinggo/iter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/dht/krpc"
)

func TestSetNilBigInt(t *testing.T) {
	i := new(big.Int)
	i.SetBytes(make([]byte, 2))
}

func TestMarshalCompactNodeInfo(t *testing.T) {
	cni := krpc.NodeInfo{
		ID: [20]byte{'a', 'b', 'c'},
	}
	addr, err := net.ResolveUDPAddr("udp4", "1.2.3.4:5")
	require.NoError(t, err)
	cni.Addr = addr
	var b [krpc.CompactIPv4NodeInfoLen]byte
	err = cni.PutCompact(b[:])
	require.NoError(t, err)
	var bb [26]byte
	copy(bb[:], []byte("abc"))
	copy(bb[20:], []byte("\x01\x02\x03\x04\x00\x05"))
	assert.EqualValues(t, bb, b)
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
	log.SetFlags(log.Flags() | log.Lshortfile)
	for _, s := range []string{
		zeroID,
		"\x03" + zeroID[1:],
		"\x03" + zeroID[1:18] + "\x55\xf0",
		"\x55" + zeroID[1:17] + "\xff\x55\x0f",
		"\x54" + zeroID[1:18] + "\x50\x0f",
	} {
		testIDs = append(testIDs, nodeIDFromString(s))
	}
	testIDs = append(testIDs, nodeID{})
}

func TestDistances(t *testing.T) {
	expectBitcount := func(i int160, count int) {
		if bitCount(i.Bytes()) != count {
			t.Fatalf("expected bitcount of %d: got %d", count, bitCount(i.Bytes()))
		}
	}
	expectBitcount(testIDs[3].Distance(&testIDs[0]), 4+8+4+4)
	expectBitcount(testIDs[3].Distance(&testIDs[1]), 4+8+4+4)
	expectBitcount(testIDs[3].Distance(&testIDs[2]), 4+8+8)
	var max int160
	max.SetMax()
	for i := 0; i < 5; i++ {
		dist := testIDs[i].Distance(&testIDs[5])
		if dist.Cmp(max) != 0 {
			t.Fatal("expected max distance for comparison with unset node id")
		}
	}
}

func TestMaxDistanceString(t *testing.T) {
	var max int160
	max.SetMax()
	require.EqualValues(t, "\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff", max.Bytes())
}

func TestClosestNodes(t *testing.T) {
	cn := newKClosestNodeIDs(2, testIDs[3])
	for _, i := range rand.Perm(len(testIDs)) {
		cn.Push(testIDs[i])
	}
	ids := iter.ToSlice(cn.IDs())
	assert.Len(t, ids, 2)
	m := map[string]bool{}
	for _, id := range ids {
		m[id.(nodeID).ByteString()] = true
	}
	log.Printf("%q", m)
	assert.True(t, m[testIDs[3].ByteString()])
	assert.True(t, m[testIDs[4].ByteString()])
}

func TestDHTDefaultConfig(t *testing.T) {
	s, err := NewServer(nil)
	assert.NoError(t, err)
	s.Close()
}

func TestPing(t *testing.T) {
	srv, err := NewServer(&ServerConfig{
		Addr:               "127.0.0.1:5680",
		NoDefaultBootstrap: true,
	})
	require.NoError(t, err)
	defer srv.Close()
	srv0, err := NewServer(&ServerConfig{
		Addr:           "127.0.0.1:5681",
		BootstrapNodes: []string{"127.0.0.1:5680"},
	})
	require.NoError(t, err)
	defer srv0.Close()
	tn, err := srv.Ping(&net.UDPAddr{
		IP:   []byte{127, 0, 0, 1},
		Port: srv0.Addr().(*net.UDPAddr).Port,
	})
	require.NoError(t, err)
	defer tn.Close()
	ok := make(chan bool)
	tn.SetResponseHandler(func(msg krpc.Msg, msgOk bool) {
		ok <- msg.SenderID() == srv0.ID()
	})
	require.True(t, <-ok)
}

func TestServerCustomNodeId(t *testing.T) {
	customId := "5a3ce1c14e7a08645677bbd1cfe7d8f956d53256"
	id, err := hex.DecodeString(customId)
	assert.NoError(t, err)
	// How to test custom *secure* Id when tester computers will have
	// different Ids? Generate custom ids for local IPs and use mini-Id?
	s, err := NewServer(&ServerConfig{
		NodeIdHex:          customId,
		NoDefaultBootstrap: true,
	})
	require.NoError(t, err)
	defer s.Close()
	assert.Equal(t, string(id), s.ID())
}

func TestAnnounceTimeout(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	s, err := NewServer(&ServerConfig{
		BootstrapNodes: []string{"1.2.3.4:5"},
	})
	require.NoError(t, err)
	var ih [20]byte
	copy(ih[:], "12341234123412341234")
	a, err := s.Announce(ih, 0, true)
	assert.NoError(t, err)
	<-a.Peers
	a.Close()
	s.Close()
}

func TestEqualPointers(t *testing.T) {
	assert.EqualValues(t, &krpc.Msg{R: &krpc.Return{}}, &krpc.Msg{R: &krpc.Return{}})
}

func TestHook(t *testing.T) {
	t.Log("TestHook: Starting with Ping intercept/passthrough")
	srv, err := NewServer(&ServerConfig{
		Addr:               "127.0.0.1:5678",
		NoDefaultBootstrap: true,
	})
	require.NoError(t, err)
	defer srv.Close()
	// Establish server with a hook attached to "ping"
	hookCalled := make(chan bool)
	srv0, err := NewServer(&ServerConfig{
		Addr:           "127.0.0.1:5679",
		BootstrapNodes: []string{"127.0.0.1:5678"},
		OnQuery: func(m *krpc.Msg, addr net.Addr) bool {
			if m.Q == "ping" {
				hookCalled <- true
			}
			return true
		},
	})
	require.NoError(t, err)
	defer srv0.Close()
	// Ping srv0 from srv to trigger hook. Should also receive a response.
	t.Log("TestHook: Servers created, hook for ping established. Calling Ping.")
	tn, err := srv.Ping(&net.UDPAddr{
		IP:   []byte{127, 0, 0, 1},
		Port: srv0.Addr().(*net.UDPAddr).Port,
	})
	assert.NoError(t, err)
	defer tn.Close()
	// Await response from hooked server
	tn.SetResponseHandler(func(msg krpc.Msg, b bool) {
		t.Log("TestHook: Sender received response from pinged hook server, so normal execution resumed.")
	})
	// Await signal that hook has been called.
	select {
	case <-hookCalled:
		{
			// Success, hook was triggered. Todo: Ensure that "ok" channel
			// receives, also, indicating normal handling proceeded also.
			t.Log("TestHook: Received ping, hook called and returned to normal execution!")
			return
		}
	case <-time.After(time.Second * 1):
		{
			t.Error("Failed to see evidence of ping hook being called after 2 seconds.")
		}
	}
}

// Check that address resolution doesn't rat out invalid SendTo addr
// arguments.
func TestResolveBadAddr(t *testing.T) {
	ua, err := net.ResolveUDPAddr("udp", "0.131.255.145:33085")
	require.NoError(t, err)
	assert.False(t, validNodeAddr(NewAddr(ua)))
}
