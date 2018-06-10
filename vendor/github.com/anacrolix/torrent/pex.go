package torrent

import "github.com/anacrolix/dht/krpc"

type peerExchangeMessage struct {
	Added       krpc.CompactIPv4NodeAddrs `bencode:"added"`
	AddedFlags  []pexPeerFlags            `bencode:"added.f"`
	Added6      krpc.CompactIPv6NodeAddrs `bencode:"added6"`
	Added6Flags []pexPeerFlags            `bencode:"added6.f"`
	Dropped     krpc.CompactIPv4NodeAddrs `bencode:"dropped"`
	Dropped6    krpc.CompactIPv6NodeAddrs `bencode:"dropped6"`
}

type pexPeerFlags byte

func (me pexPeerFlags) Get(f pexPeerFlags) bool {
	return me&f == f
}

const (
	pexPrefersEncryption = 0x01
	pexSeedUploadOnly    = 0x02
	pexSupportsUtp       = 0x04
	pexHolepunchSupport  = 0x08
	pexOutgoingConn      = 0x10
)

func (me *peerExchangeMessage) AddedPeers() (ret Peers) {
	ret.FromPex(me.Added, me.AddedFlags)
	ret.FromPex(me.Added6, me.Added6Flags)
	return
}
