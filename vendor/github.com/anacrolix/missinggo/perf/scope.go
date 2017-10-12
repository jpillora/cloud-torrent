package perf

import (
	"runtime"
)

func ScopeTimer(opts ...timerOpt) func() {
	t := NewTimer(append([]timerOpt{Name(getCallerName())}, opts...)...)
	return func() { t.Mark("returned") }
}

func getCallerName() string {
	var pc [1]uintptr
	runtime.Callers(3, pc[:])
	fs := runtime.CallersFrames(pc[:])
	f, _ := fs.Next()
	return f.Func.Name()
}
