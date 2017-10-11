package perf

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"text/tabwriter"
)

var (
	mu     sync.RWMutex
	events = map[string]*event{}
)

func init() {
	http.HandleFunc("/debug/perf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		WriteEventsTable(w)
	})
}

func WriteEventsTable(w io.Writer) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprint(tw, "description\tcount\tmin\tmean\tmax\n")
	type t struct {
		d string
		e *event
	}
	mu.RLock()
	es := make([]t, 0, len(events))
	for d, e := range events {
		e.mu.RLock()
		es = append(es, t{d, e})
		defer e.mu.RUnlock()
	}
	mu.RUnlock()
	sort.Slice(es, func(i, j int) bool {
		return es[i].e.mean() < es[j].e.mean()
	})
	for _, el := range es {
		e := el.e
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n", el.d, e.count, e.min, e.mean(), e.max)
	}
	tw.Flush()
}
