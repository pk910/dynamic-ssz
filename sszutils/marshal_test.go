// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"bytes"
	"testing"
)

// MarshalFixedBytesSlice bulk-appends fixed-size byte-array elements; the
// empty slice appends nothing.
func TestMarshalFixedBytesSlice(t *testing.T) {
	type root [4]byte
	got := MarshalFixedBytesSlice([]byte{0xaa}, []root{{1, 2, 3, 4}, {5, 6, 7, 8}})
	want := []byte{0xaa, 1, 2, 3, 4, 5, 6, 7, 8}
	if !bytes.Equal(got, want) {
		t.Fatalf("marshaled % x, want % x", got, want)
	}

	if got := MarshalFixedBytesSlice([]byte{0xaa}, []root{}); !bytes.Equal(got, []byte{0xaa}) {
		t.Fatalf("empty slice must append nothing, got % x", got)
	}
}
