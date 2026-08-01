// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
)

// ============================================================================
// BufferDecoder Tests
// ============================================================================

func TestBufferDecoder_PushLimit_ClampToLastLimit(t *testing.T) {
	dec := NewBufferDecoder(make([]byte, 10))

	dec.PushLimit(20)

	if dec.GetLength() != 10 {
		t.Errorf("expected length 10, got %d", dec.GetLength())
	}
}

func TestBufferDecoder_PopLimit_EmptyLimits(t *testing.T) {
	dec := NewBufferDecoder(make([]byte, 10))

	remaining := dec.PopLimit()

	if remaining != 0 {
		t.Errorf("expected 0, got %d", remaining)
	}
}

func TestBufferDecoder_PopLimit_MultipleLimits(t *testing.T) {
	dec := NewBufferDecoder(make([]byte, 10))

	dec.PushLimit(8)
	dec.PushLimit(3)

	if dec.GetLength() != 3 {
		t.Errorf("expected 3, got %d", dec.GetLength())
	}

	remaining := dec.PopLimit()
	if remaining != 3 {
		t.Errorf("expected 3, got %d", remaining)
	}
	if dec.GetLength() != 8 {
		t.Errorf("expected 8, got %d", dec.GetLength())
	}

	remaining = dec.PopLimit()
	if remaining != 8 {
		t.Errorf("expected 8, got %d", remaining)
	}
}

