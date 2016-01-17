package main

import (
	"bytes"
	"crypto/sha1"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/edsrzf/mmap-go"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/mmap_span"
)

var (
	torrentPath = flag.String("torrent", "/path/to/the.torrent", "path of the torrent file")
	dataPath    = flag.String("path", "/torrent/data", "path of the torrent data")
)

func fileToMmap(filename string, length int64, devZero *os.File) mmap.MMap {
	osFile, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	goMMap, err := mmap.MapRegion(osFile, int(length), mmap.RDONLY, mmap.COPY, 0)
	if err != nil {
		log.Fatal(err)
	}
	if int64(len(goMMap)) != length {
		log.Printf("file mmap has wrong size: %#v", filename)
	}
	osFile.Close()

	return goMMap
}

func main() {
	flag.Parse()
	metaInfo, err := metainfo.LoadFromFile(*torrentPath)
	if err != nil {
		log.Fatal(err)
	}
	devZero, err := os.Open("/dev/zero")
	if err != nil {
		log.Print(err)
	}
	defer devZero.Close()
	mMapSpan := &mmap_span.MMapSpan{}
	if len(metaInfo.Info.Files) > 0 {
		for _, file := range metaInfo.Info.Files {
			filename := filepath.Join(append([]string{*dataPath, metaInfo.Info.Name}, file.Path...)...)
			goMMap := fileToMmap(filename, file.Length, devZero)
			mMapSpan.Append(goMMap)
		}
		log.Println(len(metaInfo.Info.Files))
	} else {
		goMMap := fileToMmap(*dataPath, metaInfo.Info.Length, devZero)
		mMapSpan.Append(goMMap)
	}
	log.Println(mMapSpan.Size())
	log.Println(len(metaInfo.Info.Pieces))
	for piece := 0; piece < (len(metaInfo.Info.Pieces)+sha1.Size-1)/sha1.Size; piece++ {
		expectedHash := metaInfo.Info.Pieces[sha1.Size*piece : sha1.Size*(piece+1)]
		if len(expectedHash) == 0 {
			break
		}
		hash := sha1.New()
		_, err := mMapSpan.WriteSectionTo(hash, int64(piece)*metaInfo.Info.PieceLength, metaInfo.Info.PieceLength)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(piece, bytes.Equal(hash.Sum(nil), expectedHash))
	}
}
