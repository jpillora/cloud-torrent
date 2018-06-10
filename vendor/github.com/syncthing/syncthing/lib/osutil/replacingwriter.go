// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package osutil

import (
	"bytes"
	"io"
)

type ReplacingWriter struct {
	Writer io.Writer
	From   byte
	To     []byte
}

func (w ReplacingWriter) Write(bs []byte) (int, error) {
	var n, written int
	var err error

	newlineIdx := bytes.IndexByte(bs, w.From)
	for newlineIdx >= 0 {
		n, err = w.Writer.Write(bs[:newlineIdx])
		written += n
		if err != nil {
			break
		}
		if len(w.To) > 0 {
			n, err := w.Writer.Write(w.To)
			if n == len(w.To) {
				written++
			}
			if err != nil {
				break
			}
		}
		bs = bs[newlineIdx+1:]
		newlineIdx = bytes.IndexByte(bs, w.From)
	}

	n, err = w.Writer.Write(bs)
	written += n

	return written, err
}
