package bencode

import (
	"bufio"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"sync"

	"github.com/anacrolix/missinggo"
)

func is_empty_value(v reflect.Value) bool {
	return missinggo.IsEmptyValue(v)
}

type encoder struct {
	*bufio.Writer
	scratch [64]byte
}

func (e *encoder) encode(v interface{}) (err error) {
	if v == nil {
		return
	}
	defer func() {
		if e := recover(); e != nil {
			if _, ok := e.(runtime.Error); ok {
				panic(e)
			}
			var ok bool
			err, ok = e.(error)
			if !ok {
				panic(e)
			}
		}
	}()
	e.reflect_value(reflect.ValueOf(v))
	return e.Flush()
}

type string_values []reflect.Value

func (sv string_values) Len() int           { return len(sv) }
func (sv string_values) Swap(i, j int)      { sv[i], sv[j] = sv[j], sv[i] }
func (sv string_values) Less(i, j int) bool { return sv.get(i) < sv.get(j) }
func (sv string_values) get(i int) string   { return sv[i].String() }

func (e *encoder) write(s []byte) {
	_, err := e.Write(s)
	if err != nil {
		panic(err)
	}
}

func (e *encoder) write_string(s string) {
	_, err := e.WriteString(s)
	if err != nil {
		panic(err)
	}
}

func (e *encoder) reflect_string(s string) {
	b := strconv.AppendInt(e.scratch[:0], int64(len(s)), 10)
	e.write(b)
	e.write_string(":")
	e.write_string(s)
}

func (e *encoder) reflect_byte_slice(s []byte) {
	b := strconv.AppendInt(e.scratch[:0], int64(len(s)), 10)
	e.write(b)
	e.write_string(":")
	e.write(s)
}

// returns true if the value implements Marshaler interface and marshaling was
// done successfully
func (e *encoder) reflect_marshaler(v reflect.Value) bool {
	m, ok := v.Interface().(Marshaler)
	if !ok {
		// T doesn't work, try *T
		if v.Kind() != reflect.Ptr && v.CanAddr() {
			m, ok = v.Addr().Interface().(Marshaler)
			if ok {
				v = v.Addr()
			}
		}
	}
	if ok && (v.Kind() != reflect.Ptr || !v.IsNil()) {
		data, err := m.MarshalBencode()
		if err != nil {
			panic(&MarshalerError{v.Type(), err})
		}
		e.write(data)
		return true
	}

	return false
}

func (e *encoder) reflect_value(v reflect.Value) {

	if e.reflect_marshaler(v) {
		return
	}

	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			e.write_string("i1e")
		} else {
			e.write_string("i0e")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b := strconv.AppendInt(e.scratch[:0], v.Int(), 10)
		e.write_string("i")
		e.write(b)
		e.write_string("e")
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b := strconv.AppendUint(e.scratch[:0], v.Uint(), 10)
		e.write_string("i")
		e.write(b)
		e.write_string("e")
	case reflect.String:
		e.reflect_string(v.String())
	case reflect.Struct:
		e.write_string("d")
		for _, ef := range encode_fields(v.Type()) {
			field_value := v.Field(ef.i)
			if ef.omit_empty && is_empty_value(field_value) {
				continue
			}
			e.reflect_string(ef.tag)
			e.reflect_value(field_value)
		}
		e.write_string("e")
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			panic(&MarshalTypeError{v.Type()})
		}
		if v.IsNil() {
			e.write_string("de")
			break
		}
		e.write_string("d")
		sv := string_values(v.MapKeys())
		sort.Sort(sv)
		for _, key := range sv {
			e.reflect_string(key.String())
			e.reflect_value(v.MapIndex(key))
		}
		e.write_string("e")
	case reflect.Slice:
		if v.IsNil() {
			e.write_string("le")
			break
		}
		if v.Type().Elem().Kind() == reflect.Uint8 {
			s := v.Bytes()
			e.reflect_byte_slice(s)
			break
		}
		fallthrough
	case reflect.Array:
		e.write_string("l")
		for i, n := 0, v.Len(); i < n; i++ {
			e.reflect_value(v.Index(i))
		}
		e.write_string("e")
	case reflect.Interface:
		e.reflect_value(v.Elem())
	case reflect.Ptr:
		if v.IsNil() {
			v = reflect.Zero(v.Type().Elem())
		} else {
			v = v.Elem()
		}
		e.reflect_value(v)
	default:
		panic(&MarshalTypeError{v.Type()})
	}
}

type encode_field struct {
	i          int
	tag        string
	omit_empty bool
}

type encode_fields_sort_type []encode_field

func (ef encode_fields_sort_type) Len() int           { return len(ef) }
func (ef encode_fields_sort_type) Swap(i, j int)      { ef[i], ef[j] = ef[j], ef[i] }
func (ef encode_fields_sort_type) Less(i, j int) bool { return ef[i].tag < ef[j].tag }

var (
	type_cache_lock     sync.RWMutex
	encode_fields_cache = make(map[reflect.Type][]encode_field)
)

func encode_fields(t reflect.Type) []encode_field {
	type_cache_lock.RLock()
	fs, ok := encode_fields_cache[t]
	type_cache_lock.RUnlock()
	if ok {
		return fs
	}

	type_cache_lock.Lock()
	defer type_cache_lock.Unlock()
	fs, ok = encode_fields_cache[t]
	if ok {
		return fs
	}

	for i, n := 0, t.NumField(); i < n; i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		if f.Anonymous {
			continue
		}
		var ef encode_field
		ef.i = i
		ef.tag = f.Name

		tv := f.Tag.Get("bencode")
		if tv != "" {
			if tv == "-" {
				continue
			}
			name, opts := parse_tag(tv)
			if name != "" {
				ef.tag = name
			}
			ef.omit_empty = opts.contains("omitempty")
		}
		fs = append(fs, ef)
	}
	fss := encode_fields_sort_type(fs)
	sort.Sort(fss)
	encode_fields_cache[t] = fs
	return fs
}
