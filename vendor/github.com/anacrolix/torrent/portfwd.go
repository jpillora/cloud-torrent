package torrent

import (
	"log"
	"time"

	flog "github.com/anacrolix/log"
	"github.com/syncthing/syncthing/lib/nat"
	"github.com/syncthing/syncthing/lib/upnp"
)

func addPortMapping(d nat.Device, proto nat.Protocol, internalPort int, debug bool) {
	externalPort, err := d.AddPortMapping(proto, internalPort, internalPort, "anacrolix/torrent", 0)
	if err != nil {
		log.Printf("error adding %s port mapping: %s", proto, err)
	} else if externalPort != internalPort {
		log.Printf("external port %d does not match internal port %d in port mapping", externalPort, internalPort)
	} else if debug {
		log.Printf("forwarded external %s port %d", proto, externalPort)
	}
}

func (cl *Client) forwardPort() {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.config.NoDefaultPortForwarding {
		return
	}
	cl.mu.Unlock()
	ds := upnp.Discover(0, 2*time.Second)
	cl.mu.Lock()
	flog.Default.Handle(flog.Fmsg("discovered %d upnp devices", len(ds)))
	port := cl.incomingPeerPort()
	cl.mu.Unlock()
	for _, d := range ds {
		go addPortMapping(d, nat.TCP, port, cl.config.Debug)
		go addPortMapping(d, nat.UDP, port, cl.config.Debug)
	}
	cl.mu.Lock()
}
