// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import "io"

var zeroBytes []byte

func ZeroBytes() []byte {
	if len(zeroBytes) == 0 {
		zeroBytes = make([]byte, 1024)
	}
	return zeroBytes
}

// AppendZeroPadding appends the specified number of zero bytes to buf
func AppendZeroPadding(buf []byte, count int) []byte {
	if len(zeroBytes) == 0 {
		zeroBytes = ZeroBytes()
	}
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

// AppendZeroPaddingWriter appends the specified number of zero bytes to buf
func AppendZeroPaddingWriter(writer io.Writer, count int) error {
	if len(zeroBytes) == 0 {
		zeroBytes = ZeroBytes()
	}
	for count > 0 {
		toCopy := count
		if toCopy > len(zeroBytes) {
			toCopy = len(zeroBytes)
		}
		_, err := writer.Write(zeroBytes[:toCopy])
		if err != nil {
			return err
		}
		count -= toCopy
	}
	return nil
}
