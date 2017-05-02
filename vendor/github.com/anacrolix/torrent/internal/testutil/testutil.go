// Package testutil contains stuff for testing torrent-related behaviour.
//
// "greeting" is a single-file torrent of a file called "greeting" that
// "contains "hello, world\n".

package testutil

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/anacrolix/missinggo"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

const (
	GreetingFileContents = "hello, world\n"
	GreetingFileName     = "greeting"
)

func CreateDummyTorrentData(dirName string) string {
	f, _ := os.Create(filepath.Join(dirName, "greeting"))
	defer f.Close()
	f.WriteString(GreetingFileContents)
	return f.Name()
}

func GreetingMetaInfo() *metainfo.MetaInfo {
	info := metainfo.Info{
		Name:        GreetingFileName,
		Length:      int64(len(GreetingFileContents)),
		PieceLength: 5,
	}
	err := info.GeneratePieces(func(metainfo.FileInfo) (io.ReadCloser, error) {
		return ioutil.NopCloser(strings.NewReader(GreetingFileContents)), nil
	})
	if err != nil {
		panic(err)
	}
	mi := &metainfo.MetaInfo{}
	mi.InfoBytes, err = bencode.Marshal(info)
	if err != nil {
		panic(err)
	}
	return mi
}

// Gives a temporary directory containing the completed "greeting" torrent,
// and a corresponding metainfo describing it. The temporary directory can be
// cleaned away with os.RemoveAll.
func GreetingTestTorrent() (tempDir string, metaInfo *metainfo.MetaInfo) {
	tempDir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		panic(err)
	}
	CreateDummyTorrentData(tempDir)
	metaInfo = GreetingMetaInfo()
	return
}

type StatusWriter interface {
	WriteStatus(io.Writer)
}

func ExportStatusWriter(sw StatusWriter, path string) {
	http.HandleFunc(
		fmt.Sprintf("/%s/%s", missinggo.GetTestName(), path),
		func(w http.ResponseWriter, r *http.Request) {
			sw.WriteStatus(w)
		},
	)
}
