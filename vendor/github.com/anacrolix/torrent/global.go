package torrent

import (
	"crypto"
	"expvar"
	"time"
)

const (
	pieceHash        = crypto.SHA1
	maxRequests      = 250    // Maximum pending requests we allow peers to send us.
	defaultChunkSize = 0x4000 // 16KiB

	// Updated occasionally to when there's been some changes to client
	// behaviour in case other clients are assuming anything of us. See also
	// `bep20`.
	extendedHandshakeClientVersion = "go.torrent dev 20150624"
	// Peer ID client identifier prefix. We'll update this occasionally to
	// reflect changes to client behaviour that other clients may depend on.
	// Also see `extendedHandshakeClientVersion`.
	bep20 = "-GT0001-"

	nominalDialTimeout = time.Second * 30
	minDialTimeout     = 5 * time.Second

	// Justification for set bits follows.
	//
	// Extension protocol ([5]|=0x10):
	// http://www.bittorrent.org/beps/bep_0010.html
	//
	// Fast Extension ([7]|=0x04):
	// http://bittorrent.org/beps/bep_0006.html.
	// Disabled until AllowedFast is implemented.
	//
	// DHT ([7]|=1):
	// http://www.bittorrent.org/beps/bep_0005.html
	defaultExtensionBytes = "\x00\x00\x00\x00\x00\x10\x00\x01"

	defaultEstablishedConnsPerTorrent = 80
	defaultHalfOpenConnsPerTorrent    = 80
	torrentPeersHighWater             = 200
	torrentPeersLowWater              = 50

	// Limit how long handshake can take. This is to reduce the lingering
	// impact of a few bad apples. 4s loses 1% of successful handshakes that
	// are obtained with 60s timeout, and 5% of unsuccessful handshakes.
	handshakesTimeout = 20 * time.Second

	// These are our extended message IDs. Peers will use these values to
	// select which extension a message is intended for.
	metadataExtendedId = iota + 1 // 0 is reserved for deleting keys
	pexExtendedId
)

// I could move a lot of these counters to their own file, but I suspect they
// may be attached to a Client someday.
var (
	unwantedChunksReceived   = expvar.NewInt("chunksReceivedUnwanted")
	unexpectedChunksReceived = expvar.NewInt("chunksReceivedUnexpected")
	chunksReceived           = expvar.NewInt("chunksReceived")

	peersAddedBySource = expvar.NewMap("peersAddedBySource")

	uploadChunksPosted = expvar.NewInt("uploadChunksPosted")
	unexpectedCancels  = expvar.NewInt("unexpectedCancels")
	postedCancels      = expvar.NewInt("postedCancels")

	pieceHashedCorrect    = expvar.NewInt("pieceHashedCorrect")
	pieceHashedNotCorrect = expvar.NewInt("pieceHashedNotCorrect")

	unsuccessfulDials = expvar.NewInt("dialSuccessful")
	successfulDials   = expvar.NewInt("dialUnsuccessful")

	acceptUTP    = expvar.NewInt("acceptUTP")
	acceptTCP    = expvar.NewInt("acceptTCP")
	acceptReject = expvar.NewInt("acceptReject")

	peerExtensions                    = expvar.NewMap("peerExtensions")
	completedHandshakeConnectionFlags = expvar.NewMap("completedHandshakeConnectionFlags")
	// Count of connections to peer with same client ID.
	connsToSelf = expvar.NewInt("connsToSelf")
	// Number of completed connections to a client we're already connected with.
	duplicateClientConns       = expvar.NewInt("duplicateClientConns")
	receivedMessageTypes       = expvar.NewMap("receivedMessageTypes")
	receivedKeepalives         = expvar.NewInt("receivedKeepalives")
	supportedExtensionMessages = expvar.NewMap("supportedExtensionMessages")
	postedMessageTypes         = expvar.NewMap("postedMessageTypes")
	postedKeepalives           = expvar.NewInt("postedKeepalives")
	// Requests received for pieces we don't have.
	requestsReceivedForMissingPieces = expvar.NewInt("requestsReceivedForMissingPieces")

	// Track the effectiveness of Torrent.connPieceInclinationPool.
	pieceInclinationsReused = expvar.NewInt("pieceInclinationsReused")
	pieceInclinationsNew    = expvar.NewInt("pieceInclinationsNew")
	pieceInclinationsPut    = expvar.NewInt("pieceInclinationsPut")
)
