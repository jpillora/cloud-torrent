package torrent

import (
	"crypto"
	"expvar"
)

const (
	pieceHash        = crypto.SHA1
	maxRequests      = 250    // Maximum pending requests we allow peers to send us.
	defaultChunkSize = 0x4000 // 16KiB

	// These are our extended message IDs. Peers will use these values to
	// select which extension a message is intended for.
	metadataExtendedId = iota + 1 // 0 is reserved for deleting keys
	pexExtendedId
)

func defaultPeerExtensionBytes() peerExtensionBytes {
	return newPeerExtensionBytes(ExtensionBitDHT, ExtensionBitExtended, ExtensionBitFast)
}

// I could move a lot of these counters to their own file, but I suspect they
// may be attached to a Client someday.
var (
	unwantedChunksReceived   = expvar.NewInt("chunksReceivedUnwanted")
	unexpectedChunksReceived = expvar.NewInt("chunksReceivedUnexpected")
	chunksReceived           = expvar.NewInt("chunksReceived")

	torrent = expvar.NewMap("torrent")

	peersAddedBySource = expvar.NewMap("peersAddedBySource")

	pieceHashedCorrect    = expvar.NewInt("pieceHashedCorrect")
	pieceHashedNotCorrect = expvar.NewInt("pieceHashedNotCorrect")

	unsuccessfulDials = expvar.NewInt("dialSuccessful")
	successfulDials   = expvar.NewInt("dialUnsuccessful")

	peerExtensions                    = expvar.NewMap("peerExtensions")
	completedHandshakeConnectionFlags = expvar.NewMap("completedHandshakeConnectionFlags")
	// Count of connections to peer with same client ID.
	connsToSelf = expvar.NewInt("connsToSelf")
	// Number of completed connections to a client we're already connected with.
	duplicateClientConns       = expvar.NewInt("duplicateClientConns")
	receivedKeepalives         = expvar.NewInt("receivedKeepalives")
	supportedExtensionMessages = expvar.NewMap("supportedExtensionMessages")
	postedKeepalives           = expvar.NewInt("postedKeepalives")
	// Requests received for pieces we don't have.
	requestsReceivedForMissingPieces = expvar.NewInt("requestsReceivedForMissingPieces")
	requestedChunkLengths            = expvar.NewMap("requestedChunkLengths")

	messageTypesReceived = expvar.NewMap("messageTypesReceived")
	messageTypesSent     = expvar.NewMap("messageTypesSent")
	messageTypesPosted   = expvar.NewMap("messageTypesPosted")

	// Track the effectiveness of Torrent.connPieceInclinationPool.
	pieceInclinationsReused = expvar.NewInt("pieceInclinationsReused")
	pieceInclinationsNew    = expvar.NewInt("pieceInclinationsNew")
	pieceInclinationsPut    = expvar.NewInt("pieceInclinationsPut")

	fillBufferSentCancels  = expvar.NewInt("fillBufferSentCancels")
	fillBufferSentRequests = expvar.NewInt("fillBufferSentRequests")
	numFillBuffers         = expvar.NewInt("numFillBuffers")
)
