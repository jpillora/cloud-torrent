package torrent

import "strings"

type peerNetworks struct {
	tcp4, tcp6 bool
	utp4, utp6 bool
}

func handleErr(h func(), fs ...func() error) error {
	for _, f := range fs {
		err := f()
		if err != nil {
			h()
			return err
		}
	}
	return nil
}

func LoopbackListenHost(network string) string {
	if strings.Contains(network, "4") {
		return "127.0.0.1"
	} else {
		return "::1"
	}
}
