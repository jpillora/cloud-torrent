package missinggo

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileInfoAccessTime(t *testing.T) {
	f, err := ioutil.TempFile("", "")
	require.NoError(t, err)
	assert.NoError(t, f.Close())
	name := f.Name()
	t.Log(name)
	defer func() {
		err := os.Remove(name)
		if err != nil {
			t.Log(err)
		}
	}()
	fi, err := os.Stat(name)
	require.NoError(t, err)
	t.Log(FileInfoAccessTime(fi))
}
