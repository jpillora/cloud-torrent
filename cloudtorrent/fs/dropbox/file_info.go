package dropbox

import (
	"os"
	"time"

	dropbox "github.com/tj/go-dropbox"
)

// fileInfo wraps Dropbox file MetaData to implement os.FileInfo.
type fileInfo struct {
	meta *dropbox.Metadata
}

// Name of the file.
func (f *fileInfo) Name() string {
	return f.meta.Name
}

// Size of the file.
func (f *fileInfo) Size() int64 {
	return int64(f.meta.Size)
}

// IsDir returns true if the file is a directory.
func (f *fileInfo) IsDir() bool {
	return f.meta.Tag == "folder"
}

// Sys is not implemented.
func (f *fileInfo) Sys() interface{} {
	return nil
}

// ModTime returns the modification time.
func (f *fileInfo) ModTime() time.Time {
	return f.meta.ServerModified
}

// Mode returns the file mode flags.
func (f *fileInfo) Mode() os.FileMode {
	var m os.FileMode
	if f.IsDir() {
		m |= os.ModeDir
	}
	return m
}
