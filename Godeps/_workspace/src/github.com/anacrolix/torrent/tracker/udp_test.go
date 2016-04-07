package tracker

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/url"
	"sync"
	"testing"

	_ "github.com/anacrolix/envpprof"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anacrolix/torrent/util"
)

// Ensure net.IPs are stored big-endian, to match the way they're read from
// the wire.
func TestNetIPv4Bytes(t *testing.T) {
	ip := net.IP([]byte{127, 0, 0, 1})
	if ip.String() != "127.0.0.1" {
		t.FailNow()
	}
	if string(ip) != "\x7f\x00\x00\x01" {
		t.Fatal([]byte(ip))
	}
}

func TestMarshalAnnounceResponse(t *testing.T) {
	peers := util.CompactIPv4Peers{
		{[]byte{127, 0, 0, 1}, 2},
		{[]byte{255, 0, 0, 3}, 4},
	}
	b, err := peers.MarshalBinary()
	require.NoError(t, err)
	require.EqualValues(t,
		"\x7f\x00\x00\x01\x00\x02\xff\x00\x00\x03\x00\x04",
		b)
	require.EqualValues(t, 12, binary.Size(AnnounceResponseHeader{}))
}

// Failure to write an entire packet to UDP is expected to given an error.
func TestLongWriteUDP(t *testing.T) {
	t.Parallel()
	l, err := net.ListenUDP("udp", nil)
	defer l.Close()
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.DialUDP("udp", nil, l.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for msgLen := 1; ; msgLen *= 2 {
		n, err := c.Write(make([]byte, msgLen))
		if err != nil {
			require.Contains(t, err.Error(), "message too long")
			return
		}
		if n < msgLen {
			t.FailNow()
		}
	}
}

func TestShortBinaryRead(t *testing.T) {
	var data ResponseHeader
	err := binary.Read(bytes.NewBufferString("\x00\x00\x00\x01"), binary.BigEndian, &data)
	if err != io.ErrUnexpectedEOF {
		t.FailNow()
	}
}

func TestConvertInt16ToInt(t *testing.T) {
	i := 50000
	if int(uint16(int16(i))) != 50000 {
		t.FailNow()
	}
}

func TestAnnounceLocalhost(t *testing.T) {
	t.Parallel()
	srv := server{
		t: map[[20]byte]torrent{
			[20]byte{0xa3, 0x56, 0x41, 0x43, 0x74, 0x23, 0xe6, 0x26, 0xd9, 0x38, 0x25, 0x4a, 0x6b, 0x80, 0x49, 0x10, 0xa6, 0x67, 0xa, 0xc1}: {
				Seeders:  1,
				Leechers: 2,
				Peers: []util.CompactPeer{
					{[]byte{1, 2, 3, 4}, 5},
					{[]byte{6, 7, 8, 9}, 10},
				},
			},
		},
	}
	var err error
	srv.pc, err = net.ListenPacket("udp", ":0")
	require.NoError(t, err)
	defer srv.pc.Close()
	go func() {
		require.NoError(t, srv.serveOne())
	}()
	req := AnnounceRequest{
		NumWant: -1,
		Event:   Started,
	}
	rand.Read(req.PeerId[:])
	copy(req.InfoHash[:], []uint8{0xa3, 0x56, 0x41, 0x43, 0x74, 0x23, 0xe6, 0x26, 0xd9, 0x38, 0x25, 0x4a, 0x6b, 0x80, 0x49, 0x10, 0xa6, 0x67, 0xa, 0xc1})
	go func() {
		require.NoError(t, srv.serveOne())
	}()
	ar, err := Announce(fmt.Sprintf("udp://%s/announce", srv.pc.LocalAddr().String()), &req)
	require.NoError(t, err)
	assert.EqualValues(t, 1, ar.Seeders)
	assert.EqualValues(t, 2, len(ar.Peers))
}

func TestUDPTracker(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.SkipNow()
	}
	// if err := tr.Connect(); err != nil {
	// 	if strings.Contains(err.Error(), "no such host") {
	// 		t.Skip(err)
	// 	}
	// 	if strings.Contains(err.Error(), "i/o timeout") {
	// 		t.Skip(err)
	// 	}
	// 	t.Fatal(err)
	// }
	req := AnnounceRequest{
		NumWant: -1,
		// Event:   Started,
	}
	rand.Read(req.PeerId[:])
	copy(req.InfoHash[:], []uint8{0xa3, 0x56, 0x41, 0x43, 0x74, 0x23, 0xe6, 0x26, 0xd9, 0x38, 0x25, 0x4a, 0x6b, 0x80, 0x49, 0x10, 0xa6, 0x67, 0xa, 0xc1})
	ar, err := Announce("udp://tracker.openbittorrent.com:80/announce", &req)
	if ne, ok := err.(net.Error); ok {
		if ne.Timeout() {
			t.Skip(err)
		}
	}
	require.NoError(t, err)
	t.Log(ar)
}