func TestBufferDecoder_DecodeBytes_InsufficientData(t *testing.T) {
	dec := NewBufferDecoder([]byte{0x01, 0x02})

	buf := make([]byte, 5)
	_, err := dec.DecodeBytes(buf)
	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestBufferDecoder_DecodeBytesBuf_NegativeLength(t *testing.T) {
	dec := NewBufferDecoder([]byte{0x01, 0x02, 0x03})

	result, err := dec.DecodeBytesBuf(-1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3, got %d", len(result))
	}
}

func TestBufferDecoder_DecodeBytesBuf_ExceedsLimit(t *testing.T) {
	dec := NewBufferDecoder([]byte{0x01, 0x02, 0x03})

	_, err := dec.DecodeBytesBuf(10)
	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestBufferDecoder_DecodeOffset_InsufficientData(t *testing.T) {
	dec := NewBufferDecoder([]byte{0x01, 0x02})

	_, err := dec.DecodeOffset()
	if !errors.Is(err, ErrUnexpectedEOF) {
		t.Errorf("expected ErrUnexpectedEOF, got %v", err)
	}
}

// ============================================================================
// Marshal Tests
// ============================================================================

func TestMarshalOffset(t *testing.T) {
	result := MarshalOffset(nil, 0x01020304)

	if len(result) != 4 {
		t.Fatalf("expected 4 bytes, got %d", len(result))
	}
	if !bytes.Equal(result, []byte{0x04, 0x03, 0x02, 0x01}) {
		t.Errorf("expected [04 03 02 01], got %v", result)
	}
}

func TestUpdateOffset(t *testing.T) {
	buf := make([]byte, 4)
	UpdateOffset(buf, 0x01020304)

	if !bytes.Equal(buf, []byte{0x04, 0x03, 0x02, 0x01}) {
		t.Errorf("expected [04 03 02 01], got %v", buf)
	}
}

// ============================================================================
// SpecValue Tests
// ============================================================================

type mockDynamicSpecs struct {
	values map[string]uint64
	err    error
}

func (m *mockDynamicSpecs) ResolveSpecValue(name string) (bool, uint64, error) {
	if m.err != nil {
		return false, 0, m.err
	}
	val, ok := m.values[name]
	return ok, val, nil
}

func TestResolveSpecValueWithDefault_Error(t *testing.T) {
	testErr := errors.New("spec error")
	ds := &mockDynamicSpecs{err: testErr}

	_, err := ResolveSpecValueWithDefault(ds, "foo", 42)
	if !errors.Is(err, testErr) {
		t.Errorf("expected %v, got %v", testErr, err)
	}
}

func TestResolveSpecValueWithDefault_NotFound(t *testing.T) {
	ds := &mockDynamicSpecs{values: map[string]uint64{}}

	val, err := ResolveSpecValueWithDefault(ds, "missing", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("expected default 42, got %d", val)
	}
}

func TestResolveSpecValueWithDefault_Found(t *testing.T) {
	ds := &mockDynamicSpecs{values: map[string]uint64{"foo": 100}}

	val, err := ResolveSpecValueWithDefault(ds, "foo", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 100 {
		t.Errorf("expected 100, got %d", val)
	}
}

func TestResolveSpecValueWithDefault_ResolvedZeroFallsBack(t *testing.T) {
	// A resolved value of 0 is an invalid size/limit and falls back to the
	// positive static default.
	ds := &mockDynamicSpecs{values: map[string]uint64{"foo": 0}}

	val, err := ResolveSpecValueWithDefault(ds, "foo", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("expected fallback to static 42, got %d", val)
	}

	// When the static fallback is itself 0, resolving to 0 is an error: there is
	// no positive size/limit to use.
	_, err = ResolveSpecValueWithDefault(ds, "foo", 0)
	if err == nil || !errors.Is(err, ErrInvalidConstraint) {
		t.Errorf("expected ErrInvalidConstraint for resolved 0 with no static fallback, got: %v", err)
	}

	// A name absent from the spec set keeps the static value unchanged.
	dsEmpty := &mockDynamicSpecs{values: map[string]uint64{}}
	val, err = ResolveSpecValueWithDefault(dsEmpty, "missing", 7)
	if err != nil {
		t.Fatalf("unexpected error for not-found: %v", err)
	}
	if val != 7 {
		t.Errorf("expected the static fallback for not-found, got %d", val)
	}

	// Unless that static value is itself 0, which is the "no static value"
	// placeholder: the type said its value comes from the spec and the spec does
	// not define it, so there is nothing to fall back to. Same dead end as
	// resolving to 0.
	_, err = ResolveSpecValueWithDefault(dsEmpty, "missing", 0)
	if err == nil || !errors.Is(err, ErrInvalidConstraint) {
		t.Errorf("expected ErrInvalidConstraint for not-found with no static fallback, got: %v", err)
	}
}

// ============================================================================
// TreeRoot Tests
// ============================================================================

func TestCalculateLimit_NonZero(t *testing.T) {
	result := CalculateLimit(10, 5, 32)
	expected := uint64((10*32 + 31) / 32)
	if result != expected {
		t.Errorf("expected %d, got %d", expected, result)
	}
}

func TestCalculateLimit_ZeroWithZeroItems(t *testing.T) {
	result := CalculateLimit(0, 0, 0)
	if result != 1 {
		t.Errorf("expected 1, got %d", result)
	}
}

func TestCalculateLimit_ZeroWithNonZeroItems(t *testing.T) {
	result := CalculateLimit(0, 5, 0)
	if result != 5 {
		t.Errorf("expected 5, got %d", result)
	}
}

type mockHashWalker struct {
	appendCalled bool
	appendData   []byte
}

func (m *mockHashWalker) Hash() []byte          { return nil }
func (m *mockHashWalker) AppendBool(_ bool)     {}
func (m *mockHashWalker) AppendUint8(_ uint8)   {}
func (m *mockHashWalker) AppendUint16(_ uint16) {}
func (m *mockHashWalker) AppendUint32(_ uint32) {}
func (m *mockHashWalker) AppendUint64(i uint64) {
	m.appendCalled = true
	m.appendData = MarshalUint64(m.appendData, i)
}
func (m *mockHashWalker) AppendBytes32(_ []byte)                 {}
func (m *mockHashWalker) PutUint64Array(_ []uint64, _ ...uint64) {}
func (m *mockHashWalker) PutUint64(_ uint64)                     {}
func (m *mockHashWalker) PutUint32(_ uint32)                     {}
func (m *mockHashWalker) PutUint16(_ uint16)                     {}
func (m *mockHashWalker) PutUint8(_ uint8)                       {}
func (m *mockHashWalker) PutBitlist(_ []byte, _ uint64)          {}
func (m *mockHashWalker) PutProgressiveBitlist(_ []byte)         {}
func (m *mockHashWalker) PutBool(_ bool)                         {}
func (m *mockHashWalker) PutBytes(_ []byte)                      {}
func (m *mockHashWalker) FillUpTo32()                            {}
func (m *mockHashWalker) Append(i []byte) {
	m.appendCalled = true
	m.appendData = append(m.appendData, i...)
}
func (m *mockHashWalker) Index() int                                           { return 0 }
func (m *mockHashWalker) CurrentIndex() int                                    { return 0 }
func (m *mockHashWalker) StartTree(_ TreeType) int                             { return 0 }
func (m *mockHashWalker) Collapse()                                            {}
func (m *mockHashWalker) WithTemp(_ func(tmp []byte) []byte)                   {}
func (m *mockHashWalker) Merkleize(_ int)                                      {}
func (m *mockHashWalker) MerkleizeWithMixin(_ int, _, _ uint64)                {}
func (m *mockHashWalker) MerkleizeProgressive(_ int)                           {}
func (m *mockHashWalker) MerkleizeProgressiveWithMixin(_ int, _ uint64)        {}
func (m *mockHashWalker) MerkleizeProgressiveWithActiveFields(_ int, _ []byte) {}
func (m *mockHashWalker) HashRoot() ([32]byte, error)                          { return [32]byte{}, nil }

func TestHashUint64Slice_Empty(t *testing.T) {
	hh := &mockHashWalker{}
	var empty []uint64
	HashUint64Slice(hh, empty)
	if hh.appendCalled {
		t.Error("expected Append not to be called for empty slice")
	}
}

func TestHashUint64Slice_NonEmpty(t *testing.T) {
	hh := &mockHashWalker{}
	input := []uint64{1, 2}
	HashUint64Slice(hh, input)
	if !hh.appendCalled {
		t.Error("expected Append to be called")
	}
	if len(hh.appendData) != 16 {
		t.Errorf("expected 16 bytes appended, got %d", len(hh.appendData))
	}
}

func TestNextPowerOfTwo(t *testing.T) {
	tests := []struct {
		input    uint64
		expected uint64
	}{
		{1, 1},
		{2, 2},
		{3, 4},
		{4, 4},
		{5, 8},
		{7, 8},
		{8, 8},
		{9, 16},
	}
	for _, tt := range tests {
		result := NextPowerOfTwo(tt.input)
		if result != tt.expected {
			t.Errorf("NextPowerOfTwo(%d): expected %d, got %d", tt.input, tt.expected, result)
		}
	}
}

// ============================================================================
// Unmarshal Tests
// ============================================================================

func TestUnmarshallUint8(t *testing.T) {
	result := UnmarshallUint8([]byte{0x42})
	if result != 0x42 {
		t.Errorf("expected 0x42, got 0x%x", result)
	}
}

func TestUnmarshalBool(t *testing.T) {
	if !UnmarshalBool([]byte{0x01}) {
		t.Error("expected true")
	}
	if UnmarshalBool([]byte{0x00}) {
		t.Error("expected false")
	}
}

// ============================================================================
// Uint64 Slice Tests
// ============================================================================

func TestMarshalUint64Slice_Empty(t *testing.T) {
	var empty []uint64
	result := MarshalUint64Slice(nil, empty)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d bytes", len(result))
	}
}

func TestMarshalUint64Slice_NonEmpty(t *testing.T) {
	input := []uint64{1, 2}
	result := MarshalUint64Slice(nil, input)
	if len(result) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(result))
	}
	// Verify round-trip
	output := make([]uint64, 2)
	UnmarshalUint64Slice(output, result)
	if output[0] != 1 || output[1] != 2 {
		t.Errorf("expected [1, 2], got %v", output)
	}
}

