package eventsource

import (
	"bufio"
	"bytes"
	"io"
	"unicode/utf8"
)

// A Decoder reads and decodes EventSource events from an input stream.
type Decoder struct {
	r *bufio.Reader

	checkedBOM bool
}

// NewDecoder returns a new decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

func (d *Decoder) checkBOM() {
	r, _, err := d.r.ReadRune()

	if err != nil {
		// let other other callers handle this
		return
	}

	if r != 0xFEFF { // utf8 byte order mark
		d.r.UnreadRune()
	}

	d.checkedBOM = true
}

// ReadField reads a single line from the stream and parses it as a field. A
// complete event is signalled by an empty key and value. The returned error
// may either be an error from the stream, or an ErrInvalidEncoding if the
// value is not valid UTF-8.
func (d *Decoder) ReadField() (field string, value []byte, err error) {
	if !d.checkedBOM {
		d.checkBOM()
	}

	var buf []byte

	for {
		line, isPrefix, err := d.r.ReadLine()

		if err != nil {
			return "", nil, err
		}

		buf = append(buf, line...)

		if !isPrefix {
			break
		}
	}

	if len(buf) == 0 {
		return "", nil, nil
	}

	parts := bytes.SplitN(buf, []byte{':'}, 2)
	field = string(parts[0])

	if len(parts) == 2 {
		value = parts[1]
	}

	// §7. If value starts with a U+0020 SPACE character, remove it from value.
	if len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}

	if !utf8.ValidString(field) || !utf8.Valid(value) {
		err = ErrInvalidEncoding
	}

	return
}

// Decode reads the next event from its input and stores it in the provided
// Event pointer.
func (d *Decoder) Decode(e *Event) error {
	var wroteData bool

	// set default event type
	e.Type = "message"

	for {
		field, value, err := d.ReadField()

		if err != nil {
			return err
		}

		if len(field) == 0 && len(value) == 0 {
			break
		}

		switch field {
		case "id":
			e.ID = string(value)
			if len(e.ID) == 0 {
				e.ResetID = true
			}
		case "retry":
			e.Retry = string(value)
		case "event":
			e.Type = string(value)
		case "data":
			if wroteData {
				e.Data = append(e.Data, '\n')
			} else {
				wroteData = true
			}
			e.Data = append(e.Data, value...)
		}
	}

	return nil
}
