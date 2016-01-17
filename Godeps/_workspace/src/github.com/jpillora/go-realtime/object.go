package realtime

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/mattbaird/jsonpatch"
)

type key string

//an Object is embedded into a parent marshallable struct.
//an Object has N subscribers.
//when the parent changes, it is marshalled and sent to each subscriber.
type Object struct {
	mut         sync.Mutex //protects all object fields
	added       bool
	key         key
	value       interface{}
	bytes       []byte
	version     int64
	subscribers map[string]*User
	checked     bool
}

func (o *Object) add(k string, val interface{}) (*Object, error) {
	if o.added {
		return nil, fmt.Errorf("already been added to a handler")
	}
	o.added = true
	if b, err := json.Marshal(val); err != nil {
		return nil, fmt.Errorf("JSON marshalling failed: %s", err)
	} else {
		o.bytes = b //initial state
	}
	o.key = key(k)
	o.value = val
	o.version = 1
	o.subscribers = map[string]*User{}
	return o, nil
}

//Send the changes from this object since the last update Update subscribers
func (o *Object) Update() {
	o.mut.Lock()
	o.checked = false
	o.mut.Unlock()
}

type update struct {
	Key     key
	Delta   bool  `json:",omitempty"`
	Version int64 //53 usable bits
	Data    jsonBytes
}

//called by realtime.flusher ONLY!
func (o *Object) computeUpdate() bool {
	//ensure only 1 update computation at a time
	o.mut.Lock()
	defer o.mut.Unlock()
	if o.checked {
		return false
	}
	//mark
	o.checked = true
	newBytes, err := json.Marshal(o.value)
	if err != nil {
		log.Printf("go-realtime: %s: marshal failed: %s", o.key, err)
		return false
	}
	//calculate change set
	ops, _ := jsonpatch.CreatePatch(o.bytes, newBytes)
	if len(o.bytes) > 0 && len(ops) == 0 {
		return false
	}
	delta, _ := json.Marshal(ops)
	prev := o.version
	o.version++
	//send this new change to each subscriber
	for _, u := range o.subscribers {
		update := &update{
			Key:     o.key,
			Version: o.version,
		}
		u.mut.Lock()
		//choose optimal update (send the smallest)
		if u.versions[o.key] == prev && len(o.bytes) > 0 && len(delta) < len(o.bytes) {
			update.Delta = true
			update.Data = delta
		} else {
			update.Delta = false
			update.Data = newBytes
		}
		//insert pending update
		u.pending = append(u.pending, update)
		//user now has this version
		u.versions[o.key] = o.version
		u.mut.Unlock()
	}
	o.bytes = newBytes
	return true
}
