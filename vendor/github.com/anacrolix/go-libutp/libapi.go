package utp

/*
#include "utp.h"
*/
import "C"
import "sync"

var (
	mu                 sync.Mutex
	libContextToSocket = map[*C.utp_context]*Socket{}
)

func getSocketForLibContext(uc *C.utp_context) *Socket {
	return libContextToSocket[uc]
}
