// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import "testing"

func BenchmarkMarshalUint64Slice(b *testing.B) {
	s := make([]uint64, 512)
	for i := range s {
		s[i] = uint64(i)
	}
	dst := make([]byte, 0, len(s)*8)
	b.ResetTimer()
	for range b.N {
		dst = MarshalUint64Slice(dst[:0], s)
	}
	_ = dst
}

func BenchmarkUnmarshalUint64Slice(b *testing.B) {
	buf := make([]byte, 512*8)
	dst := make([]uint64, 512)
	b.ResetTimer()
	for range b.N {
		UnmarshalUint64Slice(dst, buf)
	}
}
