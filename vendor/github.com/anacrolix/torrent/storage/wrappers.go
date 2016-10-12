package storage

import (
	"errors"
	"io"
	"os"

	"github.com/anacrolix/missinggo"

	"github.com/anacrolix/torrent/metainfo"
)

type Client struct {
	ClientImpl
}

func NewClient(cl ClientImpl) *Client {
	return &Client{cl}
}

func (cl Client) OpenTorrent(info *metainfo.Info, infoHash metainfo.Hash) (*Torrent, error) {
	t, err := cl.ClientImpl.OpenTorrent(info, infoHash)
	return &Torrent{t}, err
}

type Torrent struct {
	TorrentImpl
}

func (t Torrent) Piece(p metainfo.Piece) Piece {
	return Piece{t.TorrentImpl.Piece(p), p}
}

type Piece struct {
	PieceImpl
	mip metainfo.Piece
}

func (p Piece) WriteAt(b []byte, off int64) (n int, err error) {
	if p.GetIsComplete() {
		err = errors.New("piece completed")
		return
	}
	if off+int64(len(b)) > p.mip.Length() {
		panic("write overflows piece")
	}
	missinggo.LimitLen(&b, p.mip.Length()-off)
	return p.PieceImpl.WriteAt(b, off)
}

func (p Piece) ReadAt(b []byte, off int64) (n int, err error) {
	if off < 0 {
		err = os.ErrInvalid
		return
	}
	if off >= p.mip.Length() {
		err = io.EOF
		return
	}
	missinggo.LimitLen(&b, p.mip.Length()-off)
	if len(b) == 0 {
		return
	}
	n, err = p.PieceImpl.ReadAt(b, off)
	if n > len(b) {
		panic(n)
	}
	off += int64(n)
	if err == io.EOF && off < p.mip.Length() {
		err = io.ErrUnexpectedEOF
	}
	if err == nil && off >= p.mip.Length() {
		err = io.EOF
	}
	if n == 0 && err == nil {
		err = io.ErrUnexpectedEOF
	}
	if off < p.mip.Length() && err != nil {
		p.MarkNotComplete()
	}
	return
}
