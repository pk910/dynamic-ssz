// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"math"
	"strings"
	"testing"
)

func expectPanic(t *testing.T, substr string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic message, got: %v", r)
		}
		if !strings.Contains(msg, substr) {
			t.Fatalf("expected panic containing %q, got: %q", substr, msg)
		}
	}()
	fn()
}

func TestMerkleizeWithMixinNumOverflow(t *testing.T) {
	w := NewWrapper()
	w.AppendUint64(1)
	expectPanic(t, "MerkleizeWithMixin: num", func() {
		w.MerkleizeWithMixin(0, math.MaxUint64, 1)
	})
}

func TestMerkleizeWithMixinLimitOverflow(t *testing.T) {
	w := NewWrapper()
	w.AppendUint64(1)
	expectPanic(t, "MerkleizeWithMixin: limit", func() {
		w.MerkleizeWithMixin(0, 1, math.MaxUint64)
	})
}

func TestMerkleizeProgressiveWithMixinNumOverflow(t *testing.T) {
	w := NewWrapper()
	w.AppendUint64(1)
	expectPanic(t, "MerkleizeProgressiveWithMixin: num", func() {
		w.MerkleizeProgressiveWithMixin(0, math.MaxUint64)
	})
}

func TestPutBitlistSizeOverflow(t *testing.T) {
	w := NewWrapper()
	// Craft a bitlist that produces a valid parse, then use MerkleizeWithMixin
	// overflow path. Since ParseBitlist's size is bounded by len(bb) which is int,
	// we test the overflow guard via the public method tests above.
	// Here we verify PutBitlist works correctly for a normal small bitlist.
	w.PutBitlist([]byte{0x07}, 256) // 2 bits set, sentinel at bit 2
}

func TestPutBitlistLimitOverflow(t *testing.T) {
	// On 64-bit, (maxSize+255)/256 can never exceed math.MaxInt64 because
	// math.MaxInt64 * 256 overflows uint64. This test only triggers on 32-bit.
	if math.MaxInt > math.MaxInt32 {
		t.Skip("limit overflow only possible on 32-bit platforms")
	}

	// On 32-bit: (549755813888 + 255) / 256 = 2147483649 > MaxInt32
	overflowMaxSize := uint64(math.MaxInt32+1) * 256

	w := NewWrapper()
	expectPanic(t, "PutBitlist: limit", func() {
		w.PutBitlist([]byte{0x01}, overflowMaxSize) // sentinel only, size=0
	})
}

func TestPutBitlistSizeOverflow32Bit(t *testing.T) {
	if math.MaxInt > math.MaxInt32 {
		t.Skip("size overflow only possible on 32-bit platforms")
	}

	// On 32-bit, math.MaxInt == MaxInt32 == 2147483647.
	// ParseBitlist computes size = 8*(len(buf)-1) + msb.
	// We need size > MaxInt32, so len(buf) must be > MaxInt32/8 + 1 = 268435457.
	bufLen := math.MaxInt32/8 + 2 // 268435458
	buf := make([]byte, bufLen)
	buf[bufLen-1] = 0x80 // sentinel bit at position 7

	w := NewWrapper()
	expectPanic(t, "PutBitlist: size", func() {
		w.PutBitlist(buf, math.MaxUint64)
	})
}

func TestPutProgressiveBitlistSizeOverflow32Bit(t *testing.T) {
	if math.MaxInt > math.MaxInt32 {
		t.Skip("size overflow only possible on 32-bit platforms")
	}

	bufLen := math.MaxInt32/8 + 2
	buf := make([]byte, bufLen)
	buf[bufLen-1] = 0x80

	w := NewWrapper()
	expectPanic(t, "PutProgressiveBitlist: size", func() {
		w.PutProgressiveBitlist(buf)
	})
}

func TestPutProgressiveBitlistNormal(t *testing.T) {
	w := NewWrapper()
	// Verify normal operation doesn't panic
	w.PutProgressiveBitlist([]byte{0x07}) // 2 bits, sentinel at bit 2
}
