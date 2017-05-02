package torrent_test

import (
	"log"

	"github.com/anacrolix/missinggo"

	"github.com/anacrolix/torrent"
)

func Example() {
	c, _ := torrent.NewClient(nil)
	defer c.Close()
	t, _ := c.AddMagnet("magnet:?xt=urn:btih:ZOCMZQIPFFW7OLLMIC5HUB6BPCSDEOQU")
	<-t.GotInfo()
	t.DownloadAll()
	c.WaitAll()
	log.Print("ermahgerd, torrent downloaded")
}

func Example_fileReader() {
	var (
		t *torrent.Torrent
		f torrent.File
	)
	r := t.NewReader()
	defer r.Close()
	// Access the parts of the torrent pertaining to f. Data will be
	// downloaded as required, per the configuration of the torrent.Reader.
	_ = missinggo.NewSectionReadSeeker(r, f.Offset(), f.Length())
}
