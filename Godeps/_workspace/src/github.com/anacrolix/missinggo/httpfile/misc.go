package httpfile

import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/anacrolix/missinggo"
)

var (
	ErrNotFound = os.ErrNotExist
)

// Returns -1 length if it can't determined.
func instanceLength(r *http.Response) (int64, error) {
	switch r.StatusCode {
	case http.StatusOK:
		if h := r.Header.Get("Content-Length"); h != "" {
			return strconv.ParseInt(h, 10, 64)
		} else {
			return -1, nil
		}
	case http.StatusPartialContent:
		cr, ok := missinggo.ParseHTTPBytesContentRange(r.Header.Get("Content-Range"))
		if !ok {
			return -1, errors.New("bad 206 response")
		}
		return cr.Length, nil
	default:
		return -1, errors.New(r.Status)
	}
}
