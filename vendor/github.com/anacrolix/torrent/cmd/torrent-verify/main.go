package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/anacrolix/tagflag"
	"github.com/bradfitz/iter"
	"github.com/edsrzf/mmap-go"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/mmap_span"
)

func mmapFile(name string) (mm mmap.MMap, err error) {
	f, err := os.Open(name)
	if err != nil {
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return
	}
	if fi.Size() == 0 {
		return
	}
	return mmap.MapRegion(f, -1, mmap.RDONLY, mmap.COPY, 0)
}

func verifyTorrent(info *metainfo.Info, root string) error {
	span := new(mmap_span.MMapSpan)
	for _, file := range info.UpvertedFiles() {
		filename := filepath.Join(append([]string{root, info.Name}, file.Path...)...)
		mm, err := mmapFile(filename)
		if err != nil {
			return err
		}
		if int64(len(mm)) != file.Length {
			return fmt.Errorf("file %q has wrong length", filename)
		}
		span.Append(mm)
	}
	for i := range iter.N(info.NumPieces()) {
		p := info.Piece(i)
		hash := sha1.New()
		_, err := io.Copy(hash, io.NewSectionReader(span, p.Offset(), p.Length()))
		if err != nil {
			return err
		}
		good := bytes.Equal(hash.Sum(nil), p.Hash().Bytes())
		if !good {
			return fmt.Errorf("hash mismatch at piece %d", i)
		}
		fmt.Printf("%d: %x: %v\n", i, p.Hash(), good)
	}
	return nil
}

func main() {
	log.SetFlags(log.Flags() | log.Lshortfile)
	var flags = struct {
		DataDir string
		tagflag.StartPos
		TorrentFile string
	}{}
	tagflag.Parse(&flags)
	metaInfo, err := metainfo.LoadFromFile(flags.TorrentFile)
	if err != nil {
		log.Fatal(err)
	}
	info, err := metaInfo.UnmarshalInfo()
	if err != nil {
		log.Fatalf("error unmarshalling info: %s", err)
	}
	err = verifyTorrent(&info, flags.DataDir)
	if err != nil {
		log.Fatalf("torrent failed verification: %s", err)
	}
}
