package tracker

import (
	"testing"
)

func TestUnsupportedTrackerScheme(t *testing.T) {
	t.Parallel()
	_, err := New("lol://tracker.openbittorrent.com:80/announce")
	if err != ErrBadScheme {
		t.Fatal(err)
	}
}
