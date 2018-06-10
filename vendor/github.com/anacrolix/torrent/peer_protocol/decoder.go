package peer_protocol

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"sync"
)

type Decoder struct {
	R         *bufio.Reader
	Pool      *sync.Pool
	MaxLength Integer // TODO: Should this include the length header or not?
}

// io.EOF is returned if the source terminates cleanly on a message boundary.
// TODO: Is that before or after the message?
func (d *Decoder) Decode(msg *Message) (err error) {
	var length Integer
	err = binary.Read(d.R, binary.BigEndian, &length)
	if err != nil {
		if err != io.EOF {
			err = fmt.Errorf("error reading message length: %s", err)
		}
		return
	}
	if length > d.MaxLength {
		return errors.New("message too long")
	}
	if length == 0 {
		msg.Keepalive = true
		return
	}
	msg.Keepalive = false
	r := &io.LimitedReader{d.R, int64(length)}
	// Check that all of r was utilized.
	defer func() {
		if err != nil {
			return
		}
		if r.N != 0 {
			err = fmt.Errorf("%d bytes unused in message type %d", r.N, msg.Type)
		}
	}()
	msg.Keepalive = false
	c, err := readByte(r)
	if err != nil {
		return
	}
	msg.Type = MessageType(c)
	switch msg.Type {
	case Choke, Unchoke, Interested, NotInterested, HaveAll, HaveNone:
		return
	case Have, AllowedFast, Suggest:
		err = msg.Index.Read(r)
	case Request, Cancel, Reject:
		for _, data := range []*Integer{&msg.Index, &msg.Begin, &msg.Length} {
			err = data.Read(r)
			if err != nil {
				break
			}
		}
	case Bitfield:
		b := make([]byte, length-1)
		_, err = io.ReadFull(r, b)
		msg.Bitfield = unmarshalBitfield(b)
	case Piece:
		for _, pi := range []*Integer{&msg.Index, &msg.Begin} {
			err = pi.Read(r)
			if err != nil {
				break
			}
		}
		if err != nil {
			break
		}
		//msg.Piece, err = ioutil.ReadAll(r)
		b := *d.Pool.Get().(*[]byte)
		n, err := io.ReadFull(r, b)
		if err != nil {
			if err != io.ErrUnexpectedEOF || n != int(length-9) {
				return err
			}
			b = b[0:n]
		}
		msg.Piece = b
	case Extended:
		msg.ExtendedID, err = readByte(r)
		if err != nil {
			break
		}
		msg.ExtendedPayload, err = ioutil.ReadAll(r)
	case Port:
		err = binary.Read(r, binary.BigEndian, &msg.Port)
	default:
		err = fmt.Errorf("unknown message type %#v", c)
	}
	return
}

func readByte(r io.Reader) (b byte, err error) {
	var arr [1]byte
	n, err := r.Read(arr[:])
	b = arr[0]
	if n == 1 {
		err = nil
		return
	}
	if err == nil {
		panic(err)
	}
	return
}

func unmarshalBitfield(b []byte) (bf []bool) {
	for _, c := range b {
		for i := 7; i >= 0; i-- {
			bf = append(bf, (c>>uint(i))&1 == 1)
		}
	}
	return
}
