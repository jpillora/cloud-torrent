package iter

type Callback func(interface{}) bool

type Func func(Callback)

func All(cb Callback, fs ...Func) bool {
	for _, f := range fs {
		all := true
		f(func(v interface{}) bool {
			all = all && cb(v)
			return all
		})
		if !all {
			return false
		}
	}
	return true
}
