package mse

import (
	"bytes"
	"crypto/rand"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"testing"

	"github.com/bradfitz/iter"
)

func TestReadUntil(t *testing.T) {
	test := func(data, until string, leftover int, expectedErr error) {
		r := bytes.NewReader([]byte(data))
		err := readUntil(r, []byte(until))
		if err != expectedErr {
			t.Fatal(err)
		}
		if r.Len() != leftover {
			t.Fatal(r.Len())
		}
	}
	test("feakjfeafeafegbaabc00", "abc", 2, nil)
	test("feakjfeafeafegbaadc00", "abc", 0, io.EOF)
}

func TestSuffixMatchLen(t *testing.T) {
	test := func(a, b string, expected int) {
		actual := suffixMatchLen([]byte(a), []byte(b))
		if actual != expected {
			t.Fatalf("expected %d, got %d for %q and %q", expected, actual, a, b)
		}
	}
	test("hello", "world", 0)
	test("hello", "lo", 2)
	test("hello", "llo", 3)
	test("hello", "hell", 0)
	test("hello", "helloooo!", 5)
	test("hello", "lol!", 2)
	test("hello", "mondo", 0)
	test("mongo", "webscale", 0)
	test("sup", "person", 1)
}

func handshakeTest(t testing.TB, ia []byte, aData, bData string) {
	a, b := net.Pipe()
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		a, err := InitiateHandshake(a, []byte("yep"), ia)
		if err != nil {
			t.Fatal(err)
			return
		}
		go a.Write([]byte(aData))

		var msg [20]byte
		n, _ := a.Read(msg[:])
		if n != len(bData) {
			t.FailNow()
		}
		// t.Log(string(msg[:n]))
	}()
	go func() {
		defer wg.Done()
		b, err := ReceiveHandshake(b, [][]byte{[]byte("nope"), []byte("yep"), []byte("maybe")})
		if err != nil {
			t.Fatal(err)
			return
		}
		go b.Write([]byte(bData))
		// Need to be exact here, as there are several reads, and net.Pipe is
		// most synchronous.
		msg := make([]byte, len(ia)+len(aData))
		n, _ := io.ReadFull(b, msg[:])
		if n != len(msg) {
			t.FailNow()
		}
		// t.Log(string(msg[:n]))
	}()
	wg.Wait()
	a.Close()
	b.Close()
}

func allHandshakeTests(t testing.TB) {
	handshakeTest(t, []byte("jump the gun, "), "hello world", "yo dawg")
	handshakeTest(t, nil, "hello world", "yo dawg")
	handshakeTest(t, []byte{}, "hello world", "yo dawg")
}

func TestHandshake(t *testing.T) {
	allHandshakeTests(t)
	t.Logf("crypto provides encountered: %s", cryptoProvidesCount)
}

func BenchmarkHandshake(b *testing.B) {
	for range iter.N(b.N) {
		allHandshakeTests(b)
	}
}

type trackReader struct {
	r io.Reader
	n int64
}

func (me *trackReader) Read(b []byte) (n int, err error) {
	n, err = me.r.Read(b)
	me.n += int64(n)
	return
}

func TestReceiveRandomData(t *testing.T) {
	tr := trackReader{rand.Reader, 0}
	ReceiveHandshake(readWriter{&tr, ioutil.Discard}, nil)
}
