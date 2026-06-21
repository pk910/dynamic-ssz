// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package hasher

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// progBitlistZeroTop builds a 257-bit bitlist with only bit 0 set, so bit 256
// (the single data bit of the 2nd chunk) is zero and the top 256-bit chunk is
// all-zero. This is the EIP-7916 case that exposed the chunk-count bug.
func progBitlistZeroTop() []byte {
	b := make([]byte, 257/8+1) // 33 bytes
	b[0] = 0x01                // bit 0 set
	b[257/8] |= 1 << (257 % 8) // termination bit at position 257
	return b
}

// TestParseProgressiveBitlistPadsTopChunk verifies that, unlike ParseBitlist,
// ParseProgressiveBitlist preserves all-zero top chunks by zero-padding the
// content to exactly ceil(size/256) 256-bit chunks. The progressive tree shape
// is defined by the chunk count, so dropping an all-zero top chunk would corrupt
// the root.
func TestParseProgressiveBitlistPadsTopChunk(t *testing.T) {
	bitlist := progBitlistZeroTop()

	// ParseBitlist trims the all-zero top chunk down to a single byte.
	trimmed, trimmedSize := ParseBitlist(nil, bitlist)
	if trimmedSize != 257 {
		t.Fatalf("ParseBitlist size: expected 257, got %d", trimmedSize)
	}
	if len(trimmed) != 1 {
		t.Fatalf("ParseBitlist should trim trailing zero bytes, got %d bytes", len(trimmed))
	}

	// ParseProgressiveBitlist keeps the full chunk count: ceil(257/256) = 2 chunks.
	padded, size := ParseProgressiveBitlist(nil, bitlist)
	if size != 257 {
		t.Fatalf("ParseProgressiveBitlist size: expected 257, got %d", size)
	}
	if len(padded) != 64 {
		t.Fatalf("ParseProgressiveBitlist should pad to 2 chunks (64 bytes), got %d", len(padded))
	}
	if padded[0] != 0x01 {
		t.Fatalf("expected bit 0 set, got %#x", padded[0])
	}
	for i := 1; i < 64; i++ {
		if padded[i] != 0x00 {
			t.Fatalf("expected zero padding at byte %d, got %#x", i, padded[i])
		}
	}
}

// TestParseProgressiveBitlistChunkCounts checks the padded length is always
// ceil(size/256)*32 across a range of bit lengths and patterns.
func TestParseProgressiveBitlistChunkCounts(t *testing.T) {
	for _, size := range []uint64{0, 1, 7, 8, 255, 256, 257, 511, 512, 513, 1024, 2000} {
		// build a bitlist with `size` data bits, all zero (worst case for trimming),
		// plus the termination bit at position `size`.
		byteLen := int(size)/8 + 1
		b := make([]byte, byteLen)
		b[int(size)/8] |= 1 << (size % 8)

		padded, gotSize := ParseProgressiveBitlist(nil, b)
		if gotSize != size {
			t.Fatalf("size %d: got size %d", size, gotSize)
		}
		want := int((size+255)/256) * 32
		if len(padded) != want {
			t.Fatalf("size %d: expected %d padded bytes, got %d", size, want, len(padded))
		}
	}
}

// progBitlistSpecRoot is the EIP-7916 hash tree root for the 257-bit bitlist
// (max 2000) with only bit 0 set. Cross-checked against ethereum/remerkleable
// and an independent raw-SHA256 implementation.
const progBitlistSpecRoot = "b039aa14167fdfd184839eb032e714ef89e0b42478e2db1ed1353759c200dda5"

// TestPutProgressiveBitlistAllZeroTopChunk pins the hasher's progressive bitlist
// root against the spec golden for the all-zero-top-chunk case. With a single
// container field the container root equals the field root, so the hasher root
// matches the struct-level golden.
func TestPutProgressiveBitlistAllZeroTopChunk(t *testing.T) {
	h := NewHasher()
	h.PutProgressiveBitlist(progBitlistZeroTop())

	got := hex.EncodeToString(h.buf)
	if got != progBitlistSpecRoot {
		t.Fatalf("progressive bitlist root mismatch:\n  got  %s\n  want %s", got, progBitlistSpecRoot)
	}
}

// TestParseProgressiveBitlistWithHasher exercises both the *Hasher fast path and
// the generic WithTemp fallback path.
func TestParseProgressiveBitlistWithHasher(t *testing.T) {
	bitlist := progBitlistZeroTop()

	t.Run("HasherPath", func(t *testing.T) {
		h := NewHasher()
		result, size := ParseProgressiveBitlistWithHasher(h, bitlist)
		if size != 257 {
			t.Fatalf("expected size 257, got %d", size)
		}
		if len(result) != 64 {
			t.Fatalf("expected 64 padded bytes, got %d", len(result))
		}
		if result[0] != 0x01 || !bytes.Equal(result[1:], make([]byte, 63)) {
			t.Fatalf("unexpected content: %x", result)
		}
	})

	t.Run("FallbackPath", func(t *testing.T) {
		mock := &mockHashWalker{tmp: make([]byte, 64)}
		result, size := ParseProgressiveBitlistWithHasher(mock, bitlist)
		if size != 257 {
			t.Fatalf("expected size 257, got %d", size)
		}
		if len(result) != 64 {
			t.Fatalf("expected 64 padded bytes, got %d", len(result))
		}
		if result[0] != 0x01 || !bytes.Equal(result[1:], make([]byte, 63)) {
			t.Fatalf("unexpected content: %x", result)
		}
	})
}

// TestPutProgressiveBitlistEmpty verifies the empty bitlist still produces a
// valid root (no chunks, ceil(0/256)=0).
func TestPutProgressiveBitlistEmpty(t *testing.T) {
	padded, size := ParseProgressiveBitlist(nil, []byte{})
	if size != 0 || len(padded) != 0 {
		t.Fatalf("empty bitlist: expected size 0 and no content, got size %d len %d", size, len(padded))
	}

	h := NewHasher()
	h.PutProgressiveBitlist([]byte{})
	if len(h.buf) != 32 {
		t.Fatalf("expected 32-byte root, got %d", len(h.buf))
	}
}
