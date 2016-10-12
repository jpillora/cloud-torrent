package metainfo

import (
	"io"
	"os"
	"time"

	"github.com/anacrolix/torrent/bencode"
)

type MetaInfo struct {
	InfoBytes    bencode.Bytes `bencode:"info"`
	Announce     string        `bencode:"announce,omitempty"`
	AnnounceList [][]string    `bencode:"announce-list,omitempty"`
	Nodes        []Node        `bencode:"nodes,omitempty"`
	CreationDate int64         `bencode:"creation date,omitempty"`
	Comment      string        `bencode:"comment,omitempty"`
	CreatedBy    string        `bencode:"created by,omitempty"`
	Encoding     string        `bencode:"encoding,omitempty"`
	URLList      interface{}   `bencode:"url-list,omitempty"`
}

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

func (mi MetaInfo) UnmarshalInfo() (info Info, err error) {
	err = bencode.Unmarshal(mi.InfoBytes, &info)
	return
}

func (mi MetaInfo) HashInfoBytes() (infoHash Hash) {
	return HashBytes(mi.InfoBytes)
}

// Encode to bencoded form.
func (mi MetaInfo) Write(w io.Writer) error {
	return bencode.NewEncoder(w).Encode(mi)
}

// Set good default values in preparation for creating a new MetaInfo file.
func (mi *MetaInfo) SetDefaults() {
	mi.Comment = "yoloham"
	mi.CreatedBy = "github.com/anacrolix/torrent"
	mi.CreationDate = time.Now().Unix()
	// mi.Info.PieceLength = 256 * 1024
}

// Creates a Magnet from a MetaInfo.
func (mi *MetaInfo) Magnet(displayName string, infoHash Hash) (m Magnet) {
	for _, tier := range mi.AnnounceList {
		for _, tracker := range tier {
			m.Trackers = append(m.Trackers, tracker)
		}
	}
	if m.Trackers == nil && mi.Announce != "" {
		m.Trackers = []string{mi.Announce}
	}
	m.DisplayName = displayName
	m.InfoHash = infoHash
	return
}
