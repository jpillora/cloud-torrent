package metainfo

import "fmt"

// 20-byte SHA1 hash used for info and pieces.
type Hash [20]byte

func (me Hash) Bytes() []byte {
	return me[:]
}

func (ih *Hash) AsString() string {
	return string(ih[:])
}

func (ih Hash) HexString() string {
	return fmt.Sprintf("%x", ih[:])
}
