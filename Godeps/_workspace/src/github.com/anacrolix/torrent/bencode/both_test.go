package bencode

import "testing"
import "bytes"
import "io/ioutil"

func load_file(name string, t *testing.T) []byte {
	data, err := ioutil.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func test_file_interface(t *testing.T, filename string) {
	data1 := load_file(filename, t)
	var iface interface{}

	err := Unmarshal(data1, &iface)
	if err != nil {
		t.Fatal(err)
	}

	data2, err := Marshal(iface)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(data1, data2) {
		t.Fatalf("equality expected\n")
	}

}

func TestBothInterface(t *testing.T) {
	test_file_interface(t, "_testdata/archlinux-2011.08.19-netinstall-i686.iso.torrent")
	test_file_interface(t, "_testdata/continuum.torrent")
}

type torrent_file struct {
	Info struct {
		Name        string `bencode:"name"`
		Length      int64  `bencode:"length"`
		MD5Sum      string `bencode:"md5sum,omitempty"`
		PieceLength int64  `bencode:"piece length"`
		Pieces      string `bencode:"pieces"`
		Private     bool   `bencode:"private,omitempty"`
	} `bencode:"info"`

	Announce     string      `bencode:"announce"`
	AnnounceList [][]string  `bencode:"announce-list,omitempty"`
	CreationDate int64       `bencode:"creation date,omitempty"`
	Comment      string      `bencode:"comment,omitempty"`
	CreatedBy    string      `bencode:"created by,omitempty"`
	URLList      interface{} `bencode:"url-list,omitempty"`
}

func test_file(t *testing.T, filename string) {
	data1 := load_file(filename, t)
	var f torrent_file

	err := Unmarshal(data1, &f)
	if err != nil {
		t.Fatal(err)
	}

	data2, err := Marshal(&f)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(data1, data2) {
		println(string(data2))
		t.Fatalf("equality expected")
	}
}

func TestBoth(t *testing.T) {
	test_file(t, "_testdata/archlinux-2011.08.19-netinstall-i686.iso.torrent")
}
