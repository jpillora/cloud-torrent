// +build !go1.6

package archive

import "io"

// NewCompressedZipWriter returns archive compressed with DEFLATE data format.
// The level parameter has no effect before Go 1.6.
func NewCompressedZipWriter(dst io.Writer, level int) *Archive {
	a := &Archive{
		DirMaxSize:  -1,
		DirMaxFiles: -1,
		dst:         dst,
	}
	a.archive = newCompressedZipArchive(dst)
	return a
}