func TestEncodeUint64Slice_Empty(t *testing.T) {
	enc := NewBufferEncoder(nil)
	var empty []uint64
	EncodeUint64Slice(enc, empty)
	if enc.GetPosition() != 0 {
		t.Errorf("expected position 0, got %d", enc.GetPosition())
	}
}

func TestEncodeUint64Slice_NonEmpty(t *testing.T) {
	enc := NewBufferEncoder(make([]byte, 0, 16))
	input := []uint64{1, 2}
	EncodeUint64Slice(enc, input)
	if enc.GetPosition() != 16 {
		t.Errorf("expected position 16, got %d", enc.GetPosition())
	}
	buf := enc.GetBuffer()
	// Verify by unmarshalling
	output := make([]uint64, 2)
	UnmarshalUint64Slice(output, buf)
	if output[0] != 1 || output[1] != 2 {
		t.Errorf("expected [1, 2], got %v", output)
	}
}

func TestUnmarshalUint64Slice_Empty(t *testing.T) {
	var empty []uint64
	// Should not panic
	UnmarshalUint64Slice(empty, nil)
}

func TestDecodeUint64Slice_Empty(t *testing.T) {
	dec := NewBufferDecoder(nil)
	var empty []uint64
	err := DecodeUint64Slice(dec, empty)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestDecodeUint64Slice_NonEmpty(t *testing.T) {
	// Marshal first, then decode
	input := []uint64{42, 99}
	data := MarshalUint64Slice(nil, input)
	dec := NewBufferDecoder(data)
	output := make([]uint64, 2)
	err := DecodeUint64Slice(dec, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output[0] != 42 || output[1] != 99 {
		t.Errorf("expected [42, 99], got %v", output)
	}
}

func TestReadOffset(t *testing.T) {
	result := ReadOffset([]byte{0x04, 0x03, 0x02, 0x01})
	if result != 0x01020304 {
		t.Errorf("expected 0x01020304, got 0x%x", result)
	}
}

func TestExpandSlice_Grow(t *testing.T) {
	src := []int{1, 2}
	result := ExpandSlice(src, 5)
	if len(result) != 5 {
		t.Errorf("expected length 5, got %d", len(result))
	}
}

func TestExpandSlice_Shrink(t *testing.T) {
	src := []int{1, 2, 3, 4, 5}
	result := ExpandSlice(src, 3)
	if len(result) != 3 {
		t.Errorf("expected length 3, got %d", len(result))
	}
}

func TestExpandSlice_SameSize(t *testing.T) {
	src := []int{1, 2, 3}
	result := ExpandSlice(src, 3)
	if len(result) != 3 {
		t.Errorf("expected length 3, got %d", len(result))
	}
}

func TestExpandSlice_GrowWithinCapacityZeroesTail(t *testing.T) {
	src := []int{1, 2, 9, 8, 7}[:2]
	result := ExpandSlice(src, 5)
	if len(result) != 5 {
		t.Fatalf("expected length 5, got %d", len(result))
	}
	for i, want := range []int{1, 2, 0, 0, 0} {
		if result[i] != want {
			t.Fatalf("index %d: expected %d, got %d", i, want, result[i])
		}
	}
}

func TestDecodeSlicePreallocation(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		elemSize uint64
		want     int
	}{
		{name: "negative count", count: -1, elemSize: 8, want: 0},
		{name: "empty", count: 0, elemSize: 8, want: 0},
		{name: "zero sized element", count: 100_000, elemSize: 0, want: 100_000},
		{name: "within budget", count: 32, elemSize: 8, want: 32},
		{
			name:     "clamped to byte budget",
			count:    10_000,
			elemSize: 8,
			want:     maxDecodeSlicePreallocationBytes / 8,
		},
		{
			name:     "one element exceeds budget",
			count:    100,
			elemSize: maxDecodeSlicePreallocationBytes + 1,
			want:     1,
		},
		{
			name:     "large size cannot overflow",
			count:    100,
			elemSize: ^uint64(0),
			want:     1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := decodeSlicePreallocation(test.count, test.elemSize); got != test.want {
				t.Fatalf(
					"decodeSlicePreallocation(%d, %d) = %d, want %d",
					test.count,
					test.elemSize,
					got,
					test.want,
				)
			}
		})
	}
}

