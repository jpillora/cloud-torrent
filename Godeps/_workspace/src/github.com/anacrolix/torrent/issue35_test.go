package torrent

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/anacrolix/torrent/dht"
	"github.com/anacrolix/torrent/metainfo"
)

var numclients int = 0

func cfdir() string {
	numclients++
	return filepath.Join(os.TempDir(), "wtp-test/", fmt.Sprintf("%d/", numclients))
}

func addirs(cf *Config) *Config {
	d := cfdir()
	os.MkdirAll(d, 0700)
	cf.DataDir = filepath.Join(d, "/data")
	os.MkdirAll(cf.DataDir, 0700)
	cf.ConfigDir = filepath.Join(d, "/config")
	os.MkdirAll(cf.ConfigDir, 0700)
	return cf
}

func issue35TestingConfig() *Config {
	return &Config{
		ListenAddr:           "localhost:0",
		NoDHT:                false,
		DisableTrackers:      true,
		DisableUTP:           false,
		DisableMetainfoCache: true,
		DisableIPv6:          true,
		NoUpload:             false,
		Seed:                 true,
		DataDir:              filepath.Join(os.TempDir(), "torrent-test/data"),
		ConfigDir:            filepath.Join(os.TempDir(), "torrent-test/config"),
		DHTConfig: dht.ServerConfig{
			Passive:            false,
			BootstrapNodes:     []string{},
			NoSecurity:         false,
			NoDefaultBootstrap: true,
		},
		Debug: true,
	}
}

func writeranddata(path string) error {
	var w int64
	var to_write int64 = 1024 * 1024 //1MB
	f, err := os.Create(path)
	defer f.Close()
	if err != nil {
		return err
	}
	rnd, err := os.Open("/dev/urandom")
	defer rnd.Close()
	if err != nil {
		return err
	}
	w, err = io.CopyN(f, rnd, to_write)
	if err != nil {
		return err
	}
	if w != to_write {
		return errors.New("Short read on /dev/random")
	}
	return nil
}

func TestInfohash(t *testing.T) {
	os.RemoveAll(filepath.Join(os.TempDir(), "torrent-test"))
	os.MkdirAll(filepath.Join(os.TempDir(), "torrent-test"), 0700)
	var cl_one *Client
	var cf_one *Config
	var err error
	if err != nil {
		t.Fatal(err)
	}
	cf_one = issue35TestingConfig()
	cf_one.ListenAddr = "localhost:43433"
	cf_one = addirs(cf_one)
	cl_one, err = NewClient(cf_one)
	if err != nil {
		t.Fatal(err)
	}
	tfp := filepath.Join(cf_one.DataDir, "testdata")
	writeranddata(tfp)
	b := metainfo.Builder{}
	b.AddFile(tfp)
	b.AddDhtNodes([]string{"1.2.3.4:5555"})
	ba, err := b.Submit()
	if err != nil {
		t.Fatal(err)
	}
	ttfp := filepath.Join(cf_one.ConfigDir, "/../torrent")
	ttf, err := os.Create(ttfp)
	if err != nil {
		t.Fatal(err)
	}
	ec, _ := ba.Start(ttf, runtime.NumCPU())
	err = <-ec
	if err != nil {
		t.Fatal(err)
	}
	ttf.Close()

	tor, err := cl_one.AddTorrentFromFile(ttfp)
	if err != nil {
		t.Fatal(err)
	}
	<-tor.GotInfo()
	tor.DownloadAll()
	if cl_one.WaitAll() == false {
		t.Fatal(errors.New("One did not download torrent"))
	}
}
