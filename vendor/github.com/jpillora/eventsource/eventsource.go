package eventsource

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"
)

// Event describes a single Server-Send Event
type Event struct {
	Type  string
	ID    string
	Retry string
	Data  []byte
}

// WriteEvent writes an Event onto the io.Writer.
// If w is an http.Flusher, Flush is also called.
func WriteEvent(w io.Writer, event Event) error {
	if len(event.ID) > 0 {
		if err := writeField(w, "id", []byte(event.ID)); err != nil {
			return err
		}
	}
	if len(event.Retry) > 0 {
		if err := writeField(w, "retry", []byte(event.Retry)); err != nil {
			return err
		}
	}
	if len(event.Type) > 0 {
		if err := writeField(w, "event", []byte(event.Type)); err != nil {
			return err
		}
	}
	if err := writeField(w, "data", event.Data); err != nil {
		return err
	}
	if _, err := w.Write([]byte{'\n'}); err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

func writeField(w io.Writer, field string, value []byte) error {
	if !utf8.ValidString(field) || !utf8.Valid(value) {
		return errors.New("invalid UTF-8 sequence")
	}
	for _, line := range bytes.Split(value, []byte{'\n'}) {
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if err := writeVal(w, field, line); err != nil {
			return err
		}
	}
	return nil
}

func writeVal(w io.Writer, field string, value []byte) error {
	var err error
	if len(value) == 0 {
		_, err = fmt.Fprintf(w, "%s\n", field)
	} else {
		_, err = fmt.Fprintf(w, "%s: %s\n", field, value)
	}
	return err
}
