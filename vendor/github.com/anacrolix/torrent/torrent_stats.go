package torrent

type TorrentStats struct {
	ConnStats // Aggregates stats over all connections past and present.

	ActivePeers   int
	HalfOpenPeers int
	PendingPeers  int
	TotalPeers    int
}
