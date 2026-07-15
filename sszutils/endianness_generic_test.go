// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

//go:build dynssz_endian_generic

package sszutils

import "testing"

// TestUint64SliceFallbackPaths runs the reference-vector checks with
// hostLittleEndian forced to false, so the per-element fallback paths of the
// bulk uint64 slice helpers — including the branch selecting them — execute on
// any host. Overriding the value is only possible under the
// dynssz_endian_generic build tag, which replaces the compile-time constant
// with the runtime-detected var declaration; the fallback paths are portable,
// so forcing them is safe regardless of the actual host byte order.
func TestUint64SliceFallbackPaths(t *testing.T) {
	orig := hostLittleEndian
	hostLittleEndian = false
	t.Cleanup(func() { hostLittleEndian = orig })

	t.Run("MarshalUint64Slice", checkMarshalUint64Slice)
	t.Run("UnmarshalUint64Slice", checkUnmarshalUint64Slice)
	t.Run("EncodeUint64Slice", checkEncodeUint64Slice)
	t.Run("DecodeUint64Slice", checkDecodeUint64Slice)
	t.Run("HashUint64Slice", checkHashUint64Slice)
}
