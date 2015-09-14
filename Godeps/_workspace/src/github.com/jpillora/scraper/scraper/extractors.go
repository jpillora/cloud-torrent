package scraper

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	for re, generator := range generators {
		m := re.FindStringSubmatch(value)
		if len(m) > 0 {
			if len(m) > 1 {
				value = m[1]
			}
			e.val = value
			e.fn, err = generator(value)
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

var generatorsPre = map[string]extractorGenerator{
	//attr generator
	`^@(.+)`: func(attr string) (extractorFn, error) {
		//make attribute Extractor
		return func(value string, sel *goquery.Selection) (string, *goquery.Selection) {
			value, _ = sel.Attr(attr)
			// h, _ := sel.Html()
			// log.Printf("attr===\n%s\n\n", h)
			return value, sel
		}, nil
	},
	//regex generator
	`^\/(.+)\/$`: func(reStr string) (extractorFn, error) {
		re, err := regexp.Compile(reStr)
		if err != nil {
			return nil, fmt.Errorf("Invalid regex '%s': %s", reStr, err)
		}
		return func(value string, sel *goquery.Selection) (string, *goquery.Selection) {
			ctx := value
			if ctx == "" {
				ctx, _ = sel.Html()
			}
			m := re.FindStringSubmatch(ctx)
			if len(m) == 0 {
				value = ""
			} else if m[1] != "" {
				value = m[1]
			} else {
				value = m[0]
			}
			return value, sel
		}, nil
	},
	//first() generator
	`^first\(\)$`: func(_ string) (extractorFn, error) {
		return func(value string, sel *goquery.Selection) (string, *goquery.Selection) {
			return value, sel.First()
		}, nil
	},
}

//run-time generators
var generators = map[*regexp.Regexp]extractorGenerator{}

func init() {
	for str, gen := range generatorsPre {
		generators[regexp.MustCompile(str)] = gen
	}
}
