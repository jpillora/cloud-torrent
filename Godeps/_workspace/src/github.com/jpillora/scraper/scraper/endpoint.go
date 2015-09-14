package scraper

import "github.com/PuerkitoBio/goquery"

type Endpoint struct {
	Name    string                `json:"name,omitempty"`
	Method  string                `json:"method,omitempty"`
	URL     string                `json:"url"`
	Body    string                `json:"body,omitempty"`
	Headers map[string]string     `json:"headers,omitempty"`
	List    string                `json:"list,omitempty"`
	Result  map[string]Extractors `json:"result"`
}

//extract 1 result using this endpoints extractor map
func (e *Endpoint) extract(sel *goquery.Selection) result {
	r := result{}
	for field, ext := range e.Result {
		if v := ext.execute(sel); v != "" {
			r[field] = v
		} /* else {
			log.Printf("missing %s", field)
		}*/
	}
	return r
}