func TestPreallocateDecodeSlice(t *testing.T) {
	type wideElement [maxDecodeSlicePreallocationBytes]byte

	got := PreallocateDecodeSlice([]wideElement(nil), 512)
	if len(got) != 1 || cap(got) != 1 {
		t.Fatalf("wide preallocation len/cap = %d/%d, want 1/1", len(got), cap(got))
	}

	got[0][0] = 42
	chunkLen := cap(got)
	if chunkLen == len(got) {
		chunkLen = max(2, 8, len(got)*2)
	}
	got = GrowSlice(got, min(512, chunkLen), 512)
	if got[0][0] != 42 {
		t.Fatal("incremental growth lost the preallocated element")
	}
	if len(got) != 8 || cap(got) != 8 {
		t.Fatalf("grown len/cap = %d/%d, want the 8-element geometric chunk", len(got), cap(got))
	}

	small := PreallocateDecodeSlice([]uint64(nil), 16)
	if len(small) != 16 || cap(small) != 16 {
		t.Fatalf("small preallocation len/cap = %d/%d, want 16/16", len(small), cap(small))
	}
}

// ============================================================================
// ZeroBytes Tests
// ============================================================================

func TestZeroBytes(t *testing.T) {
	result := ZeroBytes()
	if len(result) != 1024 {
		t.Errorf("expected length 1024, got %d", len(result))
	}
	for i, b := range result {
		if b != 0 {
			t.Errorf("expected zero at %d, got %d", i, b)
			break
		}
	}
}

func TestAppendZeroPadding_Small(t *testing.T) {
	result := AppendZeroPadding(nil, 10)
	if len(result) != 10 {
		t.Errorf("expected 10, got %d", len(result))
	}
	for i, b := range result {
		if b != 0 {
			t.Errorf("expected zero at %d, got %d", i, b)
		}
	}
}

func TestAppendZeroPadding_LargerThanZeroBytes(t *testing.T) {
	result := AppendZeroPadding(nil, 2000)
	if len(result) != 2000 {
		t.Errorf("expected 2000, got %d", len(result))
	}
	for i, b := range result {
		if b != 0 {
			t.Errorf("expected zero at %d, got %d", i, b)
			break
		}
	}
}

func TestAppendZeroPadding_Zero(t *testing.T) {
	result := AppendZeroPadding(nil, 0)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

// TestCalculateLimitOverflow verifies the 128-bit overflow-safe limit math so a
// large ssz-max cannot wrap to a small limit (which would collide merkle depths).
func TestCalculateLimitOverflow(t *testing.T) {
	cases := []struct {
		name                        string
		maxCapacity, numItems, size uint64
		want                        uint64
	}{
		{"small", 4, 1, 8, 1},
		{"one", 1, 1, 8, 1},
		{"zeroCapEmpty", 0, 0, 8, 1},
		{"zeroCapItems", 0, 3, 8, 3},
		{"noOverflow61", 1 << 61, 1, 8, 1 << 59}, // (2^61*8)/32 = 2^59, fits
		{"overflowWraps", (1 << 61) + 1, 1, 8, (1 << 59) + 1},
		{"hugeClamp", 1 << 62, 1, 64, 1 << 63},            // 2^62*64 = 2^68 -> /32 = 2^63
		{"overflowClamp", 1 << 60, 0, 1 << 10, 1<<64 - 1}, // 2^70 / 32 = 2^65 -> clamps to MaxUint64
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CalculateLimit(tc.maxCapacity, tc.numItems, tc.size); got != tc.want {
				t.Fatalf("CalculateLimit(%d,%d,%d) = %d, want %d", tc.maxCapacity, tc.numItems, tc.size, got, tc.want)
			}
		})
	}

	// distinct large capacities must yield distinct limits (no overflow collision)
	if CalculateLimit(1, 1, 8) == CalculateLimit((1<<61)+1, 1, 8) {
		t.Fatal("overflow collision: small and huge capacities share a limit")
	}
}

