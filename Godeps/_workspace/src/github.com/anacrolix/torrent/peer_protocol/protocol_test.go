package peer_protocol

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestBinaryReadSliceOfPointers(t *testing.T) {
	var msg Message
	r := bytes.NewBufferString("\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00@\x00")
	if r.Len() != 12 {
		t.Fatalf("expected 12 bytes left, but there %d", r.Len())
	}
	for _, data := range []*Integer{&msg.Index, &msg.Begin, &msg.Length} {
		err := data.Read(r)
		if err != nil {
			t.Fatal(err)
		}
	}
	if r.Len() != 0 {
		t.FailNow()
	}
}

func TestConstants(t *testing.T) {
	// check that iota works as expected in the const block
	if NotInterested != 3 {
		t.FailNow()
	}
}

func TestBitfieldEncode(t *testing.T) {
	bf := make([]bool, 37)
	bf[2] = true
	bf[7] = true
	bf[32] = true
	s := string(marshalBitfield(bf))
	const expected = "\x21\x00\x00\x00\x80"
	if s != expected {
		t.Fatalf("got %#v, expected %#v", s, expected)
	}
}

func TestBitfieldUnmarshal(t *testing.T) {
	bf := unmarshalBitfield([]byte("\x81\x06"))
	expected := make([]bool, 16)
	expected[0] = true
	expected[7] = true
	expected[13] = true
	expected[14] = true
	if len(bf) != len(expected) {
		t.FailNow()
	}
	for i := range expected {
		if bf[i] != expected[i] {
			t.FailNow()
		}
	}
}

func TestHaveEncode(t *testing.T) {
	actualBytes, err := Message{
		Type:  Have,
		Index: 42,
	}.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	actualString := string(actualBytes)
	expected := "\x00\x00\x00\x05\x04\x00\x00\x00\x2a"
	if actualString != expected {
		t.Fatalf("expected %#v, got %#v", expected, actualString)
	}
}

func TestShortRead(t *testing.T) {
	dec := Decoder{
		R:         bufio.NewReader(bytes.NewBufferString("\x00\x00\x00\x02\x00!")),
		MaxLength: 2,
	}
	msg := new(Message)
	err := dec.Decode(msg)
	if !strings.Contains(err.Error(), "1 bytes unused in message type 0") {
		t.Fatal(err)
	}
}

func TestUnexpectedEOF(t *testing.T) {
	msg := new(Message)
	for _, stream := range []string{
		"\x00\x00\x00",     // Header truncated.
		"\x00\x00\x00\x01", // Expecting 1 more byte.
		// Request with wrong length, and too short anyway.
		"\x00\x00\x00\x06\x06\x00\x00\x00\x00\x00",
		// Request truncated.
		"\x00\x00\x00\x0b\x06\x00\x00\x00\x00\x00",
	} {
		dec := Decoder{
			R:         bufio.NewReader(bytes.NewBufferString(stream)),
			MaxLength: 42,
		}
		err := dec.Decode(msg)
		if err == nil {
			t.Fatalf("expected an error decoding %q", stream)
		}
	}
}

func TestMarshalKeepalive(t *testing.T) {
	b, err := (Message{
		Keepalive: true,
	}).MarshalBinary()
	if err != nil {
		t.Fatalf("error marshalling keepalive: %s", err)
	}
	bs := string(b)
	const expected = "\x00\x00\x00\x00"
	if bs != expected {
		t.Fatalf("marshalled keepalive is %q, expected %q", bs, expected)
	}
}

func TestMarshalPortMsg(t *testing.T) {
	b, err := (Message{
		Type: Port,
		Port: 0xaabb,
	}).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "\x00\x00\x00\x03\x09\xaa\xbb" {
		t.FailNow()
	}
}

func TestUnmarshalPortMsg(t *testing.T) {
	var m Message
	d := Decoder{
		R:         bufio.NewReader(bytes.NewBufferString("\x00\x00\x00\x03\x09\xaa\xbb")),
		MaxLength: 8,
	}
	err := d.Decode(&m)
	if err != nil {
		t.Fatal(err)
	}
	if m.Type != Port {
		t.FailNow()
	}
	if m.Port != 0xaabb {
		t.FailNow()
	}
}
