package bencode

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"runtime"
	"strconv"
	"strings"
)

type decoder struct {
	*bufio.Reader
	offset int64
	buf    bytes.Buffer
	key    string
}

func (d *decoder) decode(v interface{}) (err error) {
	defer func() {
		if e := recover(); e != nil {
			if _, ok := e.(runtime.Error); ok {
				panic(e)
			}
			err = e.(error)
		}
	}()

	pv := reflect.ValueOf(v)
	if pv.Kind() != reflect.Ptr || pv.IsNil() {
		return &UnmarshalInvalidArgError{reflect.TypeOf(v)}
	}

	if !d.parse_value(pv.Elem()) {
		d.throwSyntaxError(d.offset-1, errors.New("unexpected 'e'"))
	}
	return nil
}

func check_for_unexpected_eof(err error, offset int64) {
	if err == io.EOF {
		panic(&SyntaxError{
			Offset: offset,
			What:   io.ErrUnexpectedEOF,
		})
	}
}

func (d *decoder) read_byte() byte {
	b, err := d.ReadByte()
	if err != nil {
		check_for_unexpected_eof(err, d.offset)
		panic(err)
	}

	d.offset++
	return b
}

// reads data writing it to 'd.buf' until 'sep' byte is encountered, 'sep' byte
// is consumed, but not included into the 'd.buf'
func (d *decoder) read_until(sep byte) {
	for {
		b := d.read_byte()
		if b == sep {
			return
		}
		d.buf.WriteByte(b)
	}
}

func check_for_int_parse_error(err error, offset int64) {
	if err != nil {
		panic(&SyntaxError{
			Offset: offset,
			What:   err,
		})
	}
}

func (d *decoder) throwSyntaxError(offset int64, err error) {
	panic(&SyntaxError{
		Offset: offset,
		What:   err,
	})
}

// called when 'i' was consumed
func (d *decoder) parse_int(v reflect.Value) {
	start := d.offset - 1
	d.read_until('e')
	if d.buf.Len() == 0 {
		panic(&SyntaxError{
			Offset: start,
			What:   errors.New("empty integer value"),
		})
	}

	s := d.buf.String()

	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		check_for_int_parse_error(err, start)

		if v.OverflowInt(n) {
			panic(&UnmarshalTypeError{
				Value: "integer " + s,
				Type:  v.Type(),
			})
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, 64)
		check_for_int_parse_error(err, start)

		if v.OverflowUint(n) {
			panic(&UnmarshalTypeError{
				Value: "integer " + s,
				Type:  v.Type(),
			})
		}
		v.SetUint(n)
	case reflect.Bool:
		v.SetBool(s != "0")
	default:
		panic(&UnmarshalTypeError{
			Value: "integer " + s,
			Type:  v.Type(),
		})
	}
	d.buf.Reset()
}

func (d *decoder) parse_string(v reflect.Value) {
	start := d.offset - 1

	// read the string length first
	d.read_until(':')
	length, err := strconv.ParseInt(d.buf.String(), 10, 64)
	check_for_int_parse_error(err, start)

	d.buf.Reset()
	n, err := io.CopyN(&d.buf, d, length)
	d.offset += n
	if err != nil {
		check_for_unexpected_eof(err, d.offset)
		panic(&SyntaxError{
			Offset: d.offset,
			What:   errors.New("unexpected I/O error: " + err.Error()),
		})
	}

	switch v.Kind() {
	case reflect.String:
		v.SetString(d.buf.String())
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.Uint8 {
			panic(&UnmarshalTypeError{
				Value: "string",
				Type:  v.Type(),
			})
		}
		sl := make([]byte, len(d.buf.Bytes()))
		copy(sl, d.buf.Bytes())
		v.Set(reflect.ValueOf(sl))
	default:
		panic(&UnmarshalTypeError{
			Value: "string",
			Type:  v.Type(),
		})
	}

	d.buf.Reset()
}

