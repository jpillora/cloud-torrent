package missinggo

import "reflect"

func Max(_less interface{}, vals ...interface{}) interface{} {
	ret := reflect.ValueOf(vals[0])
	less := reflect.ValueOf(_less)
	for _, _v := range vals[1:] {
		v := reflect.ValueOf(_v)
		out := less.Call([]reflect.Value{ret, v})
		if out[0].Bool() {
			ret = v
		}
	}
	return ret.Interface()
}
