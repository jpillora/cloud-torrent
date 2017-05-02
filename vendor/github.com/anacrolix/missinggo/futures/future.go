package futures

import "sync"

func Start(fn func() interface{}) *F {
	f := &F{
		done: make(chan struct{}),
	}
	go func() {
		result := fn()
		f.setResult(result)
	}()
	return f
}

type F struct {
	mu     sync.Mutex
	result interface{}
	done   chan struct{}
}

func (f *F) Result() interface{} {
	<-f.done
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.result
}

func (f *F) Done() <-chan struct{} {
	return f.done
}

func (f *F) setResult(result interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.result = result
	close(f.done)
}