func (d *decoder) parse_dict(v reflect.Value) {
	switch v.Kind() {
	case reflect.Map:
		t := v.Type()
		if t.Key().Kind() != reflect.String {
			panic(&UnmarshalTypeError{
				Value: "object",
				Type:  t,
			})
		}
		if v.IsNil() {
			v.Set(reflect.MakeMap(t))
		}
	case reflect.Struct:
	default:
		panic(&UnmarshalTypeError{
			Value: "object",
			Type:  v.Type(),
		})
	}

	var map_elem reflect.Value

	// so, at this point 'd' byte was consumed, let's just read key/value
	// pairs one by one
	for {
		var valuev reflect.Value
		keyv := reflect.ValueOf(&d.key).Elem()
		if !d.parse_value(keyv) {
			return
		}

		// get valuev as a map value or as a struct field
		switch v.Kind() {
		case reflect.Map:
			elem_type := v.Type().Elem()
			if !map_elem.IsValid() {
				map_elem = reflect.New(elem_type).Elem()
			} else {
				map_elem.Set(reflect.Zero(elem_type))
			}
			valuev = map_elem
		case reflect.Struct:
			var f reflect.StructField
			var ok bool

			t := v.Type()
			for i, n := 0, t.NumField(); i < n; i++ {
				f = t.Field(i)
				tag := f.Tag.Get("bencode")
				if tag == "-" {
					continue
				}
				if f.Anonymous {
					continue
				}

				tag_name, _ := parse_tag(tag)
				if tag_name == d.key {
					ok = true
					break
				}

				if f.Name == d.key {
					ok = true
					break
				}

				if strings.EqualFold(f.Name, d.key) {
					ok = true
					break
				}
			}

			if ok {
				if f.PkgPath != "" {
					panic(&UnmarshalFieldError{
						Key:   d.key,
						Type:  v.Type(),
						Field: f,
					})
				} else {
					valuev = v.FieldByIndex(f.Index)
				}
			} else {
				_, ok := d.parse_value_interface()
				if !ok {
					return
				}
				continue
			}
		}

		// now we need to actually parse it
		if !d.parse_value(valuev) {
			return
		}

		if v.Kind() == reflect.Map {
			v.SetMapIndex(keyv, valuev)
		}
	}
}

func (d *decoder) parse_list(v reflect.Value) {
	switch v.Kind() {
	case reflect.Array, reflect.Slice:
	default:
		panic(&UnmarshalTypeError{
			Value: "array",
			Type:  v.Type(),
		})
	}

	i := 0
	for {
		if v.Kind() == reflect.Slice && i >= v.Len() {
			v.Set(reflect.Append(v, reflect.Zero(v.Type().Elem())))
		}

		ok := false
		if i < v.Len() {
			ok = d.parse_value(v.Index(i))
		} else {
			_, ok = d.parse_value_interface()
		}

		if !ok {
			break
		}

		i++
	}

	if i < v.Len() {
		if v.Kind() == reflect.Array {
			z := reflect.Zero(v.Type().Elem())
			for n := v.Len(); i < n; i++ {
				v.Index(i).Set(z)
			}
		} else {
			v.SetLen(i)
		}
	}

	if i == 0 && v.Kind() == reflect.Slice {
		v.Set(reflect.MakeSlice(v.Type(), 0, 0))
	}
}

func (d *decoder) read_one_value() bool {
	b, err := d.ReadByte()
	if err != nil {
		panic(err)
	}
	if b == 'e' {
		d.UnreadByte()
		return false
	} else {
		d.offset++
		d.buf.WriteByte(b)
	}

	switch b {
	case 'd', 'l':
		// read until there is nothing to read
		for d.read_one_value() {
		}
		// consume 'e' as well
		b = d.read_byte()
		d.buf.WriteByte(b)
	case 'i':
		d.read_until('e')
		d.buf.WriteString("e")
	default:
		if b >= '0' && b <= '9' {
			start := d.buf.Len() - 1
			d.read_until(':')
			length, err := strconv.ParseInt(d.buf.String()[start:], 10, 64)
			check_for_int_parse_error(err, d.offset-1)

			d.buf.WriteString(":")
			n, err := io.CopyN(&d.buf, d, length)
			d.offset += n
			if err != nil {
				check_for_unexpected_eof(err, d.offset)
				panic(&SyntaxError{
					Offset: d.offset,
					What:   errors.New("unexpected I/O error: " + err.Error()),
				})
			}
			break
		}

		d.raiseUnknownValueType(b, d.offset-1)
	}

	return true

}

func (d *decoder) parse_unmarshaler(v reflect.Value) bool {
	m, ok := v.Interface().(Unmarshaler)
	if !ok {
		// T doesn't work, try *T
		if v.Kind() != reflect.Ptr && v.CanAddr() {
			m, ok = v.Addr().Interface().(Unmarshaler)
			if ok {
				v = v.Addr()
			}
		}
	}
	if ok && (v.Kind() != reflect.Ptr || !v.IsNil()) {
		if d.read_one_value() {
			err := m.UnmarshalBencode(d.buf.Bytes())
			d.buf.Reset()
			if err != nil {
				panic(&UnmarshalerError{v.Type(), err})
			}
			return true
		}
		d.buf.Reset()
	}

	return false
}

