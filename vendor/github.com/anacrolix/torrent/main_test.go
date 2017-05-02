package torrent

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	// select {}
	os.Exit(code)
}
