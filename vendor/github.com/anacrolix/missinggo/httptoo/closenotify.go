package httptoo

import (
	"context"
	"net/http"
)

var closeNotifyContextKey = new(struct{})

func RequestWithCloseNotify(r *http.Request, w http.ResponseWriter) *http.Request {
	if r.Context().Value(closeNotifyContextKey) != nil {
		return r
	}
	v := make(chan struct{})
	r = r.WithContext(context.WithValue(r.Context(), closeNotifyContextKey, v))
	cn := w.(http.CloseNotifier).CloseNotify()
	go func() {
		select {
		case <-cn:
			close(v)
		case <-r.Context().Done():
		}
	}()
	return r
}

func RequestCloseNotifyValue(r *http.Request) <-chan struct{} {
	return r.Context().Value(closeNotifyContextKey).(chan struct{})
}
