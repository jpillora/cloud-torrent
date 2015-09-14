package tracker

import (
	"testing"
)

func TestUnsupportedTrackerScheme(t *testing.T) {
	_, err := New("lol://tracker.openbittorrent.com:80/announce")
	if err != ErrBadScheme {
		t.Fatal(err)
	}
}
