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
		info := &metainfo.Info.Info
		if flags.JustName {
			fmt.Printf("%s\n", metainfo.Info.Name)
			continue
		}
		d := map[string]interface{}{
			"Name":        info.Name,
			"NumPieces":   info.NumPieces(),
			"PieceLength": info.PieceLength,
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
