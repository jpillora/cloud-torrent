package dht

import "math"

type int160 struct {
	bits [20]uint8
}

func (me *int160) SetBytes(b []byte) {
	n := copy(me.bits[:], b)
	if n != 20 {
		panic(n)
	}
}

func (me int160) Bytes() []byte {
	return me.bits[:]
}

func (l int160) Cmp(r int160) int {
	for i := range l.bits {
		if l.bits[i] < r.bits[i] {
			return -1
		} else if l.bits[i] > r.bits[i] {
			return 1
		}
	}
	return 0
}

func (me *int160) SetMax() {
	for i := range me.bits {
		me.bits[i] = math.MaxUint8
	}
}

func (me *int160) Xor(a, b *int160) {
	for i := range me.bits {
		me.bits[i] = a.bits[i] ^ b.bits[i]
	}
}
