package torrent

import (
	"io"
	"sync"

	pp "github.com/anacrolix/torrent/peer_protocol"
)

type ConnStats struct {
	// Torrent "piece" messages, or data chunks.
	ChunksWritten int64 // Num piece messages sent.
	ChunksRead    int64
	// Total bytes on the wire. Includes handshakes and encryption.
	BytesWritten int64
	BytesRead    int64
	// Data bytes, actual torrent data.
	DataBytesWritten int64
	DataBytesRead    int64
}

func (cs *ConnStats) wroteMsg(msg *pp.Message) {
	switch msg.Type {
	case pp.Piece:
		cs.ChunksWritten++
		cs.DataBytesWritten += int64(len(msg.Piece))
	}
}

func (cs *ConnStats) readMsg(msg *pp.Message) {
	switch msg.Type {
	case pp.Piece:
		cs.ChunksRead++
		cs.DataBytesRead += int64(len(msg.Piece))
	}
}

func (cs *ConnStats) wroteBytes(n int64) {
	cs.BytesWritten += n
}

func (cs *ConnStats) readBytes(n int64) {
	cs.BytesRead += n
}

type connStatsReadWriter struct {
	rw io.ReadWriter
	l  sync.Locker
	c  *connection
}

func (me connStatsReadWriter) Write(b []byte) (n int, err error) {
	n, err = me.rw.Write(b)
	me.l.Lock()
	me.c.wroteBytes(int64(n))
	me.l.Unlock()
	return
}

func (me connStatsReadWriter) Read(b []byte) (n int, err error) {
	n, err = me.rw.Read(b)
	me.l.Lock()
	me.c.readBytes(int64(n))
	me.l.Unlock()
	return
}
