package scraper

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var templateRe = regexp.MustCompile(`\{\{\s*(\w+)\s*(:(\w+))?\s*\}\}`)

func template(isurl bool, str string, vars map[string]string) (out string, err error) {
	out = templateRe.ReplaceAllStringFunc(str, func(key string) string {
		m := templateRe.FindStringSubmatch(key)
		k := m[1]
		value, ok := vars[k]
		//missing - apply defaults or error
		if !ok {
			if m[3] != "" {
				value = m[3]
			} else {
				err = errors.New("Missing param: " + k)
			}
		}
		//determine if we need to escape
		if isurl {
			queryi := strings.Index(str, "?")
			keyi := strings.Index(str, key)
			if queryi != -1 && keyi > queryi {
				value = url.QueryEscape(value)
			}
		}
		return value
	})
	return
}

func checkSelector(s string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	doc, _ := goquery.NewDocumentFromReader(bytes.NewBufferString(`<html>
		<body>
			<h3>foo bar</h3>
		</body>
	</html>`))
	doc.Find(s)
	return
}

func jsonerr(err error) []byte {
	return []byte(`{"error":"` + err.Error() + `"}`)
}

func logf(format string, args ...interface{}) {
	log.Printf("[scraper] "+format, args...)
}
