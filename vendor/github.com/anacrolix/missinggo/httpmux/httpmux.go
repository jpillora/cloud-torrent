package httpmux

import (
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/context"
)

var pathParamContextKey = &struct{}{}

type Mux struct {
	handlers []handler
}

func New() *Mux {
	return new(Mux)
}

type handler struct {
	path        *regexp.Regexp
	userHandler http.Handler
}

func (me *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	matches := me.matchingHandlers(r)
	if len(matches) == 0 {
		http.NotFound(w, r)
		return
	}
	m := matches[0]
	r = r.WithContext(context.WithValue(r.Context(), pathParamContextKey, &PathParams{m}))
	m.handler.userHandler.ServeHTTP(w, r)
}

type match struct {
	handler    handler
	submatches []string
}

func (me *Mux) matchingHandlers(r *http.Request) (ret []match) {
	for _, h := range me.handlers {
		subs := h.path.FindStringSubmatch(r.URL.Path)
		if subs == nil {
			continue
		}
		ret = append(ret, match{h, subs})
	}
	return
}

func (me *Mux) distinctHandlerRegexp(r *regexp.Regexp) bool {
	for _, h := range me.handlers {
		if h.path.String() == r.String() {
			return false
		}
	}
	return true
}

func (me *Mux) Handle(path string, h http.Handler) {
	expr := "^" + path
	if !strings.HasSuffix(expr, "$") {
		expr += "$"
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		panic(err)
	}
	if !me.distinctHandlerRegexp(re) {
		panic(fmt.Sprintf("path %q is not distinct", path))
	}
	me.handlers = append(me.handlers, handler{re, h})
}

func (me *Mux) HandleFunc(path string, hf func(http.ResponseWriter, *http.Request)) {
	me.Handle(path, http.HandlerFunc(hf))
}

func Path(parts ...string) string {
	return path.Join(parts...)
}

type PathParams struct {
	match match
}

func (me *PathParams) ByName(name string) string {
	for i, sn := range me.match.handler.path.SubexpNames()[1:] {
		if sn == name {
			return me.match.submatches[i+1]
		}
	}
	return ""
}

func RequestPathParams(r *http.Request) *PathParams {
	ctx := r.Context()
	return ctx.Value(pathParamContextKey).(*PathParams)
}

func PathRegexpParam(name string, re string) string {
	return fmt.Sprintf("(?P<%s>%s)", name, re)
}

func Param(name string) string {
	return fmt.Sprintf("(?P<%s>[^/]+)", name)
}

func RestParam(name string) string {
	return fmt.Sprintf("(?P<%s>.*)$", name)
}

func NonEmptyRestParam(name string) string {
	return fmt.Sprintf("(?P<%s>.+)$", name)
}
