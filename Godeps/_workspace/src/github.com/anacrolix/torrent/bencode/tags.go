package bencode

import (
	"strings"
)

type tag_options string

func parse_tag(tag string) (string, tag_options) {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx], tag_options(tag[idx+1:])
	}
	return tag, tag_options("")
}

func (this tag_options) contains(option_name string) bool {
	if len(this) == 0 {
		return false
	}

	s := string(this)
	for s != "" {
		var next string
		i := strings.Index(s, ",")
		if i != -1 {
			s, next = s[:i], s[i+1:]
		}
		if s == option_name {
			return true
		}
		s = next
	}
	return false
}
