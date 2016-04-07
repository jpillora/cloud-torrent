package mmap_span

import (
	"io"
	"log"

	"github.com/edsrzf/mmap-go"
)

type segment struct {
	*mmap.MMap
}

func (me segment) Size() int64 {
	return int64(len(*me.MMap))
}

type MMapSpan struct {
	span
}

func (me *MMapSpan) Append(mmap mmap.MMap) {
	me.span = append(me.span, segment{&mmap})
}

func (me MMapSpan) Close() error {
	for _, mMap := range me.span {
		err := mMap.(segment).Unmap()
		if err != nil {
			log.Print(err)
		}
	}
	return nil
}

func (me MMapSpan) Size() (ret int64) {
	for _, seg := range me.span {
		ret += seg.Size()
	}
	return
}

func (me MMapSpan) ReadAt(p []byte, off int64) (n int, err error) {
	me.ApplyTo(off, func(intervalOffset int64, interval sizer) (stop bool) {
		_n := copy(p, (*interval.(segment).MMap)[intervalOffset:])
		p = p[_n:]
		n += _n
		return len(p) == 0
	})
	if len(p) != 0 {
		err = io.EOF
	}
	return
}

func (me MMapSpan) WriteAt(p []byte, off int64) (n int, err error) {
	me.ApplyTo(off, func(iOff int64, i sizer) (stop bool) {
		mMap := i.(segment)
		_n := copy((*mMap.MMap)[iOff:], p)
		// err = mMap.Sync(gommap.MS_ASYNC)
		// if err != nil {
		// 	return true
		// }
		p = p[_n:]
		n += _n
		return len(p) == 0
	})
	if err != nil && len(p) != 0 {
		err = io.ErrShortWrite
	}
	return
}
