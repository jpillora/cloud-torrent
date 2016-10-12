package httptoo

import "net/http"

type Middleware func(http.Handler) http.Handler

func WrapHandler(middleware []Middleware, h http.Handler) (ret http.Handler) {
	ret = h
	for i := range middleware {
		ret = middleware[len(middleware)-1-i](ret)
	}
	return
}

func WrapHandlerFunc(middleware []Middleware, hf func(http.ResponseWriter, *http.Request)) (ret http.Handler) {
	return WrapHandler(middleware, http.HandlerFunc(hf))
}
