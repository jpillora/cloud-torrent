package refclose

import (
	"sync"
	"testing"

	"github.com/bradfitz/iter"
	"github.com/stretchr/testify/assert"
)

type refTest struct {
	pool RefPool
	key  interface{}
	objs map[*object]struct{}
	t    *testing.T
}

func (me refTest) run() {
	me.objs = make(map[*object]struct{})
	var (
		mu     sync.Mutex
		curObj *object
		wg     sync.WaitGroup
	)
	for range iter.N(1000) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ref := me.pool.NewRef(me.key)
			mu.Lock()
			if curObj == nil {
				curObj = new(object)
				me.objs[curObj] = struct{}{}
			}
			// obj := curObj
			mu.Unlock()
			ref.SetCloser(func() {
				mu.Lock()
				if curObj.closed {
					panic("object already closed")
				}
				curObj.closed = true
				curObj = nil
				mu.Unlock()
			})
			ref.Release()
		}()
	}
	wg.Wait()
	me.t.Logf("created %d objects", len(me.objs))
	assert.True(me.t, len(me.objs) >= 1)
	for obj := range me.objs {
		assert.True(me.t, obj.closed)
	}
}

type object struct {
	closed bool
}

func Test(t *testing.T) {
	refTest{
		key: 3,
		t:   t,
	}.run()
}
