package scraper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type Extractor struct {
	val string
	fn  extractorFn
}

func NewExtractor(value string) (*Extractor, error) {
	e := &Extractor{}
	if err := e.Set(value); err != nil {
		return nil, err
	}
	return e, nil
}

func MustExtractor(value string) *Extractor {
	e := &Extractor{}
	if err := e.Set(value); err != nil {
		panic(err)
	}
	return e
}

//sets the current string Value as the Extractor function
func (e *Extractor) Set(value string) (err error) {
	for _, g := range generators {
		if g.match(value) {
			e.val = value
			e.fn, err = g.generate(value)
			return
		}
	}
	e.val = value
	e.fn, err = defaultGenerator(value)
	return
}

type extractorFn func(string, *goquery.Selection) (string, *goquery.Selection)

type extractorGenerator func(string) (extractorFn, error)

type Extractors []*Extractor

//execute all Extractors on the query
func (ex Extractors) execute(s *goquery.Selection) string {
	v := ""
	for _, e := range ex {
		v, s = e.fn(v, s)
	}
	return v
}

func (ex *Extractors) UnmarshalJSON(data []byte) error {
	//force array
	if bytes.IndexRune(data, '[') != 0 {
		data = append([]byte{'['}, append(data, ']')...)
	}
	//parse strings
	strs := []string{}
	if err := json.Unmarshal(data, &strs); err != nil {
		return err
	}
	//reset Extractors
	*ex = make(Extractors, len(strs))
	//convert all strings
	for i, s := range strs {
		e := &Extractor{}
		if err := e.Set(s); err != nil {
			return err
		}
		(*ex)[i] = e
	}
	return nil
}

func (ex Extractors) MarshalJSON() ([]byte, error) {
	strs := make([]string, len(ex))
	for i, e := range ex {
		strs[i] = e.val
	}
	return json.Marshal(strs)
}

//selector Extractor
var defaultGenerator = func(selstr string) (extractorFn, error) {
	if err := checkSelector(selstr); err != nil {
		return nil, fmt.Errorf("Invalid selector: %s", err)
	}
	return func(value string, sel *goquery.Selection) (string, *goquery.Selection) {
		s := sel.Find(selstr)
		if value == "" {
			if l := s.Length(); l == 1 {
				value = s.Text()
			} else if l > 1 {
				strs := make([]string, l)
				s.Each(func(i int, s *goquery.Selection) {
					strs[i] = s.Text()
				})
				value = strings.Join(strs, ",")
			}
		}
		return value, s
	}, nil
}

//custom Extractor functions
var generators = []struct {
	match    func(extractor string) bool
	generate extractorGenerator
}{
	//attr generator
	{
		match: func(extractor string) bool {
			return strings.HasPrefix(extractor, "@")
		},
		generate: func(extractor string) (extractorFn, error) {
			attr := strings.TrimPrefix(extractor, "@")
			//make attribute Extractor
			return func(value string, sel *goquery.Selection) (string, *goquery.Selection) {
				value, _ = sel.Attr(attr)
				// h, _ := sel.Html()
				// logf("%s => %s\n%s\n\n", attr, value, h)
				return value, sel
			}, nil
		},
	},
	//regex match generator
	{
		match: func(extractor string) bool {
			return strings.HasPrefix(extractor, "/") && strings.HasSuffix(extractor, "/")
		},
		generate: func(extractor string) (extractorFn, error) {
			reStr := strings.TrimSuffix(strings.TrimPrefix(extractor, "/"), "/")
			re, err := regexp.Compile(reStr)
			if err != nil {
				return nil, fmt.Errorf("Invalid regex '%s': %s", reStr, err)
			}
			return func(value string, sel *goquery.Selection) (string, *goquery.Selection) {
				ctx := value
				if ctx == "" {
					ctx, _ = sel.Html() //force text
				}
				m := re.FindStringSubmatch(ctx)
				if len(m) == 0 {
					value = ""
				} else if len(m) >= 2 && m[1] != "" {
					value = m[1]
				} else {
					value = m[0]
				}
				return value, sel
			}, nil
		},
	},
	//regex (sed syntax) replace generator
	//TODO support more options and support $N replacements
	{
		match: func(extractor string) bool {
			if !strings.HasPrefix(extractor, "s") || len(extractor) < 5 {
				return false
			}
			parts := strings.Split(extractor, string(extractor[1]))
			if len(parts) != 4 {
				return false
			}
			match := parts[1]
			opts := parts[3]
			return match != "" && (opts == "" || opts == "g")
		},
		generate: func(extractor string) (extractorFn, error) {
			parts := strings.Split(extractor, string(extractor[1]))
			match := parts[1]
			repl := parts[2]
			opts := parts[3]
			re, err := regexp.Compile(match)
			if err != nil {
				return nil, fmt.Errorf("Invalid regex '%s' (%s)", match, err)
			}
			all := opts == "g"
			return func(value string, sel *goquery.Selection) (string, *goquery.Selection) {
				ctx := value
				if ctx == "" {
					ctx, _ = sel.Html()
				}
				i := 0
				value = re.ReplaceAllStringFunc(ctx, func(in string) string {
					first := i == 0
					i++
					if !all && !first {
						return in
					}
					return repl
				})
				return value, sel
			}, nil
		},
	},
	//first generator
	{
		match: func(extractor string) bool {
			return extractor == "first()"
		},
		generate: func(_ string) (extractorFn, error) {
			return func(value string, sel *goquery.Selection) (string, *goquery.Selection) {
				return value, sel.First()
			}, nil
		},
	},
	//query param generator
	{
		match: func(extractor string) bool {
			return strings.HasPrefix(extractor, "query-param(") && strings.HasSuffix(extractor, ")")
		},
		generate: func(extractor string) (extractorFn, error) {
			param := strings.TrimSuffix(strings.TrimPrefix(extractor, "query-param("), ")")
			return func(value string, sel *goquery.Selection) (string, *goquery.Selection) {
				ctx := value
				if ctx == "" {
					ctx, _ = sel.Html() //force text
				}
				u, err := url.Parse(ctx)
				if err != nil {
					return "", sel
				}
				return u.Query().Get(param), sel
			}, nil
		},
	},
}
