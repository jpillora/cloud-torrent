package dht

import (
	"expvar"
)

var (
	read               = expvar.NewInt("dhtRead")
	readZeroPort       = expvar.NewInt("dhtReadZeroPort")
	readBlocked        = expvar.NewInt("dhtReadBlocked")
	readNotKRPCDict    = expvar.NewInt("dhtReadNotKRPCDict")
	readUnmarshalError = expvar.NewInt("dhtReadUnmarshalError")
	readQuery          = expvar.NewInt("dhtReadQuery")
	readQueryBad       = expvar.NewInt("dhtQueryBad")
	readAnnouncePeer   = expvar.NewInt("dhtReadAnnouncePeer")
	announceErrors     = expvar.NewInt("dhtAnnounceErrors")
	writeErrors        = expvar.NewInt("dhtWriteErrors")
	writes             = expvar.NewInt("dhtWrites")
	readInvalidToken   = expvar.NewInt("dhtReadInvalidToken")
)
