package futures

import (
	"sync"
)

func AsCompleted(fs ...*F) <-chan *F {
	ret := make(chan *F, len(fs))
	var wg sync.WaitGroup
	for _, f := range fs {
		wg.Add(1)
		go func(f *F) {
			defer wg.Done()
			<-f.Done()
			ret <- f
		}(f)
	}
	go func() {
		wg.Wait()
		close(ret)
	}()
	return ret
}
