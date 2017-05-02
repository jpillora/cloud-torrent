package chans

import (
	"reflect"
)

// Receives from any channel until it's closed.
func Drain(ch interface{}) {
	chValue := reflect.ValueOf(ch)
	for {
		_, ok := chValue.Recv()
		if !ok {
			break
		}
	}
}
