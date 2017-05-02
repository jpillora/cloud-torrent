package dht

import "encoding/hex"

type nodeID struct {
	_int int160
	set  bool
}

func (me *nodeID) SetFromBytes(b []byte) {
	me._int.SetBytes(b)
	me.set = true
}

func (me nodeID) String() string {
	b := me._int.bits[:]
	if len(b) != 20 {
		panic(len(b))
	}
	return hex.EncodeToString(b)
}

func (nid *nodeID) IsSet() bool {
	return nid.set
}

func (me *nodeID) Int160() *int160 {
	if !me.set {
		panic("not set")
	}
	return &me._int
}

func nodeIDFromString(s string) (ret nodeID) {
	ret.SetFromBytes([]byte(s))
	return
}

func (nid0 *nodeID) Distance(nid1 *nodeID) (ret int160) {
	if nid0.IsSet() != nid1.IsSet() {
		ret.SetMax()
		return
	}
	ret.Xor(&nid0._int, &nid1._int)
	return
}

func (nid nodeID) ByteString() string {
	return string(nid.Int160().Bytes())
}