// CalculateBitlistLimit must compute ceil(maxSize/256) and must not collapse to
// a tiny value when maxSize is near math.MaxUint64.
func TestCalculateBitlistLimit(t *testing.T) {
	cases := []struct {
		max  uint64
		want uint64
	}{
		{0, 0},
		{1, 1},
		{255, 1},
		{256, 1},
		{257, 2},
		{512, 2},
		{513, 3},
	}
	for _, c := range cases {
		if got := CalculateBitlistLimit(c.max); got != c.want {
			t.Errorf("CalculateBitlistLimit(%d) = %d, want %d", c.max, got, c.want)
		}
	}

	maxU := ^uint64(0)
	if got := CalculateBitlistLimit(maxU); got != maxU/256+1 {
		t.Errorf("CalculateBitlistLimit(MaxUint64) = %d, want %d", got, maxU/256+1)
	}
	if CalculateBitlistLimit(maxU) <= CalculateBitlistLimit(256) {
		t.Error("a huge maxSize must not collapse to the same limit as a small one")
	}
}

// ExpandSlice must treat a negative size as empty instead of panicking on a
// negative slice bound.
func TestExpandSlice_NegativeSize(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ExpandSlice panicked on negative size: %v", r)
		}
	}()

	out := ExpandSlice([]byte{1, 2, 3}, -1)
	if out == nil || len(out) != 0 {
		t.Errorf("expected non-nil empty slice, got %#v", out)
	}
}

// GetOffsetSlice must not panic on a negative size.
func TestGetOffsetSlice_NegativeSize(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GetOffsetSlice panicked on negative size: %v", r)
		}
	}()

	out := GetOffsetSlice(-1)
	if len(out) != 0 {
		t.Errorf("expected empty slice, got len %d", len(out))
	}
}

// The low-level unmarshal helpers must zero-pad short buffers instead of
// panicking.
func TestUnmarshallHelpers_ShortBuffer(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unmarshal helper panicked on short buffer: %v", r)
		}
	}()

	if v := UnmarshallUint64([]byte{1, 2, 3}); v != 0x030201 {
		t.Errorf("UnmarshallUint64 short = %d, want %d", v, 0x030201)
	}
	if v := UnmarshallUint32([]byte{1}); v != 1 {
		t.Errorf("UnmarshallUint32 short = %d, want 1", v)
	}
	if v := UnmarshallUint16([]byte{}); v != 0 {
		t.Errorf("UnmarshallUint16 empty = %d, want 0", v)
	}
	if v := UnmarshallUint8(nil); v != 0 {
		t.Errorf("UnmarshallUint8 nil = %d, want 0", v)
	}
	if UnmarshalBool(nil) {
		t.Error("UnmarshalBool nil = true, want false")
	}
	if v := ReadOffset([]byte{1, 2}); v != 0x0201 {
		t.Errorf("ReadOffset short = %d, want %d", v, 0x0201)
	}

	// Full-length buffers must decode normally.
	if v := UnmarshallUint64([]byte{1, 0, 0, 0, 0, 0, 0, 0}); v != 1 {
		t.Errorf("UnmarshallUint64 = %d, want 1", v)
	}
	if v := UnmarshallUint32([]byte{2, 0, 0, 0}); v != 2 {
		t.Errorf("UnmarshallUint32 = %d, want 2", v)
	}
	if v := UnmarshallUint16([]byte{3, 0}); v != 3 {
		t.Errorf("UnmarshallUint16 = %d, want 3", v)
	}
	if !UnmarshalBool([]byte{1}) {
		t.Error("UnmarshalBool([1]) = false, want true")
	}
	if v := ReadOffset([]byte{4, 0, 0, 0}); v != 4 {
		t.Errorf("ReadOffset = %d, want 4", v)
	}
}

// A negative PushLimit followed by a read-all DecodeBytesBuf must not panic.
func TestBufferDecoder_PushLimitNegative(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("decoder panicked on negative limit: %v", r)
		}
	}()

	dec := NewBufferDecoder([]byte{1, 2, 3, 4, 5})
	dec.PushLimit(-1)
	if _, err := dec.DecodeBytesBuf(-1); err == nil {
		// An empty remaining range is acceptable; the requirement is no panic.
		t.Log("DecodeBytesBuf returned nil error on empty range")
	}
}

// BufferEncoder back-patch and padding helpers must not panic on out-of-range
// positions; padding beyond the current capacity grows the buffer like any
// other write.
func TestBufferEncoder_OutOfRange(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("encoder panicked on out-of-range position: %v", r)
		}
	}()

	enc := NewBufferEncoder(make([]byte, 16))
	enc.EncodeOffsetAt(-1, 42)
	enc.EncodeOffsetAt(1<<30, 42)
	enc.EncodeZeroPadding(-1)
	enc.EncodeZeroPadding(64)
	if got := len(enc.GetBuffer()); got != 16+64 {
		t.Errorf("buffer length after padding = %d, want %d", got, 16+64)
	}
}

