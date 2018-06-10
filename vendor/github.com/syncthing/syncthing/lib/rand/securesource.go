// Copyright (C) 2016 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package rand

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"io"
	"sync"
)

// The secureSource is a math/rand.Source that reads bytes from
// crypto/rand.Reader. It means we can use the convenience functions
// provided by math/rand.Rand on top of a secure source of numbers. It is
// concurrency safe for ease of use.
type secureSource struct {
	rd  io.Reader
	mut sync.Mutex
}

func newSecureSource() *secureSource {
	return &secureSource{
		// Using buffering on top of the rand.Reader increases our
		// performance by about 20%, even though it means we must use
		// locking.
		rd: bufio.NewReader(rand.Reader),
	}
}

func (s *secureSource) Seed(int64) {
	panic("SecureSource is not seedable")
}

func (s *secureSource) Int63() int64 {
	var buf [8]byte

	// Read eight bytes of entropy from the buffered, secure random number
	// generator. The buffered reader isn't concurrency safe, so we lock
	// around that.
	s.mut.Lock()
	_, err := io.ReadFull(s.rd, buf[:])
	s.mut.Unlock()
	if err != nil {
		panic("randomness failure: " + err.Error())
	}

	// Grab those bytes as an uint64
	v := binary.BigEndian.Uint64(buf[:])

	// Mask of the high bit and return the resulting int63
	return int64(v & (1<<63 - 1))
}
