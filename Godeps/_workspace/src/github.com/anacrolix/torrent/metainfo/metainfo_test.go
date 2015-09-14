package metainfo

import (
	"bytes"
	"path"
	"testing"

	"github.com/anacrolix/torrent/bencode"
)

func test_file(t *testing.T, filename string) {
	mi, err := LoadFromFile(filename)
	if err != nil {
		t.Fatal(err)
	}

	if len(mi.Info.Files) == 1 {
		t.Logf("Single file: %s (length: %d)\n", mi.Info.Name, mi.Info.Files[0].Length)
	} else {
		t.Logf("Multiple files: %s\n", mi.Info.Name)
		for _, f := range mi.Info.Files {
			t.Logf(" - %s (length: %d)\n", path.Join(f.Path...), f.Length)
		}
	}

	for _, group := range mi.AnnounceList {
		for _, tracker := range group {
			t.Logf("Tracker: %s\n", tracker)
		}
	}
	// for _, url := range mi.WebSeedURLs {
	// 	t.Logf("URL: %s\n", url)
	// }

	b, err := bencode.Marshal(mi.Info)
	if !bytes.Equal(b, mi.Info.Bytes) {
		t.Logf("\n%q\n%q", b[len(b)-20:], mi.Info.Bytes[len(mi.Info.Bytes)-20:])
		t.Fatal("encoded and decoded bytes don't match")
	}
}

func TestFile(t *testing.T) {
	test_file(t, "_testdata/archlinux-2011.08.19-netinstall-i686.iso.torrent")
	test_file(t, "_testdata/continuum.torrent")
	test_file(t, "_testdata/23516C72685E8DB0C8F15553382A927F185C4F01.torrent")
	test_file(t, "_testdata/trackerless.torrent")
}