// Writing into the un-grown area is a bug: an under-sized buffer grows with
// append semantics, so every write helper produces the same output as with a
// pre-sized buffer and back-patched offsets stay valid across the growth.
func TestBufferEncoder_GrowsOnDemand(t *testing.T) {
	small := NewBufferEncoder(make([]byte, 0, 2))
	presized := NewBufferEncoder(make([]byte, 0, 1024))

	for _, enc := range []*BufferEncoder{small, presized} {
		enc.EncodeUint64(0x1122334455667788)
		enc.EncodeBool(true)
		enc.EncodeUint8(7)
		enc.EncodeUint16(0xAABB)
		enc.EncodeUint32(0xDDCCBBAA)
		enc.EncodeOffset(20)
		enc.EncodeBytes([]byte{1, 2, 3, 4, 5})
		enc.EncodeZeroPadding(3)
		enc.EncodeOffsetAt(0, 99)
	}

	want := presized.GetBuffer()
	got := small.GetBuffer()
	if len(got) != 8+1+1+2+4+4+5+3 {
		t.Errorf("unexpected output length %d", len(got))
	}
	if !bytes.Equal(got, want) {
		t.Errorf("grown output diverges from pre-sized output:\n  got:  %x\n  want: %x", got, want)
	}
}

// ZeroBytes and AppendZeroPadding must be race-free under concurrent use.
func TestZeroBytesConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ZeroBytes()
			_ = AppendZeroPadding(make([]byte, 0, 64), 40)
		}()
	}
	wg.Wait()
}

// These helpers are called from generated code, so they are only reached
// indirectly from another package's tests. Exercising them directly keeps both
// regimes — known region length and open region — honest.

func TestGrowSlice(t *testing.T) {
	// A negative size yields an empty, non-nil slice rather than panicking.
	if got := GrowSlice([]int(nil), -1, -1); got == nil || len(got) != 0 {
		t.Fatalf("GrowSlice(nil, -1, -1) = %v, want empty non-nil", got)
	}

	// Growth is geometric, so repeated single-element growth must not
	// reallocate every time.
	s := GrowSlice([]int(nil), 1, -1)
	if len(s) != 1 {
		t.Fatalf("len = %d, want 1", len(s))
	}
	if cap(s) < 8 {
		t.Fatalf("cap = %d, want at least the 8-element floor", cap(s))
	}
	firstCap := cap(s)
	for i := 2; i <= firstCap; i++ {
		s = GrowSlice(s, i, -1)
	}
	if cap(s) != firstCap {
		t.Fatalf("cap changed to %d while growing within capacity", cap(s))
	}
	s[0] = 42
	s = GrowSlice(s, firstCap+1, -1)
	if cap(s) < 2*firstCap {
		t.Fatalf("cap = %d, want at least %d (geometric growth)", cap(s), 2*firstCap)
	}
	if s[0] != 42 {
		t.Fatal("growth lost existing elements")
	}

	// Shrinking re-slices in place; re-growing must zero the reused tail so a
	// short decode cannot expose the previous contents.
	s = GrowSlice(s, 4, -1)
	for i := range s {
		s[i] = 7
	}
	s = GrowSlice(s, 2, -1)
	s = GrowSlice(s, 4, -1)
	if s[2] != 0 || s[3] != 0 {
		t.Fatalf("regrown tail = %v, want zeroed", s[2:4])
	}

	// An allocation limit clamps newly allocated capacity.
	bounded := GrowSlice(make([]int, 80), 100, 100)
	if len(bounded) != 100 || cap(bounded) != 100 {
		t.Fatalf("bounded growth len/cap = %d/%d, want 100/100", len(bounded), cap(bounded))
	}
	floored := GrowSlice(make([]int, 1), 2, 2)
	if len(floored) != 2 || cap(floored) != 2 {
		t.Fatalf("bounded floor growth len/cap = %d/%d, want 2/2", len(floored), cap(floored))
	}
	storage := make([]int, 1, 160)
	reused := GrowSlice(storage, 100, 100)
	if len(reused) != 100 || cap(reused) != 160 || &reused[0] != &storage[0] {
		t.Fatalf("reuse len/cap = %d/%d, want 100/160 over the existing backing array", len(reused), cap(reused))
	}

	// A limit below the requested size cannot be honoured: the result must still
	// hold size elements, so the limit is raised to size rather than producing a
	// capacity smaller than the length, which make would reject outright.
	underLimit := GrowSlice([]int(nil), 4, 2)
	if len(underLimit) != 4 || cap(underLimit) != 4 {
		t.Fatalf("under-limit growth len/cap = %d/%d, want 4/4", len(underLimit), cap(underLimit))
	}
}

