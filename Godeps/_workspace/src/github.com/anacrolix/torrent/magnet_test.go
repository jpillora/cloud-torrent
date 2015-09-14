package torrent

import (
	"encoding/hex"
	"reflect"
	"testing"
)

var (
	exampleMagnetURI = `magnet:?xt=urn:btih:51340689c960f0778a4387aef9b4b52fd08390cd&dn=Shit+Movie+%281985%29+1337p+-+Eru&tr=http%3A%2F%2Fhttp.was.great%21&tr=udp%3A%2F%2Fanti.piracy.honeypot%3A6969`
	exampleMagnet    = Magnet{
		DisplayName: "Shit Movie (1985) 1337p - Eru",
		Trackers: []string{
			"http://http.was.great!",
			"udp://anti.piracy.honeypot:6969",
		},
	}
)

// Converting from our Magnet type to URL string.
func TestMagnetString(t *testing.T) {
	hex.Decode(exampleMagnet.InfoHash[:], []byte("51340689c960f0778a4387aef9b4b52fd08390cd"))
	s := exampleMagnet.String()
	if s != exampleMagnetURI {
		t.Fatalf("\nexpected:\n\t%q\nactual\n\t%q", exampleMagnetURI, s)
	}
}

func TestParseMagnetURI(t *testing.T) {
	var uri string
	var m Magnet
	var err error

	// parsing the legit Magnet URI with btih-formatted xt should not return errors
	uri = "magnet:?xt=urn:btih:ZOCMZQIPFFW7OLLMIC5HUB6BPCSDEOQU"
	_, err = ParseMagnetURI(uri)
	if err != nil {
		t.Errorf("Attempting parsing the proper Magnet btih URI:\"%v\" failed with err: %v", uri, err)
	}

	// Checking if the magnet instance struct is built correctly from parsing
	m, err = ParseMagnetURI(exampleMagnetURI)
	if err != nil || !reflect.DeepEqual(exampleMagnet, m) {
		t.Errorf("ParseMagnetURI(%s) returned %v, expected %v", uri, m, exampleMagnet)
	}

	// empty string URI case
	_, err = ParseMagnetURI("")
	if err == nil {
		t.Errorf("Parsing empty string as URI should have returned an error but didn't")
	}

	// only BTIH (BitTorrent info hash)-formatted magnet links are currently supported
	// must return error correctly when encountering other URN formats
	uri = "magnet:?xt=urn:sha1:YNCKHTQCWBTRNJIV4WNAE52SJUQCZO5C"
	_, err = ParseMagnetURI(uri)
	if err == nil {
		t.Errorf("Magnet URI with non-BTIH URNs (like \"%v\") are not supported and should return an error", uri)
	}

	// resilience to the broken hash
	uri = "magnet:?xt=urn:btih:this hash is really broken"
	_, err = ParseMagnetURI(uri)
	if err == nil {
		t.Errorf("Failed to detect broken Magnet URI: %v", uri)
	}

}