// Returns true if there was a value and it's now stored in 'v', otherwise
// there was an end symbol ("e") and no value was stored.
func (d *decoder) parse_value(v reflect.Value) bool {
	// we support one level of indirection at the moment
	if v.Kind() == reflect.Ptr {
		// if the pointer is nil, allocate a new element of the type it
		// points to
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}

	if d.parse_unmarshaler(v) {
		return true
	}

	// common case: interface{}
	if v.Kind() == reflect.Interface && v.NumMethod() == 0 {
		iface, _ := d.parse_value_interface()
		v.Set(reflect.ValueOf(iface))
		return true
	}

	b, err := d.ReadByte()
	if err != nil {
		panic(err)
	}
	d.offset++

	switch b {
	case 'e':
		return false
	case 'd':
		d.parse_dict(v)
	case 'l':
		d.parse_list(v)
	case 'i':
		d.parse_int(v)
	default:
		if b >= '0' && b <= '9' {
			// string
			// append first digit of the length to the buffer
			d.buf.WriteByte(b)
			d.parse_string(v)
			break
		}

		d.raiseUnknownValueType(b, d.offset-1)
	}

	return true
}

// An unknown bencode type character was encountered.
func (d *decoder) raiseUnknownValueType(b byte, offset int64) {
	panic(&SyntaxError{
		Offset: offset,
		What:   fmt.Errorf("unknown value type %+q", b),
	})
}

func (d *decoder) parse_value_interface() (interface{}, bool) {
	b, err := d.ReadByte()
	if err != nil {
		panic(err)
	}
	d.offset++

	switch b {
	case 'e':
		return nil, false
	case 'd':
		return d.parse_dict_interface(), true
	case 'l':
		return d.parse_list_interface(), true
	case 'i':
		return d.parse_int_interface(), true
	default:
		if b >= '0' && b <= '9' {
			// string
			// append first digit of the length to the buffer
			d.buf.WriteByte(b)
			return d.parse_string_interface(), true
		}

		d.raiseUnknownValueType(b, d.offset-1)
		panic("unreachable")
	}
}

func (d *decoder) parse_int_interface() (ret interface{}) {
	start := d.offset - 1
	d.read_until('e')
	if d.buf.Len() == 0 {
		panic(&SyntaxError{
			Offset: start,
			What:   errors.New("empty integer value"),
		})
	}

	n, err := strconv.ParseInt(d.buf.String(), 10, 64)
	if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
		i := new(big.Int)
		_, ok := i.SetString(d.buf.String(), 10)
		if !ok {
			panic(&SyntaxError{
				Offset: start,
				What:   errors.New("failed to parse integer"),
			})
		}
		ret = i
	} else {
		check_for_int_parse_error(err, start)
		ret = n
	}

	d.buf.Reset()
	return
}

func (d *decoder) parse_string_interface() interface{} {
	start := d.offset - 1

	// read the string length first
	d.read_until(':')
	length, err := strconv.ParseInt(d.buf.String(), 10, 64)
	check_for_int_parse_error(err, start)

	d.buf.Reset()
	n, err := io.CopyN(&d.buf, d, length)
	d.offset += n
	if err != nil {
		check_for_unexpected_eof(err, d.offset)
		panic(&SyntaxError{
			Offset: d.offset,
			What:   errors.New("unexpected I/O error: " + err.Error()),
		})
	}

	s := d.buf.String()
	d.buf.Reset()
	return s
}

func (d *decoder) parse_dict_interface() interface{} {
	dict := make(map[string]interface{})
	for {
		keyi, ok := d.parse_value_interface()
		if !ok {
			break
		}

		key, ok := keyi.(string)
		if !ok {
			panic(&SyntaxError{
				Offset: d.offset,
				What:   errors.New("non-string key in a dict"),
			})
		}

		valuei, ok := d.parse_value_interface()
		if !ok {
			break
		}

		dict[key] = valuei
	}
	return dict
}

func (d *decoder) parse_list_interface() interface{} {
	var list []interface{}
	for {
		valuei, ok := d.parse_value_interface()
		if !ok {
			break
		}

		list = append(list, valuei)
	}
	if list == nil {
		list = make([]interface{}, 0, 0)
	}
	return list
}