func TestDecodeByteListInto(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	t.Run("known-length reuses destination", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		dst := make([]byte, 0, 64)
		got, err := DecodeByteListInto(dec, dst, -1)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("got %v, want %v", got, data)
		}
		if &got[0] != &dst[:1][0] {
			t.Fatal("destination was not reused")
		}
	})

	t.Run("known-length rejects over max before allocating", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		if _, err := DecodeByteListInto(dec, nil, 4); !errors.Is(err, ErrListTooBig) {
			t.Fatalf("err = %v, want ErrListTooBig", err)
		}
	})

	t.Run("known-length empty region", func(t *testing.T) {
		dec := NewBufferDecoder(nil)
		got, err := DecodeByteListInto(dec, nil, -1)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, err %v", got, err)
		}
	})

	t.Run("known-length short read", func(t *testing.T) {
		dec := NewStreamDecoder(&shortReader{data: data[:4], maxRead: 2}, len(data), 8)
		if _, err := DecodeByteListInto(dec, nil, -1); !errors.Is(err, ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want ErrUnexpectedEOF", err)
		}
	})

	t.Run("open region reads to EOF", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
		dec.PushOpenLimit()
		got, err := DecodeByteListInto(dec, nil, -1)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("got %v, want %v", got, data)
		}
	})

	t.Run("open region reports over-max as a list error", func(t *testing.T) {
		// The cap is applied while reading, so the payload is never allocated,
		// but the error still has to read as a list-limit violation rather than
		// a stream-size one.
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
		dec.PushOpenLimit()
		if _, err := DecodeByteListInto(dec, nil, 4); !errors.Is(err, ErrListTooBig) {
			t.Fatalf("err = %v, want ErrListTooBig", err)
		}
	})

	t.Run("open region surfaces read errors", func(t *testing.T) {
		want := errors.New("boom")
		dec := NewUnknownStreamDecoder(&errReader{data: data, errAfter: 2, err: want}, 8, 0)
		dec.PushOpenLimit()
		if _, err := DecodeByteListInto(dec, nil, -1); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})

	t.Run("open region keeps the stream allowance distinct from the cap", func(t *testing.T) {
		// The allowance (4) fires while the caller's cap (64) is untouched.
		// Both causes surface as ErrStreamTooLarge, and reporting the allowance
		// overrun as a list-limit violation names a length the payload never
		// declared.
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 2, 4)
		dec.PushOpenLimit()
		_, err := DecodeByteListInto(dec, nil, 64)
		if !errors.Is(err, ErrStreamTooLarge) {
			t.Fatalf("err = %v, want ErrStreamTooLarge", err)
		}
		if errors.Is(err, ErrListTooBig) {
			t.Fatalf("stream-allowance overrun reported as a list-limit violation: %v", err)
		}
	})
}

func TestDecodeUint64ListInto(t *testing.T) {
	data := make([]byte, 24)
	for i, v := range []uint64{1, 2, 3} {
		for b := range 8 {
			data[i*8+b] = byte(v >> (8 * b))
		}
	}

	t.Run("known-length", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		got, err := DecodeUint64ListInto(dec, []uint64(nil), -1)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got) != 3 || got[0] != 1 || got[2] != 3 {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("known-length misaligned", func(t *testing.T) {
		dec := NewBufferDecoder(data[:20])
		// Misalignment is reported through the unexpected-EOF sentinel.
		_, err := DecodeUint64ListInto(dec, []uint64(nil), -1)
		if !errors.Is(err, ErrUnexpectedEOF) || !strings.Contains(err.Error(), "not a multiple") {
			t.Fatalf("err = %v, want a list-alignment error", err)
		}
	})

	t.Run("known-length over max", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		if _, err := DecodeUint64ListInto(dec, []uint64(nil), 2); !errors.Is(err, ErrListTooBig) {
			t.Fatalf("err = %v, want ErrListTooBig", err)
		}
	})

	t.Run("known-length short read", func(t *testing.T) {
		dec := NewStreamDecoder(bytes.NewReader(data[:8]), len(data), 8)
		if _, err := DecodeUint64ListInto(dec, []uint64(nil), -1); !errors.Is(err, ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want ErrUnexpectedEOF", err)
		}
	})

	t.Run("open region reads to EOF", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
		dec.PushOpenLimit()
		got, err := DecodeUint64ListInto(dec, []uint64(nil), -1)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got) != 3 || got[0] != 1 || got[2] != 3 {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("open region enforces max per element", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
		dec.PushOpenLimit()
		if _, err := DecodeUint64ListInto(dec, []uint64(nil), 2); !errors.Is(err, ErrListTooBig) {
			t.Fatalf("err = %v, want ErrListTooBig", err)
		}
	})

	t.Run("open region rejects a trailing partial element", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(data[:20]), 8, 0)
		dec.PushOpenLimit()
		if _, err := DecodeUint64ListInto(dec, []uint64(nil), -1); !errors.Is(err, ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want ErrUnexpectedEOF", err)
		}
	})

	t.Run("open region surfaces read errors", func(t *testing.T) {
		want := errors.New("boom")
		dec := NewUnknownStreamDecoder(&errReader{data: data, errAfter: 2, err: want}, 8, 0)
		dec.PushOpenLimit()
		if _, err := DecodeUint64ListInto(dec, []uint64(nil), -1); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})

	t.Run("named uint64 element type", func(t *testing.T) {
		type slot uint64
		dec := NewBufferDecoder(data)
		got, err := DecodeUint64ListInto(dec, []slot(nil), -1)
		if err != nil || len(got) != 3 || got[1] != slot(2) {
			t.Fatalf("got %v, err %v", got, err)
		}
	})
}

