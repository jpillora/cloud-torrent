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
	"runtime"
	"testing"
	"time"
)

func TestMonotonic(t *testing.T) {
	upper := 10
	if runtime.GOOS == "darwin" {
		// a bug only showed up when we did this test for seconds on darwin
		upper = 10000
	}
	for i := 0; i < upper; i++ {
		start := Monotonic()
		time.Sleep(1 * time.Millisecond)
		end := Monotonic()
		if end <= start {
			t.Fatalf("time didn't advance monotonically: %s", end-start)
		}
	}
}

func BenchmarkMonotonic(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Monotonic()
	}
}

func BenchmarkTimeNow(b *testing.B) {
	for i := 0; i < b.N; i++ {
		time.Now()
	}
}

func TestNow(t *testing.T) {
	now := time.Now()
	check := func() {
		now := time.Now()
		mono_now := Now()
		var diff time.Duration
		if now.After(mono_now) {
			diff = now.Sub(mono_now)
		} else {
			diff = mono_now.Sub(now)
		}
		if diff >= time.Millisecond {
			t.Fatalf("computers suck: %s", diff)
		}
	}
	check()
	time.Sleep(5 * time.Millisecond)
	check()
	time.Sleep(20 * time.Millisecond)
	check()
	SetTime(now)
	new_now := time.Now()
	mono_now := Now()
	var diff time.Duration
	if now.After(mono_now) {
		diff = now.Sub(mono_now)
	} else {
		diff = mono_now.Sub(now)
	}
	if diff >= 10*time.Microsecond {
		t.Fatalf("computers suck: %s", diff)
	}
	if new_now.After(mono_now) {
		diff = new_now.Sub(mono_now)
	} else {
		diff = mono_now.Sub(new_now)
	}
	if diff < 25*time.Millisecond {
		t.Fatalf("computers suck: %s", diff)
	}
	diff -= 25 * time.Millisecond
	if diff >= 1*time.Millisecond {
		t.Fatalf("computers suck: %s", diff)
	}
}
