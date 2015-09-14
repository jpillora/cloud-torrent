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

// Package monotime provides a monotonic timer for Go 1.2
// Go 1.3 will support monotonic time on its own.
package monotime

import (
	"time"
)

var (
	wallclock_offset time.Time
	monotonic_offset time.Duration
)

// Monotonic returns a time duration from some fixed point in the past.
func Monotonic() time.Duration {
	sec, nsec := monotime()
	return time.Duration(sec*1000000000 + int64(nsec))
}

func init() {
	SetTime(time.Now())
}

// SetTime sets the current time, used by Now() for stable offset generation.
// SetTime is not threadsafe, and should be run during process start only.
func SetTime(t time.Time) {
	monotonic_offset = Monotonic()
	wallclock_offset = t
}

// Now returns something close to time.Now(), but is monotonically increasing.
// Use SetTime to change the clock that Now() is based off of.
func Now() time.Time {
	return wallclock_offset.Add(Monotonic() - monotonic_offset)
}
