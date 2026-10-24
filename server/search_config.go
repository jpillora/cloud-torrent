package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/andybalholm/cascadia"
	"github.com/jpillora/scraper/scraper"
)

const maxSearchAttempts = 3

// providerCatalog keeps extraction and mirror configuration private. Only the
// legacy presentation descriptors are synchronized with browsers.
type providerCatalog struct {
	scraper.Handler
	options map[string]providerOptions
}
type providerOptions struct {
	Type    string          `json:"type"`
	Mirrors []string        `json:"mirrors"`
	JSON    *jsonExtraction `json:"json"`
}
type jsonExtraction struct {
	List   *string           `json:"list"`
	Fields map[string]string `json:"fields"`
}

var providerID = regexp.MustCompile(`^[a-zA-Z0-9_-]+(?:/item)?$`)
var searchFields = map[string]bool{"name": true, "url": true, "path": true, "torrent": true, "magnet": true, "infohash": true, "size": true, "seeds": true, "peers": true, "tracker": true}

func newSearchHandler(config []byte) (*providerCatalog, error) {
	h := &providerCatalog{Handler: scraper.Handler{Log: false, Debug: false}}
	if err := h.LoadConfig(config); err != nil {
		return nil, err
	}
	return h, nil
}
func (h *providerCatalog) LoadConfig(config []byte) error {
	if len(config) > 1<<20 {
		return errors.New("provider configuration too large")
	}
	// Build and validate a complete replacement before publishing any endpoint.
	var raw map[string]map[string]json.RawMessage
	if err := json.Unmarshal(config, &raw); err != nil {
		return err
	}
	if raw == nil {
		return errors.New("provider catalog must be an object")
	}
	allowed := map[string]bool{"name": true, "method": true, "url": true, "body": true, "headers": true, "list": true, "result": true, "debug": true, "type": true, "mirrors": true, "json": true}
	for _, definition := range raw {
		for field := range definition {
			if !allowed[strings.ToLower(field)] {
				return errors.New("unknown provider configuration field")
			}
		}
	}
	var options map[string]providerOptions
	if err := json.Unmarshal(config, &options); err != nil {
		return err
	}
	candidate := scraper.Handler{}
	if err := candidate.LoadConfig(config); err != nil {
		return err
	}
	if len(candidate.Config) > 128 {
		return errors.New("too many providers")
	}
	for id, e := range candidate.Config {
		if !providerID.MatchString(id) || e == nil {
			return errors.New("invalid provider identifier")
		}
		opt := options[id]
		if opt.Type != "" && opt.Type != "html" && opt.Type != "json" {
			return errors.New("unsupported provider type")
		}
		if e.Method != "" && e.Method != http.MethodGet || e.Body != "" {
			return errors.New("providers must use read-only GET")
		}
		if len(opt.Mirrors) >= maxSearchAttempts {
			return errors.New("at most two mirrors are allowed")
		}
		templates := append([]string{e.URL}, opt.Mirrors...)
		seen := map[string]bool{}
		for _, template := range templates {
			// Variables are allowed only after the URL authority, never in hosts.
			authorityTemplate := searchVariables.ReplaceAllStringFunc(template, func(token string) string {
				if searchVariables.FindStringSubmatch(token)[1] == "item" {
					return "/item"
				}
				return token
			})
			rendered := searchVariables.ReplaceAllString(authorityTemplate, "value")
			u, err := url.Parse(rendered)
			if err != nil || !safeSearchURL(u) || u.Fragment != "" {
				return errors.New("invalid provider URL")
			}
			authorityEnd := strings.Index(authorityTemplate, "://") + 3
			if end := strings.IndexAny(authorityTemplate[authorityEnd:], "/?#"); end >= 0 {
				authorityEnd += end
			} else {
				authorityEnd = len(authorityTemplate)
			}
			if strings.Contains(authorityTemplate[:authorityEnd], "{{") || seen[template] {
				return errors.New("invalid or duplicate provider template")
			}
			seen[template] = true
		}
		if len(e.Headers) > 32 {
			return errors.New("too many provider headers")
		}
		headers := http.Header{}
		for key, value := range e.Headers {
			if len(key) > 128 || len(value) > 8192 || strings.ContainsAny(key+value, "\r\n") || strings.ContainsAny(key, " :\t") || key == "" {
				return errors.New("invalid provider header")
			}
			switch strings.ToLower(key) {
			case "host", "connection", "transfer-encoding", "content-length", "proxy-authorization":
				return errors.New("unsupported provider header")
			}
			headers.Set(key, value)
		}
		if headers.Get("User-Agent") == "" {
			headers.Set("User-Agent", userAgent)
		}
		e.Headers = map[string]string{}
		for key := range headers {
			e.Headers[key] = headers.Get(key)
		}
		if opt.Type == "json" {
			if opt.JSON == nil || len(opt.JSON.Fields) == 0 || len(opt.JSON.Fields) > 16 || e.List != "" || len(e.Result) > 0 {
				return errors.New("JSON provider requires json extraction only")
			}
			if opt.JSON.List != nil {
				if err := validateJSONPointer(*opt.JSON.List); err != nil {
					return err
				}
			}
			for field, path := range opt.JSON.Fields {
				if !searchFields[field] {
					return errors.New("unknown result field")
				}
				if err := validateJSONPointer(path); err != nil {
					return err
				}
			}
		} else {
			if opt.JSON != nil || len(e.Result) == 0 || len(e.Result) > 16 {
				return errors.New("HTML provider requires result extraction")
			}
			if e.List != "" {
				if _, err := cascadia.Compile(e.List); err != nil {
					return errors.New("invalid HTML list selector")
				}
			}
			for field := range e.Result {
				if !searchFields[field] {
					return errors.New("unknown result field")
				}
			}
		}
	}
	h.Handler = candidate
	h.options = options
	return nil
}
func safeSearchURL(u *url.URL) bool {
	return u != nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}
