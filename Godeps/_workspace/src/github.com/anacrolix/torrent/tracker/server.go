package tracker

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"

	"github.com/anacrolix/torrent/util"
)

type torrent struct {
	Leechers int32
	Seeders  int32
	Peers    util.CompactIPv4Peers
}

type server struct {
	pc    net.PacketConn
	conns map[int64]struct{}
	t     map[[20]byte]torrent
}

func marshal(parts ...interface{}) (ret []byte, err error) {
	var buf bytes.Buffer
	for _, p := range parts {
		err = binary.Write(&buf, binary.BigEndian, p)
		if err != nil {
			return
		}
	}
	ret = buf.Bytes()
	return
}

func (me *server) respond(addr net.Addr, rh ResponseHeader, parts ...interface{}) (err error) {
	b, err := marshal(append([]interface{}{rh}, parts...)...)
	if err != nil {
		return
	}
	_, err = me.pc.WriteTo(b, addr)
	return
}

func (me *server) newConn() (ret int64) {
	ret = rand.Int63()
	if me.conns == nil {
		me.conns = make(map[int64]struct{})
	}
	me.conns[ret] = struct{}{}
	return
}

func (me *server) serveOne() (err error) {
	b := make([]byte, 0x10000)
	n, addr, err := me.pc.ReadFrom(b)
	if err != nil {
		return
	}
	r := bytes.NewReader(b[:n])
	var h RequestHeader
	err = readBody(r, &h)
	if err != nil {
		return
	}
	switch h.Action {
	case ActionConnect:
		if h.ConnectionId != connectRequestConnectionId {
			return
		}
		connId := me.newConn()
		err = me.respond(addr, ResponseHeader{
			ActionConnect,
			h.TransactionId,
		}, ConnectionResponse{
			connId,
		})
		return
	case ActionAnnounce:
		if _, ok := me.conns[h.ConnectionId]; !ok {
			me.respond(addr, ResponseHeader{
				TransactionId: h.TransactionId,
				Action:        ActionError,
			}, []byte("not connected"))
			return
		}
		var ar AnnounceRequest
		err = readBody(r, &ar)
		if err != nil {
			return
		}
		t := me.t[ar.InfoHash]
		b, err = t.Peers.MarshalBinary()
		if err != nil {
			panic(err)
		}
		err = me.respond(addr, ResponseHeader{
			TransactionId: h.TransactionId,
			Action:        ActionAnnounce,
		}, AnnounceResponseHeader{
			Interval: 900,
			Leechers: t.Leechers,
			Seeders:  t.Seeders,
		}, b)
		return
	default:
		err = fmt.Errorf("unhandled action: %d", h.Action)
		me.respond(addr, ResponseHeader{
			TransactionId: h.TransactionId,
			Action:        ActionError,
		}, []byte("unhandled action"))
		return
	}
}
