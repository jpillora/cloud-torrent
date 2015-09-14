package opts

import (
	"bytes"
	"strconv"
	"strings"
)

func camel2dash(s string) string {
	return strings.ToLower(s)
}

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