func TestAnnounceRandomInfoHashThirdParty(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		// This test involves contacting third party servers that may have
		// unpreditable results.
		t.SkipNow()
	}
	req := AnnounceRequest{
		Event: Stopped,
	}
	rand.Read(req.PeerId[:])
	rand.Read(req.InfoHash[:])
	wg := sync.WaitGroup{}
	success := make(chan bool)
	fail := make(chan struct{})
	for _, url := range []string{
		"udp://tracker.openbittorrent.com:80/announce",
		"udp://tracker.publicbt.com:80",
		"udp://tracker.istole.it:6969",
		"udp://tracker.ccc.de:80",
		"udp://tracker.open.demonii.com:1337",
		"udp://open.demonii.com:1337",
		"udp://exodus.desync.com:6969",
	} {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			resp, err := Announce(url, &req)
			if err != nil {
				t.Logf("error announcing to %s: %s", url, err)
				return
			}
			if resp.Leechers != 0 || resp.Seeders != 0 || len(resp.Peers) != 0 {
				// The info hash we generated was random in 2^160 space. If we
				// get a hit, something is weird.
				t.Fatal(resp)
			}
			t.Logf("announced to %s", url)
			// TODO: Can probably get stuck here, but it's just a throwaway
			// test.
			success <- true
		}(url)
	}
	go func() {
		wg.Wait()
		close(fail)
	}()
	select {
	case <-fail:
		// It doesn't matter if they all fail, the servers could just be down.
	case <-success:
		// Bail as quickly as we can. One success is enough.
	}
}

// Check that URLPath option is done correctly.
func TestURLPathOption(t *testing.T) {
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	go func() {
		_, err := Announce((&url.URL{
			Scheme: "udp",
			Host:   conn.LocalAddr().String(),
			Path:   "/announce",
		}).String(), &AnnounceRequest{})
		if err != nil {
			defer conn.Close()
		}
		require.NoError(t, err)
	}()
	var b [512]byte
	_, addr, _ := conn.ReadFrom(b[:])
	r := bytes.NewReader(b[:])
	var h RequestHeader
	read(r, &h)
	w := &bytes.Buffer{}
	write(w, ResponseHeader{
		TransactionId: h.TransactionId,
	})
	write(w, ConnectionResponse{42})
	conn.WriteTo(w.Bytes(), addr)
	n, _, _ := conn.ReadFrom(b[:])
	r = bytes.NewReader(b[:n])
	read(r, &h)
	read(r, &AnnounceRequest{})
	all, _ := ioutil.ReadAll(r)
	if string(all) != "\x02\x09/announce" {
		t.FailNow()
	}
	w = &bytes.Buffer{}
	write(w, ResponseHeader{
		TransactionId: h.TransactionId,
	})
	write(w, AnnounceResponseHeader{})
	conn.WriteTo(w.Bytes(), addr)
}
