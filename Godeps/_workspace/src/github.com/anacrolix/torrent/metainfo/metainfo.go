package metainfo

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/missinggo"

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

func (info *Info) BuildFromFilePath(root string) (err error) {
	info.Name = filepath.Base(root)
	info.Files = nil
	err = filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		log.Println(path, root, err)
		if fi.IsDir() {
			// Directories are implicit in torrent files.
			return nil
		} else if path == root {
			// The root is a file.
			info.Length = fi.Size()
			return nil
		}
		relPath, err := filepath.Rel(root, path)
		log.Println(relPath, err)
		if err != nil {
			return fmt.Errorf("error getting relative path: %s", err)
		}
		info.Files = append(info.Files, FileInfo{
			Path:   strings.Split(relPath, string(filepath.Separator)),
			Length: fi.Size(),
		})
		return nil
	})
	if err != nil {
		return
	}
	err = info.GeneratePieces(func(fi FileInfo) (io.ReadCloser, error) {
		return os.Open(filepath.Join(root, strings.Join(fi.Path, string(filepath.Separator))))
	})
	if err != nil {
		err = fmt.Errorf("error generating pieces: %s", err)
	}
	return
}

func (info *Info) writeFiles(w io.Writer, open func(fi FileInfo) (io.ReadCloser, error)) error {
	for _, fi := range info.UpvertedFiles() {
		r, err := open(fi)
		if err != nil {
			return fmt.Errorf("error opening %v: %s", fi, err)
		}
		wn, err := io.CopyN(w, r, fi.Length)
		r.Close()
		if wn != fi.Length || err != nil {
			return fmt.Errorf("error hashing %v: %s", fi, err)
		}
	}
	return nil
}

// Set info.Pieces by hashing info.Files.
func (info *Info) GeneratePieces(open func(fi FileInfo) (io.ReadCloser, error)) error {
	if info.PieceLength == 0 {
		return errors.New("piece length must be non-zero")
	}
	pr, pw := io.Pipe()
	go func() {
		err := info.writeFiles(pw, open)
		pw.CloseWithError(err)
	}()
	defer pr.Close()
	var pieces []byte
	for {
		hasher := sha1.New()
		wn, err := io.CopyN(hasher, pr, info.PieceLength)
		if err == io.EOF {
			err = nil
		}
		if err != nil {
			return err
		}
		if wn == 0 {
			break
		}
		pieces = hasher.Sum(pieces)
		if wn < info.PieceLength {
			break
		}
	}
	info.Pieces = pieces
	return nil
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
	if len(me.Pieces)%20 != 0 {
		panic(len(me.Pieces))
	}
	return len(me.Pieces) / 20
}

func (me *InfoEx) Piece(i int) Piece {
	return Piece{me, i}
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
	Hash  *Hash
	Bytes []byte
}

var (
	_ bencode.Marshaler   = InfoEx{}
	_ bencode.Unmarshaler = &InfoEx{}
)

func (this *InfoEx) UnmarshalBencode(data []byte) error {
	this.Bytes = append(make([]byte, 0, len(data)), data...)
	h := sha1.New()
	_, err := h.Write(this.Bytes)
	if err != nil {
		panic(err)
	}
	this.Hash = new(Hash)
	missinggo.CopyExact(this.Hash, h.Sum(nil))
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
	Nodes        []Node      `bencode:"nodes,omitempty"`
	CreationDate int64       `bencode:"creation date,omitempty"`
	Comment      string      `bencode:"comment,omitempty"`
	CreatedBy    string      `bencode:"created by,omitempty"`
	Encoding     string      `bencode:"encoding,omitempty"`
	URLList      interface{} `bencode:"url-list,omitempty"`
}

// Encode to bencoded form.
func (mi *MetaInfo) Write(w io.Writer) error {
	return bencode.NewEncoder(w).Encode(mi)
}

// Set good default values in preparation for creating a new MetaInfo file.
func (mi *MetaInfo) SetDefaults() {
	mi.Comment = "yoloham"
	mi.CreatedBy = "github.com/anacrolix/torrent"
	mi.CreationDate = time.Now().Unix()
	mi.Info.PieceLength = 256 * 1024
}

// Magnetize creates a Magnet from a MetaInfo
func (mi *MetaInfo) Magnet() (m Magnet) {
	for _, tier := range mi.AnnounceList {
		for _, tracker := range tier {
			m.Trackers = append(m.Trackers, tracker)
		}
	}
	m.DisplayName = mi.Info.Name
	m.InfoHash = *mi.Info.Hash
	return
}
