package metainfo

import (
	"crypto/sha1"
	"io"
	"os"

	"github.com/anacrolix/torrent/bencode"
)

// Information specific to a single file inside the MetaInfo structure.
type FileInfo struct {
	Length int64    `bencode:"length"`
	Path   []string `bencode:"path"`
}

// Load a MetaInfo from an io.Reader. Returns a non-nil error in case of
// failure.
func Load(r io.Reader) (*MetaInfo, error) {
	var mi MetaInfo
	d := bencode.NewDecoder(r)
	err := d.Decode(&mi)
	if err != nil {
		return nil, err
	}
	return &mi, nil
}

// Convenience function for loading a MetaInfo from a file.
func LoadFromFile(filename string) (*MetaInfo, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Load(f)
}

// The info dictionary.
type Info struct {
	PieceLength int64      `bencode:"piece length"`
	Pieces      []byte     `bencode:"pieces"`
	Name        string     `bencode:"name"`
	Length      int64      `bencode:"length,omitempty"`
	Private     bool       `bencode:"private,omitempty"`
	Files       []FileInfo `bencode:"files,omitempty"`
}

func (me *Info) TotalLength() (ret int64) {
	if me.IsDir() {
		for _, fi := range me.Files {
			ret += fi.Length
		}
	} else {
		ret = me.Length
	}
	return
}

func (me *Info) NumPieces() int {
	return len(me.Pieces) / 20
}

type Piece interface {
	Hash() []byte
	Length() int64
	Offset() int64
}

type piece struct {
	Info *Info
	i    int
}

func (me piece) Length() int64 {
	if me.i == me.Info.NumPieces()-1 {
		return me.Info.TotalLength() - int64(me.i)*me.Info.PieceLength
	}
	return me.Info.PieceLength
}

func (me piece) Offset() int64 {
	return int64(me.i) * me.Info.PieceLength
}

func (me piece) Hash() []byte {
	return me.Info.Pieces[me.i*20 : (me.i+1)*20]
}

func (me *Info) Piece(i int) piece {
	return piece{me, i}
}

func (i *Info) IsDir() bool {
	return len(i.Files) != 0
}

// The files field, converted up from the old single-file in the parent info
// dict if necessary. This is a helper to avoid having to conditionally handle
// single and multi-file torrent infos.
func (i *Info) UpvertedFiles() []FileInfo {
	if len(i.Files) == 0 {
		return []FileInfo{{
			Length: i.Length,
			// Callers should determine that Info.Name is the basename, and
			// thus a regular file.
			Path: nil,
		}}
	}
	return i.Files
}

// The info dictionary with its hash and raw bytes exposed, as these are
// important to Bittorrent.
type InfoEx struct {
	Info
	Hash  []byte
	Bytes []byte
}

var (
	_ bencode.Marshaler   = InfoEx{}
	_ bencode.Unmarshaler = &InfoEx{}
)

func (this *InfoEx) UnmarshalBencode(data []byte) error {
	this.Bytes = make([]byte, 0, len(data))
	this.Bytes = append(this.Bytes, data...)
	h := sha1.New()
	_, err := h.Write(this.Bytes)
	if err != nil {
		panic(err)
	}
	this.Hash = h.Sum(nil)
	return bencode.Unmarshal(data, &this.Info)
}

func (this InfoEx) MarshalBencode() ([]byte, error) {
	if this.Bytes != nil {
		return this.Bytes, nil
	}
	return bencode.Marshal(&this.Info)
}

type MetaInfo struct {
	Info         InfoEx      `bencode:"info"`
	Announce     string      `bencode:"announce,omitempty"`
	AnnounceList [][]string  `bencode:"announce-list,omitempty"`
	Nodes        [][]string  `bencode:"nodes,omitempty"`
	CreationDate int64       `bencode:"creation date,omitempty"`
	Comment      string      `bencode:"comment,omitempty"`
	CreatedBy    string      `bencode:"created by,omitempty"`
	Encoding     string      `bencode:"encoding,omitempty"`
	URLList      interface{} `bencode:"url-list,omitempty"`
}
