package ctrlflow

type valueWrapper struct {
	value interface{}
}

func Panic(val interface{}) {
	panic(valueWrapper{val})
}

func Recover(handler func(interface{}) bool) {
	r := recover()
	if r == nil {
		return
	}
	if vw, ok := r.(valueWrapper); ok {
		if handler(vw.value) {
			return
		}
	}
	panic(r)
}
