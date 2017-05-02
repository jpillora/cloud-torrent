package eventsource

import (
	"bytes"
	"reflect"
	"testing"
)

func longLine() string {
	buf := make([]byte, 4096)
	for i := 0; i < len(buf); i++ {
		buf[i] = 'a'
	}

	return string(buf)
}

func TestDecoderReadField(t *testing.T) {
	table := []struct {
		in    string
		field string
		value []byte
		err   error
	}{
		{"\n", "", nil, nil},
		{"id", "id", nil, nil},
		{"id:", "id", nil, nil},
		{"id:1", "id", []byte("1"), nil},
		{"id: 1", "id", []byte("1"), nil},
		{"data: " + longLine(), "data", []byte(longLine()), nil},
		{"\xFF\xFE\xFD", "\xFF\xFE\xFD", nil, ErrInvalidEncoding},
		{"data: \xFF\xFE\xFD", "data", []byte("\xFF\xFE\xFD"), ErrInvalidEncoding},
	}

	for i, tt := range table {
		dec := NewDecoder(bytes.NewBufferString(tt.in))

		field, value, err := dec.ReadField()

		if err != tt.err {
			t.Errorf("%d. expected err=%q, got %q", i, tt.err, err)
			continue
		}

		if exp, got := tt.field, field; exp != got {
			t.Errorf("%d. expected field=%q, got %q", i, exp, got)
		}

		if exp, got := tt.value, value; !bytes.Equal(exp, got) {
			t.Errorf("%d. expected value=%q, got %q", i, exp, got)
		}
	}
}

func TestDecoderDecode(t *testing.T) {
	table := []struct {
		in  string
		out Event
	}{
		{"event: type\ndata\n\n", Event{Type: "type"}},
		{"id: 123\ndata\n\n", Event{Type: "message", ID: "123"}},
		{"retry: 10000\ndata\n\n", Event{Type: "message", Retry: "10000"}},
		{"data: data\n\n", Event{Type: "message", Data: []byte("data")}},
		{"id\ndata\n\n", Event{Type: "message", ResetID: true}},
	}

	for i, tt := range table {
		dec := NewDecoder(bytes.NewBufferString(tt.in))

		var event Event
		if err := dec.Decode(&event); err != nil {
			t.Errorf("%d. %s", i, err)
			continue
		}

		if !reflect.DeepEqual(event, tt.out) {
			t.Errorf("%d. expected %#v, got %#v", i, tt.out, event)
		}
	}
}
