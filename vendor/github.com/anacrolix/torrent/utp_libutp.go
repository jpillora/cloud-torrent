// +build cgo,!disable_libutp

package torrent

import (
	"github.com/anacrolix/go-libutp"
)

func NewUtpSocket(network, addr string) (utpSocket, error) {
	s, err := utp.NewSocket(network, addr)
	if s == nil {
		return nil, err
	} else {
		return s, err
	}
}
