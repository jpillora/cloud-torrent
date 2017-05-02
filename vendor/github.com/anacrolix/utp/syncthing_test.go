package utp

import (
	"io"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func getTCPConnectionPair() (net.Conn, net.Conn, error) {
	lst, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}

	var conn0 net.Conn
	var err0 error
	done := make(chan struct{})
	go func() {
		conn0, err0 = lst.Accept()
		close(done)
	}()

	conn1, err := net.Dial("tcp", lst.Addr().String())
	if err != nil {
		return nil, nil, err
	}

	<-done
	if err0 != nil {
		return nil, nil, err0
	}
	return conn0, conn1, nil
}

func getUTPConnectionPair() (net.Conn, net.Conn, error) {
	lst, err := NewSocket("udp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	defer lst.Close()

	var conn0 net.Conn
	var err0 error
	done := make(chan struct{})
	go func() {
		conn0, err0 = lst.Accept()
		close(done)
	}()

	conn1, err := Dial(lst.Addr().String())
	if err != nil {
		return nil, nil, err
	}

	<-done
	if err0 != nil {
		return nil, nil, err0
	}

	return conn0, conn1, nil
}

func requireWriteAll(t testing.TB, b []byte, w io.Writer) {
	n, err := w.Write(b)
	require.NoError(t, err)
	require.EqualValues(t, len(b), n)
}

func requireReadExactly(t testing.TB, b []byte, r io.Reader) {
	n, err := io.ReadFull(r, b)
	require.NoError(t, err)
	require.EqualValues(t, len(b), n)
}

func benchConnPair(b *testing.B, c0, c1 net.Conn) {
	b.ReportAllocs()
	request := make([]byte, 52)
	response := make([]byte, (128<<10)+8)
	b.SetBytes(int64(len(request) + len(response)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c, s := func() (net.Conn, net.Conn) {
			if i%2 == 0 {
				return c0, c1
			} else {
				return c1, c0
			}
		}()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			requireWriteAll(b, request, c)
			requireReadExactly(b, response[:8], c)
			requireReadExactly(b, response[8:], c)
		}()
		go func() {
			defer wg.Done()
			requireReadExactly(b, request[:8], s)
			requireReadExactly(b, request[8:], s)
			requireWriteAll(b, response, s)
		}()
		wg.Wait()
	}
}

func BenchmarkSyncthingTCP(b *testing.B) {
	conn0, conn1, err := getTCPConnectionPair()
	if err != nil {
		b.Fatal(err)
	}

	defer conn0.Close()
	defer conn1.Close()

	benchConnPair(b, conn0, conn1)
}

func BenchmarkSyncthingUDPUTP(b *testing.B) {
	conn0, conn1, err := getUTPConnectionPair()
	if err != nil {
		b.Fatal(err)
	}

	defer conn0.Close()
	defer conn1.Close()

	benchConnPair(b, conn0, conn1)
}

func BenchmarkSyncthingInprocUTP(b *testing.B) {
	c0, c1 := connPair()
	defer c0.Close()
	defer c1.Close()
	benchConnPair(b, c0, c1)
}
