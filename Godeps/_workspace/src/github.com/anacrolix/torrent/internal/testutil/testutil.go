// Package testutil contains stuff for testing torrent-related behaviour.
//
// "greeting" is a single-file torrent of a file called "greeting" that
// "contains "hello, world\n".

package testutil

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"github.com/anacrolix/missinggo"

	"github.com/anacrolix/torrent/metainfo"
)

const GreetingFileContents = "hello, world\n"

func CreateDummyTorrentData(dirName string) string {
	f, _ := os.Create(filepath.Join(dirName, "greeting"))
	defer f.Close()
	f.WriteString(GreetingFileContents)
	return f.Name()
}

// Writes to w, a metainfo containing the file at name.
func CreateMetaInfo(name string, w io.Writer) {
	var mi metainfo.MetaInfo
	mi.Info.Name = filepath.Base(name)
	fi, _ := os.Stat(name)
	mi.Info.Length = fi.Size()
	mi.Announce = "lol://cheezburger"
	mi.Info.PieceLength = 5
	err := mi.Info.GeneratePieces(func(metainfo.FileInfo) (io.ReadCloser, error) {
		return os.Open(name)
	})
	if err != nil {
		panic(err)
	}
	err = mi.Write(w)
	if err != nil {
		panic(err)
	}
}

// Gives a temporary directory containing the completed "greeting" torrent,
// and a corresponding metainfo describing it. The temporary directory can be
// cleaned away with os.RemoveAll.
func GreetingTestTorrent() (tempDir string, metaInfo *metainfo.MetaInfo) {
	tempDir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		panic(err)
	}
	name := CreateDummyTorrentData(tempDir)
	w := &bytes.Buffer{}
	CreateMetaInfo(name, w)
	metaInfo, _ = metainfo.Load(w)
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
