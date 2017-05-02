package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/anacrolix/tagflag"
	"github.com/bradfitz/iter"

	"github.com/anacrolix/torrent/metainfo"
)

var flags struct {
	JustName    bool
	PieceHashes bool
	Files       bool
	tagflag.StartPos
	TorrentFiles []string
}

func main() {
	tagflag.Parse(&flags)
	for _, filename := range flags.TorrentFiles {
		metainfo, err := metainfo.LoadFromFile(filename)
		if err != nil {
			log.Print(err)
			continue
		}
		info, err := metainfo.UnmarshalInfo()
		if err != nil {
			log.Printf("error unmarshalling info: %s", err)
			continue
		}
		if flags.JustName {
			fmt.Printf("%s\n", info.Name)
			continue
		}
		d := map[string]interface{}{
			"Name":         info.Name,
			"NumPieces":    info.NumPieces(),
			"PieceLength":  info.PieceLength,
			"InfoHash":     metainfo.HashInfoBytes().HexString(),
			"NumFiles":     len(info.UpvertedFiles()),
			"TotalLength":  info.TotalLength(),
			"Announce":     metainfo.Announce,
			"AnnounceList": metainfo.AnnounceList,
		}
		if flags.Files {
			d["Files"] = info.Files
		}
		if flags.PieceHashes {
			d["PieceHashes"] = func() (ret []string) {
				for i := range iter.N(info.NumPieces()) {
					ret = append(ret, hex.EncodeToString(info.Pieces[i*20:(i+1)*20]))
				}
				return
			}()
		}
		b, _ := json.MarshalIndent(d, "", "  ")
		os.Stdout.Write(b)
	}
	if !flags.JustName {
		os.Stdout.WriteString("\n")
	}
}