func TestDecodeDelegateBuffer(t *testing.T) {
	data := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	t.Run("exact size", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		got, err := DecodeDelegateBuffer(dec, 4)
		if err != nil || !bytes.Equal(got, data[:4]) {
			t.Fatalf("got %v, err %v", got, err)
		}
		if dec.GetPosition() != 4 {
			t.Fatalf("position = %d, want 4", dec.GetPosition())
		}
	})

	t.Run("exact size past the end", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		if _, err := DecodeDelegateBuffer(dec, 99); !errors.Is(err, ErrUnexpectedEOF) {
			t.Fatalf("err = %v, want ErrUnexpectedEOF", err)
		}
	})

	t.Run("whole known region", func(t *testing.T) {
		dec := NewBufferDecoder(data)
		got, err := DecodeDelegateBuffer(dec, -1)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("got %v, err %v", got, err)
		}
	})

	t.Run("whole open region", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(data), 8, 0)
		dec.PushOpenLimit()
		got, err := DecodeDelegateBuffer(dec, -1)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("got %v, err %v", got, err)
		}
	})
}

func TestRegionEmpty(t *testing.T) {
	t.Run("bounded", func(t *testing.T) {
		dec := NewBufferDecoder([]byte{1})
		if empty, err := RegionEmpty(dec); err != nil || empty {
			t.Fatalf("empty = %v, err = %v", empty, err)
		}
		if _, err := dec.DecodeUint8(); err != nil {
			t.Fatal(err)
		}
		if empty, err := RegionEmpty(dec); err != nil || !empty {
			t.Fatalf("empty = %v, err = %v", empty, err)
		}
	})

	t.Run("open region at EOF", func(t *testing.T) {
		dec := NewUnknownStreamDecoder(bytes.NewReader(nil), 8, 0)
		dec.PushOpenLimit()
		if empty, err := RegionEmpty(dec); err != nil || !empty {
			t.Fatalf("empty = %v, err = %v", empty, err)
		}
	})

	t.Run("open region surfaces read errors", func(t *testing.T) {
		want := errors.New("boom")
		dec := NewUnknownStreamDecoder(&errReader{data: nil, errAfter: 0, err: want}, 8, 0)
		dec.PushOpenLimit()
		if _, err := RegionEmpty(dec); !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
	})
}

// TestBufferDecoder_Available covers Available() on a buffer-backed decoder,
// which reports the whole remaining region.
func TestBufferDecoder_Available(t *testing.T) {
	dec := NewBufferDecoder([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	if got := dec.Available(); got != 8 {
		t.Fatalf("Available() = %d, want 8", got)
	}
	if _, err := dec.DecodeUint32(); err != nil {
		t.Fatalf("DecodeUint32: %v", err)
	}
	if got := dec.Available(); got != 4 {
		t.Fatalf("Available() after consuming 4 bytes = %d, want 4", got)
	}
}

// TestCredibleCount_Guards covers the two early-return guards in CredibleCount:
// a non-positive count, and a non-positive element size on an unknown-length
// decoder (where the count cannot be bounded from delivered bytes).
func TestCredibleCount_Guards(t *testing.T) {
	// count <= 0 short-circuits to 0.
	if got := CredibleCount(NewBufferDecoder(nil), 0, 4); got != 0 {
		t.Fatalf("CredibleCount(_, 0, 4) = %d, want 0", got)
	}
	if got := CredibleCount(NewBufferDecoder(nil), -3, 4); got != 0 {
		t.Fatalf("CredibleCount(_, -3, 4) = %d, want 0", got)
	}
	// A known-length decoder trusts the count unchanged.
	if got := CredibleCount(NewBufferDecoder(make([]byte, 4)), 7, 4); got != 7 {
		t.Fatalf("CredibleCount(buffer, 7, 4) = %d, want 7", got)
	}
	// An unknown-length decoder with a non-positive element size cannot bound the
	// count from delivered bytes, so it is returned unchanged.
	unk := NewUnknownStreamDecoder(bytes.NewReader(nil), 8, 0)
	if unk.LengthKnown() {
		t.Fatal("precondition: fresh unknown decoder must report LengthKnown()==false")
	}
	if got := CredibleCount(unk, 5, 0); got != 5 {
		t.Fatalf("CredibleCount(unknown, 5, 0) = %d, want 5", got)
	}
}
