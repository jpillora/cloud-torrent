package missinggo

import "golang.org/x/net/context"

type ContextedReader struct {
	R   ReadContexter
	Ctx context.Context
}

func (me ContextedReader) Read(b []byte) (int, error) {
	return me.R.ReadContext(me.Ctx, b)
}

type ReadContexter interface {
	ReadContext(context.Context, []byte) (int, error)
}
