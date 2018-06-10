// Copyright (C) 2014 The Syncthing Authors.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

// +build !windows,!darwin

package osutil

import "golang.org/x/text/unicode/norm"

func NormalizedFilename(s string) string {
	return norm.NFC.String(s)
}

func NativeFilename(s string) string {
	return s
}
