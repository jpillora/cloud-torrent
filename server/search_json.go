package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/jpillora/scraper/scraper"
)

// JSON pointers (RFC 6901) avoid executable expressions and wildcard expansion.
// Depth, fields, decoded bytes and result counts are bounded independently.
func validateJSONPointer(pointer string) error {
	if pointer == "" {
		return nil
	}
	if len(pointer) > 1024 || !strings.HasPrefix(pointer, "/") || strings.Count(pointer, "/") > 16 {
		return errors.New("invalid JSON pointer")
	}
	for i := 0; i < len(pointer); i++ {
		if pointer[i] == '~' {
			i++
			if i >= len(pointer) || (pointer[i] != '0' && pointer[i] != '1') {
				return errors.New("invalid JSON pointer escape")
			}
		}
	}
	return nil
}
func jsonPointer(value any, pointer string) (any, bool) {
	if pointer == "" {
		return value, true
	}
	for _, part := range strings.Split(pointer[1:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch node := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = node[part]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node) || strconv.Itoa(index) != part {
				return nil, false
			}
			value = node[index]
		default:
			return nil, false
		}
	}
	return value, true
}
func extractSearchJSON(body []byte, extraction *jsonExtraction) ([]scraper.Result, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("invalid provider JSON")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("trailing provider JSON")
	}
	pointer := ""
	if extraction.List != nil {
		pointer = *extraction.List
	}
	selected, ok := jsonPointer(value, pointer)
	if !ok {
		return nil, errors.New("missing provider result list")
	}
	nodes := []any{selected}
	if extraction.List != nil {
		var isArray bool
		nodes, isArray = selected.([]any)
		if !isArray {
			return nil, errors.New("provider list must be an array")
		}
	}
	if len(nodes) > maxSearchResults {
		return nil, errors.New("too many provider results")
	}
	rows := make([]scraper.Result, 0, len(nodes))
	for _, node := range nodes {
		if _, ok := node.(map[string]any); !ok {
			return nil, errors.New("provider result must be an object")
		}
		row := scraper.Result{}
		for field, path := range extraction.Fields {
			scalar, ok := jsonPointer(node, path)
			if !ok || scalar == nil {
				continue
			}
			switch v := scalar.(type) {
			case string:
				row[field] = v
			case json.Number:
				row[field] = v.String()
			case bool:
				row[field] = strconv.FormatBool(v)
			default:
				return nil, errors.New("provider field must be scalar")
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
