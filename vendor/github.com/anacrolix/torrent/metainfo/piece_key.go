package metainfo

// Uniquely identifies a piece.
type PieceKey struct {
	InfoHash Hash
	Index    int
}
