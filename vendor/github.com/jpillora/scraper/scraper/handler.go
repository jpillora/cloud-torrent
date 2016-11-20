package scraper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

//a single result
type result map[string]string

//the configuration file
type Config map[string]*Endpoint

type Handler struct {
	Config  Config            `opts:"-"`
	Headers map[string]string `opts:"-"`
	Auth    string            `help:"Basic auth credentials <user>:<pass>"`
	Log     bool              `opts:"-"`
	Debug   bool              `help:"Enable debug output"`
}

func (h *Handler) LoadConfigFile(path string) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return h.LoadConfig(b)
}

func (h *Handler) LoadConfig(b []byte) error {
	c := Config{}
	//json unmarshal performs selector validation
	if err := json.Unmarshal(b, &c); err != nil {
		return err
	}
	if h.Log {
		for k, e := range c {
			if strings.HasPrefix(k, "/") {
				delete(c, k)
				k = strings.TrimPrefix(k, "/")
				c[k] = e
			}
			logf("Loaded endpoint: /%s", k)
			e.debug = h.Debug
		}
	}
	if h.Debug {
		logf("Enabled debug mode")
	}
	//replace config
	h.Config = c
	return nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	//basic auth
	if h.Auth != "" {
		u, p, _ := r.BasicAuth()
		if h.Auth != u+":"+p {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Access Denied"))
			return
		}
	}

	//always JSON!
	w.Header().Set("Content-Type", "application/json")

	//admin actions
	if r.URL.Path == "" || r.URL.Path == "/" {
		get := false
		if r.Method == "GET" {
			get = true
		} else if r.Method == "POST" {
			b, err := ioutil.ReadAll(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(jsonerr(err))
				return
			}
			if err := h.LoadConfig(b); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write(jsonerr(err))
				return
			}
			get = true
		}

		if !get {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write(jsonerr(errors.New("Use GET or POST")))
		}
		b, _ := json.MarshalIndent(h.Config, "", "  ")
		w.Write(b)
		return
	}
	//search actions
	id := r.URL.Path[1:] //exclude root slash
	if e, ok := h.Config[id]; ok {
		h.execute(e, w, r)
		return
	}
	w.WriteHeader(404)
	w.Write(jsonerr(fmt.Errorf("Endpoint /%s not found", id)))
}

func (h *Handler) execute(e *Endpoint, w http.ResponseWriter, r *http.Request) {

	values := r.URL.Query()

	url, err := template(true, e.URL, values)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write(jsonerr(err))
		return
	}

	method := e.Method
	if method == "" {
		method = "GET"
	}

	body := io.Reader(nil)
	if e.Body != "" {
		if s, err := template(true, e.Body, values); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(jsonerr(err))
			return
		} else {
			body = strings.NewReader(s)
		}
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(jsonerr(err))
		return
	}

	if h.Headers != nil {
		for k, v := range h.Headers {
			req.Header.Set(k, v)
		}
	}
	if e.Headers != nil {
		for k, v := range e.Headers {
			req.Header.Set(k, v)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(jsonerr(err))
		return
	}

	if h.Log {
		logf("%s %s => %s", method, url, resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(jsonerr(err))
	}
	sel := doc.Selection

	var out interface{}
	//out will be either a list of results, or a single result
	if e.List != "" {
		var results []result
		sels := sel.Find(e.List)
		if h.Debug {
			logf("list: %s => #%d elements", e.List, sels.Length())
		}
		sels.Each(func(i int, sel *goquery.Selection) {
			r := e.extract(sel)
			if len(r) == len(e.Result) {
				results = append(results, r)
			} else if h.Debug {
				logf("excluded #%d: has %d fields, expected %d", i, len(r), len(e.Result))
			}
		})
		out = results
	} else {
		out = e.extract(sel)
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		w.Write([]byte("JSON Error: " + err.Error()))
	}
}
