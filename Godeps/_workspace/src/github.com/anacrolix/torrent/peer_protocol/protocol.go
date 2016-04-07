package peer_protocol

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
)

type (
	MessageType byte
	Integer     uint32
)

func (i *Integer) Read(r io.Reader) error {
	return binary.Read(r, binary.BigEndian, i)
}

// It's perfectly fine to cast these to an int.
func (i Integer) Int() int {
	return int(i)
}

const (
	Protocol = "\x13BitTorrent protocol"
)

const (
	Choke         MessageType = iota
	Unchoke                   // 1
	Interested                // 2
	NotInterested             // 3
	Have                      // 4
	Bitfield                  // 5
	Request                   // 6
	Piece                     // 7
	Cancel                    // 8
	Port                      // 9

	// BEP 6
	Suggest     = 0xd  // 13
	HaveAll     = 0xe  // 14
	HaveNone    = 0xf  // 15
	Reject      = 0x10 // 16
	AllowedFast = 0x11 // 17

	Extended = 20

	HandshakeExtendedID = 0

	RequestMetadataExtensionMsgType = 0
	DataMetadataExtensionMsgType    = 1
	RejectMetadataExtensionMsgType  = 2
)

type Message struct {
	Keepalive            bool
	Type                 MessageType
	Index, Begin, Length Integer
	Piece                []byte
	Bitfield             []bool
	ExtendedID           byte
	ExtendedPayload      []byte
	Port                 uint16
}

func (msg Message) MarshalBinary() (data []byte, err error) {
	buf := &bytes.Buffer{}
	if !msg.Keepalive {
		err = buf.WriteByte(byte(msg.Type))
		if err != nil {
			return
		}
		switch msg.Type {
		case Choke, Unchoke, Interested, NotInterested, HaveAll, HaveNone:
		case Have:
			err = binary.Write(buf, binary.BigEndian, msg.Index)
		case Request, Cancel, Reject:
			for _, i := range []Integer{msg.Index, msg.Begin, msg.Length} {
				err = binary.Write(buf, binary.BigEndian, i)
				if err != nil {
					break
				}
			}
		case Bitfield:
			_, err = buf.Write(marshalBitfield(msg.Bitfield))
		case Piece:
			for _, i := range []Integer{msg.Index, msg.Begin} {
				err = binary.Write(buf, binary.BigEndian, i)
				if err != nil {
					return
				}
			}
			n, err := buf.Write(msg.Piece)
			if err != nil {
				break
			}
			if n != len(msg.Piece) {
				panic(n)
			}
		case Extended:
			err = buf.WriteByte(msg.ExtendedID)
			if err != nil {
				return
			}
			_, err = buf.Write(msg.ExtendedPayload)
		case Port:
			err = binary.Write(buf, binary.BigEndian, msg.Port)
		default:
			err = fmt.Errorf("unknown message type: %v", msg.Type)
		}
	}
	data = make([]byte, 4+buf.Len())
	binary.BigEndian.PutUint32(data, uint32(buf.Len()))
	if buf.Len() != copy(data[4:], buf.Bytes()) {
		panic("bad copy")
	}
	return
}

type Decoder struct {
	R         *bufio.Reader
	MaxLength Integer // TODO: Should this include the length header or not?
}

// io.EOF is returned if the source terminates cleanly on a message boundary.
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
	b := make([]byte, length)
	_, err = io.ReadFull(d.R, b)
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		if err != io.ErrUnexpectedEOF {
			err = fmt.Errorf("error reading message: %s", err)
		}
		return
	}
	r := bytes.NewReader(b)
	// Check that all of r was utilized.
	defer func() {
		if err != nil {
			return
		}
		if r.Len() != 0 {
			err = fmt.Errorf("%d bytes unused in message type %d", r.Len(), msg.Type)
		}
	}()
	msg.Keepalive = false
	c, err := r.ReadByte()
	if err != nil {
		return
	}
	msg.Type = MessageType(c)
	switch msg.Type {
	case Choke, Unchoke, Interested, NotInterested, HaveAll, HaveNone:
		return
	case Have:
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
		msg.Piece, err = ioutil.ReadAll(r)
	case Extended:
		msg.ExtendedID, err = r.ReadByte()
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

type Bytes []byte

func (b Bytes) MarshalBinary() ([]byte, error) {
	return b, nil
}

func unmarshalBitfield(b []byte) (bf []bool) {
	for _, c := range b {
		for i := 7; i >= 0; i-- {
			bf = append(bf, (c>>uint(i))&1 == 1)
		}
	}
	return
}

func marshalBitfield(bf []bool) (b []byte) {
	b = make([]byte, (len(bf)+7)/8)
	for i, have := range bf {
		if !have {
			continue
		}
		c := b[i/8]
		c |= 1 << uint(7-i%8)
		b[i/8] = c
	}
	return
}
