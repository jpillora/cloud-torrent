package tracker

import (
	"testing"
)

func TestUnsupportedTrackerScheme(t *testing.T) {
	t.Parallel()
	_, err := Announce("lol://tracker.openbittorrent.com:80/announce", nil)
	if err != ErrBadScheme {
		t.Fatal(err)
	}
}
