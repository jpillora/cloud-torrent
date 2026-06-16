package server

// Test fixtures from https://webtorrent.io/free-torrents — well-seeded,
// WebSeed-backed free torrents, so downloads are deterministic. To refresh,
// copy a current magnet from that page and update the infohash/name below.
const (
	sintelInfoHash = "08ada5a7a6183aae1e09d831df6748d566095a10"
	sintelMagnet   = "magnet:?xt=urn:btih:" + sintelInfoHash + "&dn=Sintel"
	sintelName     = "Sintel"
)
