// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"bytes"
	"errors"
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

func TestNextPowerOfTwo(t *testing.T) {
	tests := []struct {
		input    uint64
		expected uint
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

// ============================================================================
// ZeroBytes Tests
// ============================================================================

func TestZeroBytes(t *testing.T) {
	// Reset global for test isolation
	zeroBytes = nil

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
	zeroBytes = nil
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
	zeroBytes = nil
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
