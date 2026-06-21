// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

// zeroBytes is a shared 1024-byte slice of zeros, initialized once at package
// load. Eager initialization keeps reads lock-free and race-free on the hot
// padding path. The slice must not be modified by callers.
var zeroBytes = make([]byte, 1024)

// ZeroBytes returns the shared zero-filled slice. The returned slice must not be
// modified by callers.
func ZeroBytes() []byte {
	return zeroBytes
}

// AppendZeroPadding appends the specified number of zero bytes to buf
func AppendZeroPadding(buf []byte, count int) []byte {
	for count > 0 {
		toCopy := count
		if toCopy > len(zeroBytes) {
			toCopy = len(zeroBytes)
		}
		buf = append(buf, zeroBytes[:toCopy]...)
		count -= toCopy
	}
	return buf
}
