package missinggo

import "net/http"

// A http.ResponseWriter that tracks the status of the response. The status
// code, and number of bytes written for example.
type StatusResponseWriter struct {
	http.ResponseWriter
	Code         int
	BytesWritten int64
}

func (me *StatusResponseWriter) Write(b []byte) (n int, err error) {
	if me.Code == 0 {
		me.Code = 200
	}
	n, err = me.ResponseWriter.Write(b)
	me.BytesWritten += int64(n)
	return
}

func (me *StatusResponseWriter) WriteHeader(code int) {
	me.ResponseWriter.WriteHeader(code)
	me.Code = code
}
