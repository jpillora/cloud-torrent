package tracker

import (
	"errors"
	"net"
	"net/url"
)

// Marshalled as binary by the UDP client, so be careful making changes.
type AnnounceRequest struct {
	InfoHash   [20]byte
	PeerId     [20]byte
	Downloaded int64
	Left       uint64
	Uploaded   int64
	// Apparently this is optional. None can be used for announces done at
	// regular intervals.
	Event     AnnounceEvent
	IPAddress int32
	Key       int32
	NumWant   int32 // How many peer addresses are desired. -1 for default.
	Port      uint16
} // 82 bytes

type AnnounceResponse struct {
	Interval int32 // Minimum seconds the local peer should wait before next announce.
	Leechers int32
	Seeders  int32
	Peers    []Peer
}

type AnnounceEvent int32

func (e AnnounceEvent) String() string {
	// See BEP 3, "event".
	return []string{"empty", "completed", "started", "stopped"}[e]
}

type Peer struct {
	IP   net.IP
	Port int
}

const (
	None      AnnounceEvent = iota
	Completed               // The local peer just completed the torrent.
	Started                 // The local peer has just resumed this torrent.
	Stopped                 // The local peer is leaving the swarm.
)

var (
	ErrBadScheme = errors.New("unknown scheme")
)

func Announce(urlStr string, req *AnnounceRequest) (res AnnounceResponse, err error) {
	return AnnounceHost(urlStr, req, "")
}

func AnnounceHost(urlStr string, req *AnnounceRequest, host string) (res AnnounceResponse, err error) {
	_url, err := url.Parse(urlStr)
	if err != nil {
		return
	}
	switch _url.Scheme {
	case "http", "https":
		return announceHTTP(req, _url, host)
	case "udp":
		return announceUDP(req, _url)
	default:
		err = ErrBadScheme
		return
	}
}
