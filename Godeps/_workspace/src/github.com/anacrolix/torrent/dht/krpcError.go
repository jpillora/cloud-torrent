package dht

import (
	"fmt"

	"github.com/anacrolix/torrent/bencode"
)

// Represented as a string or list in bencode.
type KRPCError struct {
	Code int
	Msg  string
}

var (
	_ bencode.Unmarshaler = &KRPCError{}
	_ bencode.Marshaler   = &KRPCError{}
	_ error               = KRPCError{}
)

func (me *KRPCError) UnmarshalBencode(_b []byte) (err error) {
	var _v interface{}
	err = bencode.Unmarshal(_b, &_v)
	if err != nil {
		return
	}
	switch v := _v.(type) {
	case []interface{}:
		me.Code = int(v[0].(int64))
		me.Msg = v[1].(string)
	case string:
		me.Msg = v
	default:
		err = fmt.Errorf(`KRPC error bencode value has unexpected type: %T`, _v)
	}
	return
}

func (me KRPCError) MarshalBencode() (ret []byte, err error) {
	return bencode.Marshal([]interface{}{me.Code, me.Msg})
}

func (me KRPCError) Error() string {
	return fmt.Sprintf("KRPC error %d: %s", me.Code, me.Msg)
}
