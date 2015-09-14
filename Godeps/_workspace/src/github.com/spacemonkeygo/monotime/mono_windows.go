// Copyright (C) 2014 Space Monkey, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package monotime

import (
	"syscall"
	"unsafe"
)

var (
	kernel32 = syscall.MustLoadDLL("kernel32.dll")
	qpf      = kernel32.MustFindProc("QueryPerformanceFrequency")
	qpc      = kernel32.MustFindProc("QueryPerformanceCounter")
	freq     = freqOrDie()
)

func freqOrDie() int64 {
	var poops int64
	r1, _, err := qpf.Call(uintptr(unsafe.Pointer(&poops)))
	if r1 == 0 || err.(syscall.Errno) != 0 {
		panic(err)
	}
	return poops
}

func monotime() (sec int64, nsec int32) {
	var counter int64
	// this may have pessimistic performance (on older hardware) or be wrong on
	// pre-vista.  see http://msdn.microsoft.com/en-us/library/windows/desktop/dn553408(v=vs.85).aspx
	r1, _, err := qpc.Call(uintptr(unsafe.Pointer(&counter)))
	if r1 == 0 || err.(syscall.Errno) != 0 {
		panic(err)
	}
	us := (counter * 1000000) / freq
	sec = us / 1000000
	us -= sec * 1000000
	nsec = int32(us) * 1000
	return sec, nsec
}
