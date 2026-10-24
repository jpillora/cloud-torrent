package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jpillora/scraper/scraper"
)

// Provider requests use the application's transport instead of the scraper's
// package-global DefaultClient. The dependency remains the config validator.
func (s *Server) executeSearch(w http.ResponseWriter, r *http.Request, e *scraper.Endpoint, options providerOptions) {

	timeout := s.searchTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	r = r.WithContext(ctx)
	s.searchInit.Do(func() { s.searchSlots = make(chan struct{}, maxSearchConcurrency) })
	select {
	case s.searchSlots <- struct{}{}:
		defer func() { <-s.searchSlots }()
	default:
		http.Error(w, "too many provider requests", http.StatusTooManyRequests)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	policyClient, err := s.remoteClient()
	if err != nil {
		http.Error(w, "invalid outbound policy", 500)
		return
	}
	templates := append([]string{e.URL}, options.Mirrors...)
	var rows []scraper.Result
	var failure error
	for _, template := range templates {
		if ctx.Err() != nil {
			break
		}
		raw, err := renderSearchURL(template, r.URL.Query())
		if err != nil {
			http.Error(w, "invalid provider request", 400)
			return
		}
		rows, err = fetchSearchAttempt(ctx, policyClient, raw, e, options)
		if err == nil {
			failure = nil
			break
		}
		failure = err
	}
	if ctx.Err() != nil {
		http.Error(w, "provider request expired", 504)
		return
	}
	if failure != nil {
		http.Error(w, "provider attempts failed or response is invalid", 502)
		return
	}
	var result any = rows
	if e.List == "" && (options.JSON == nil || options.JSON.List == nil) && len(rows) == 1 {
		result = rows[0]
	}
	_ = json.NewEncoder(w).Encode(result)
}

// Each attempt is closed before the next starts, sharing the caller's deadline
// and concurrency slot. An empty successful extraction never triggers fallback.
func fetchSearchAttempt(ctx context.Context, client *http.Client, raw string, e *scraper.Endpoint, options providerOptions) ([]scraper.Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	for key, value := range e.Headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("provider status")
	}
	body, err := readSearchBody(resp)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var rows []scraper.Result
	if options.Type == "json" {
		rows, err = extractSearchJSON(body, options.JSON)
	} else {
		var doc *goquery.Document
		doc, err = goquery.NewDocumentFromReader(bytes.NewReader(body))
		if err == nil {
			rows, err = extractSearchHTML(e, doc)
		}
	}
	if err != nil {
		return nil, err
	}
	base := req.URL
	if resp.Request != nil && resp.Request.URL != nil {
		base = resp.Request.URL
	}
	for _, row := range rows {
		if err := normalizeSearchResult(row, base); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

var searchVariables = regexp.MustCompile(`\{\{\s*(\w+)\s*(?::(\w+))?\s*\}\}`)

func renderSearchURL(template string, values url.Values) (string, error) {
	var missing bool
	queryIndex := strings.IndexByte(template, '?')
	out := searchVariables.ReplaceAllStringFunc(template, func(token string) string {
		match := searchVariables.FindStringSubmatch(token)
		value := values.Get(match[1])
		if value == "" {
			value = match[2]
		}
		if value == "" {
			missing = true
		}
		if match[1] == "item" && (!strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//")) {
			missing = true
		}
		if queryIndex >= 0 && strings.Index(template, token) > queryIndex {
			return url.QueryEscape(value)
		}
		// Item paths may contain slash-separated segments, never authority syntax.
		pieces := strings.Split(value, "/")
		for i := range pieces {
			pieces[i] = url.PathEscape(pieces[i])
		}
		return strings.Join(pieces, "/")
	})
	if missing {
		return "", errors.New("missing query parameter")
	}
	return out, nil
}

func extractSearchHTML(e *scraper.Endpoint, doc *goquery.Document) ([]scraper.Result, error) {
	encoded, err := json.Marshal(e.Result)
	if err != nil {
		return nil, err
	}
	rules := map[string][]string{}
	if err = json.Unmarshal(encoded, &rules); err != nil {
		return nil, err
	}
	nodes := doc.Selection
	if e.List != "" {
		nodes = nodes.Find(e.List)
	}
	rows := []scraper.Result{}
	var failed error
	nodes.EachWithBreak(func(_ int, row *goquery.Selection) bool {
		if len(rows) >= maxSearchResults {
			failed = errors.New("too many provider results")
			return false
		}
		result := scraper.Result{}
		for field, steps := range rules {
			value, err := extractHTMLSteps(row, steps)
			if err != nil {
				failed = err
				return false
			}
			if value != "" {
				result[field] = value
			}
		}
		if e.List == "" || len(result) == len(rules) {
			rows = append(rows, result)
		}
		return true
	})
	return rows, failed
}

func extractHTMLSteps(selection *goquery.Selection, steps []string) (string, error) {
	value := ""
	for _, step := range steps {
		switch {
		case strings.HasPrefix(step, "@"):
			value, _ = selection.Attr(strings.TrimPrefix(step, "@"))
		case step == "first()":
			selection = selection.First()
		case step == "html()":
			value, _ = selection.Html()
		case step == "trim()":
			value = strings.TrimSpace(value)
		case strings.HasPrefix(step, "query-param(") && strings.HasSuffix(step, ")"):
			u, err := url.Parse(value)
			if err != nil {
				return "", err
			}
			value = u.Query().Get(strings.TrimSuffix(strings.TrimPrefix(step, "query-param("), ")"))
		case strings.HasPrefix(step, "/") && strings.HasSuffix(step, "/"):
			pattern, err := regexp.Compile(step[1 : len(step)-1])
			if err != nil {
				return "", err
			}
			input := value
			if input == "" {
				input, _ = selection.Html()
			}
			match := pattern.FindStringSubmatch(input)
			value = ""
			if len(match) > 1 {
				value = match[1]
			} else if len(match) == 1 {
				value = match[0]
			}
		case len(step) > 4 && step[0] == 's' && len(strings.Split(step, string(step[1]))) == 4:
			parts := strings.Split(step, string(step[1]))
			pattern, err := regexp.Compile(parts[1])
			if err != nil {
				return "", err
			}
			input := value
			if input == "" {
				input, _ = selection.Html()
			}
			count := 0
			value = pattern.ReplaceAllStringFunc(input, func(match string) string {
				count++
				if parts[3] != "g" && count > 1 {
					return match
				}
				return parts[2]
			})
		default:
			selection = selection.Find(step)
			if value == "" {
				parts := []string{}
				selection.Each(func(_ int, s *goquery.Selection) { parts = append(parts, s.Text()) })
				value = strings.Join(parts, ",")
			}
		}
	}
	return value, nil
}

const (
	maxSearchBytes       = 2 << 20
	maxSearchResults     = 500
	maxSearchConcurrency = 4
)

// Limit decoded bytes even when provider headers explicitly request gzip and
// therefore disable net/http's automatic decompression.
func readSearchBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	default:
		return nil, errors.New("unsupported provider encoding")
	}
	body, err := io.ReadAll(io.LimitReader(reader, maxSearchBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxSearchBytes {
		return nil, errors.New("provider response too large")
	}
	return body, nil
}
