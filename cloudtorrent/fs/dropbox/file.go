package dropbox

import "github.com/spf13/afero/mem"

type file struct {
	*mem.File
}
