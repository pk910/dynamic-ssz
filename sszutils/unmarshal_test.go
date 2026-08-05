// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"bytes"
	"testing"
)

// UnmarshalFixedBytesSlice bulk-copies contiguous bytes into fixed-size
// byte-array elements; the empty destination reads nothing.
func TestUnmarshalFixedBytesSlice(t *testing.T) {
	type root [4]byte
	dst := make([]root, 2)
	UnmarshalFixedBytesSlice(dst, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	if dst[0] != (root{1, 2, 3, 4}) || dst[1] != (root{5, 6, 7, 8}) {
		t.Fatalf("decoded %v", dst)
	}

	UnmarshalFixedBytesSlice([]root{}, []byte{1, 2, 3, 4})
}

// SizeListSlice sizes eagerly when the decoder knows its input length and
// grows from the credible byte count when it does not.
func TestSizeListSlice(t *testing.T) {
	buffered := NewBufferDecoder(make([]byte, 64))
	got := SizeListSlice(buffered, []uint64(nil), 4, 8)
	if len(got) != 4 {
		t.Fatalf("buffered decoder must size eagerly, got len %d", len(got))
	}

	// An unknown-length decoder seeds the slice from the bytes that have
	// actually arrived (none yet here), not from the claimed count.
	streamed := NewStreamDecoder(bytes.NewReader(make([]byte, 16)), -1, 0)
	got = SizeListSlice(streamed, []uint64(nil), 1<<20, 8)
	if got == nil || len(got) != 0 {
		t.Fatalf("undelivered bytes must not size the allocation, got len %d", len(got))
	}
}
