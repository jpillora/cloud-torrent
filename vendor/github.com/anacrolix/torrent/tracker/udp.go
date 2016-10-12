package tracker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/url"
	"time"

	"github.com/anacrolix/missinggo"
	"github.com/anacrolix/missinggo/pproffd"

	"github.com/anacrolix/torrent/util"
)

type Action int32

const (
	ActionConnect Action = iota
	ActionAnnounce
	ActionScrape
	ActionError

	connectRequestConnectionId = 0x41727101980

	// BEP 41
	optionTypeEndOfOptions = 0
	optionTypeNOP          = 1
	optionTypeURLData      = 2
)

type ConnectionRequest struct {
	ConnectionId int64
	Action       int32
	TransctionId int32
}

type ConnectionResponse struct {
	ConnectionId int64
}

type ResponseHeader struct {
	Action        Action
	TransactionId int32
}

type RequestHeader struct {
	ConnectionId  int64
	Action        Action
	TransactionId int32
} // 16 bytes

type AnnounceResponseHeader struct {
	Interval int32
	Leechers int32
	Seeders  int32
}

func newTransactionId() int32 {
	return int32(rand.Uint32())
}

func timeout(contiguousTimeouts int) (d time.Duration) {
	if contiguousTimeouts > 8 {
		contiguousTimeouts = 8
	}
	d = 15 * time.Second
	for ; contiguousTimeouts > 0; contiguousTimeouts-- {
		d *= 2
	}
	return
}

type udpAnnounce struct {
	contiguousTimeouts   int
	connectionIdReceived time.Time
	connectionId         int64
	socket               net.Conn
	url                  url.URL
}

func (c *udpAnnounce) Close() error {
	if c.socket != nil {
		return c.socket.Close()
	}
	return nil
}

func (c *udpAnnounce) Do(req *AnnounceRequest) (res AnnounceResponse, err error) {
	err = c.connect()
	if err != nil {
		return
	}
	reqURI := c.url.RequestURI()
	// Clearly this limits the request URI to 255 bytes. BEP 41 supports
	// longer but I'm not fussed.
	options := append([]byte{optionTypeURLData, byte(len(reqURI))}, []byte(reqURI)...)
	b, err := c.request(ActionAnnounce, req, options)
	if err != nil {
		return
	}
	var h AnnounceResponseHeader
	err = readBody(b, &h)
	if err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		err = fmt.Errorf("error parsing announce response: %s", err)
		return
	}
	res.Interval = h.Interval
	res.Leechers = h.Leechers
	res.Seeders = h.Seeders
	cps, err := util.UnmarshalIPv4CompactPeers(b.Bytes())
	if err != nil {
		return
	}
	for _, cp := range cps {
		res.Peers = append(res.Peers, Peer{
			IP:   cp.IP[:],
			Port: int(cp.Port),
		})
	}
	return
}

// body is the binary serializable request body. trailer is optional data
// following it, such as for BEP 41.
func (c *udpAnnounce) write(h *RequestHeader, body interface{}, trailer []byte) (err error) {
	var buf bytes.Buffer
	err = binary.Write(&buf, binary.BigEndian, h)
	if err != nil {
		panic(err)
	}
	if body != nil {
		err = binary.Write(&buf, binary.BigEndian, body)
		if err != nil {
			panic(err)
		}
	}
	_, err = buf.Write(trailer)
	if err != nil {
		return
	}
	n, err := c.socket.Write(buf.Bytes())
	if err != nil {
		return
	}
	if n != buf.Len() {
		panic("write should send all or error")
	}
	return
}

func read(r io.Reader, data interface{}) error {
	return binary.Read(r, binary.BigEndian, data)
}

func write(w io.Writer, data interface{}) error {
	return binary.Write(w, binary.BigEndian, data)
}

// args is the binary serializable request body. trailer is optional data
// following it, such as for BEP 41.
func (c *udpAnnounce) request(action Action, args interface{}, options []byte) (responseBody *bytes.Buffer, err error) {
	tid := newTransactionId()
	err = c.write(&RequestHeader{
		ConnectionId:  c.connectionId,
		Action:        action,
		TransactionId: tid,
	}, args, options)
	if err != nil {
		return
	}
	c.socket.SetReadDeadline(time.Now().Add(timeout(c.contiguousTimeouts)))
	b := make([]byte, 0x800) // 2KiB
	for {
		var n int
		n, err = c.socket.Read(b)
		if opE, ok := err.(*net.OpError); ok {
			if opE.Timeout() {
				c.contiguousTimeouts++
				return
			}
		}
		if err != nil {
			return
		}
		buf := bytes.NewBuffer(b[:n])
		var h ResponseHeader
		err = binary.Read(buf, binary.BigEndian, &h)
		switch err {
		case io.ErrUnexpectedEOF:
			continue
		case nil:
		default:
			return
		}
		if h.TransactionId != tid {
			continue
		}
		c.contiguousTimeouts = 0
		if h.Action == ActionError {
			err = errors.New(buf.String())
		}
		responseBody = buf
		return
	}
}

func readBody(r io.Reader, data ...interface{}) (err error) {
	for _, datum := range data {
		err = binary.Read(r, binary.BigEndian, datum)
		if err != nil {
			break
		}
	}
	return
}

func (c *udpAnnounce) connected() bool {
	return !c.connectionIdReceived.IsZero() && time.Now().Before(c.connectionIdReceived.Add(time.Minute))
}

func (c *udpAnnounce) connect() (err error) {
	if c.connected() {
		return nil
	}
	c.connectionId = connectRequestConnectionId
	if c.socket == nil {
		hmp := missinggo.SplitHostMaybePort(c.url.Host)
		if hmp.NoPort {
			hmp.NoPort = false
			hmp.Port = 80
		}
		c.socket, err = net.Dial("udp", hmp.String())
		if err != nil {
			return
		}
		c.socket = pproffd.WrapNetConn(c.socket)
	}
	b, err := c.request(ActionConnect, nil, nil)
	if err != nil {
		return
	}
	var res ConnectionResponse
	err = readBody(b, &res)
	if err != nil {
		return
	}
	c.connectionId = res.ConnectionId
	c.connectionIdReceived = time.Now()
	return
}

func announceUDP(ar *AnnounceRequest, _url *url.URL) (AnnounceResponse, error) {
	ua := udpAnnounce{
		url: *_url,
	}
	defer ua.Close()
	return ua.Do(ar)
}
