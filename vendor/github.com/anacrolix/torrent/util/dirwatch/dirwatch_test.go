package dirwatch

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestDirwatch(t *testing.T) {
	tempDirName, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDirName)
	t.Logf("tempdir: %q", tempDirName)
	dw, err := New(tempDirName)
	defer dw.Close()
	if err != nil {
		t.Fatal(err)
	}
}
