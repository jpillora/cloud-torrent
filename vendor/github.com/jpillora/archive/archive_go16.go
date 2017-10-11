// +build go1.6

package archive

import (
	"archive/zip"
	"compress/flate"
	"io"
)

// NewCompressedZipWriter returns archive compressed with DEFLATE data format
func NewCompressedZipWriter(dst io.Writer, level int) *Archive {
	a := &Archive{
		DirMaxSize:  -1,
		DirMaxFiles: -1,
		dst:         dst,
	}

	czip := newCompressedZipArchive(dst)
	czip.writer.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, level)
	})

	a.archive = czip
	return a
}
