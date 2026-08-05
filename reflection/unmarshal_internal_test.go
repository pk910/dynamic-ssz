// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection

import (
	"reflect"
	"testing"
)

// expandSliceValue guards against callers handing it a non-slice target or a
// negative length; both fall back to a fresh, never-negative allocation.
func TestExpandSliceValueDefensive(t *testing.T) {
	sliceT := reflect.TypeOf([]uint64{})

	grown := expandSliceValue(reflect.ValueOf("not a slice"), sliceT, 2)
	if grown.Kind() != reflect.Slice || grown.Len() != 2 {
		t.Fatalf("non-slice target must yield a fresh slice of the requested size, got %v len %d", grown.Kind(), grown.Len())
	}

	grown = expandSliceValue(reflect.ValueOf([]uint64{1}), sliceT, -1)
	if grown.Kind() != reflect.Slice || grown.Len() != 0 {
		t.Fatalf("negative size must yield an empty slice, got %v len %d", grown.Kind(), grown.Len())
	}
}
