package opts

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func camel2const(s string) string {
	b := bytes.Buffer{}
	var c rune
	start := 0
	end := 0
	for end, c = range s {
		if c >= 'A' && c <= 'Z' {
			//uppercase all prior letters and add an underscore
			if start < end {
				b.WriteString(strings.ToTitle(s[start:end] + "_"))
				start = end
			}
		}
	}
	//write remaining string
	b.WriteString(strings.ToTitle(s[start : end+1]))
	return b.String()
}

func nletters(r rune, n int) string {
	str := make([]rune, n)
	for i, _ := range str {
		str[i] = r
	}
	return string(str)
}

func str2str(src string, dst *string) {
	if src != "" {
		*dst = src
	}
}

func str2bool(src string, dst *bool) {
	if src != "" {
		*dst = strings.ToLower(src) == "true" || src == "1"
	}
}

func str2int(src string, dst *int) {
	if src != "" {
		n, err := strconv.Atoi(src)
		if err == nil {
			*dst = n
		}
	}
}

func constrain(str string, width int) string {
	words := anyspace.Split(str, -1)
	n := 0
	for i, w := range words {
		d := width - n
		wn := len(w) + 1 //+space
		n += wn
		if n > width && n-width > d {
			n = wn
			w = "\n" + w
		}
		words[i] = w
	}
	return strings.Join(words, " ")
}

//borrowed from https://github.com/huandu/xstrings/blob/master/convert.go#L77
func camel2dash(str string) string {
	if len(str) == 0 {
		return ""
	}

	buf := &bytes.Buffer{}
	var prev, r0, r1 rune
	var size int

	r0 = '-'

	for len(str) > 0 {
		prev = r0
		r0, size = utf8.DecodeRuneInString(str)
		str = str[size:]

		switch {
		case r0 == utf8.RuneError:
			buf.WriteByte(byte(str[0]))

		case unicode.IsUpper(r0):
			if prev != '-' {
				buf.WriteRune('-')
			}

			buf.WriteRune(unicode.ToLower(r0))

			if len(str) == 0 {
				break
			}

			r0, size = utf8.DecodeRuneInString(str)
			str = str[size:]

			if !unicode.IsUpper(r0) {
				buf.WriteRune(r0)
				break
			}

			// find next non-upper-case character and insert `_` properly.
			// it's designed to convert `HTTPServer` to `http_server`.
			// if there are more than 2 adjacent upper case characters in a word,
			// treat them as an abbreviation plus a normal word.
			for len(str) > 0 {
				r1 = r0
				r0, size = utf8.DecodeRuneInString(str)
				str = str[size:]

				if r0 == utf8.RuneError {
					buf.WriteRune(unicode.ToLower(r1))
					buf.WriteByte(byte(str[0]))
					break
				}

				if !unicode.IsUpper(r0) {
					if r0 == '-' || r0 == ' ' || r0 == '_' {
						r0 = '-'
						buf.WriteRune(unicode.ToLower(r1))
					} else {
						buf.WriteRune('-')
						buf.WriteRune(unicode.ToLower(r1))
						buf.WriteRune(r0)
					}

					break
				}

				buf.WriteRune(unicode.ToLower(r1))
			}

			if len(str) == 0 || r0 == '-' {
				buf.WriteRune(unicode.ToLower(r0))
				break
			}

		default:
			if r0 == ' ' || r0 == '_' {
				r0 = '-'
			}

			buf.WriteRune(r0)
		}
	}

	return buf.String()
}
