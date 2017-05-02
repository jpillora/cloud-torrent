package iter

type map_ struct {
	Iterator
	f func(interface{}) interface{}
}

func (me map_) Value() interface{} {
	return me.f(me.Iterator.Value())
}

func Map(i Iterator, f func(interface{}) interface{}) Iterator {
	return map_{i, f}
}
