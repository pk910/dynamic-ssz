// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"errors"
	"testing"
)

func TestSszError_ErrorNoPath(t *testing.T) {
	err := NewSszError(ErrUnexpectedEOF, "need 4 bytes, got 2")
	expected := "unexpected end of SSZ: need 4 bytes, got 2"
	if err.Error() != expected {
		t.Errorf("got %q, want %q", err.Error(), expected)
	}
}

func TestSszError_ErrorNoMessage(t *testing.T) {
	err := NewSszError(ErrOffset, "")
	expected := "incorrect offset"
	if err.Error() != expected {
		t.Errorf("got %q, want %q", err.Error(), expected)
	}
}

func TestSszError_ErrorWithPath(t *testing.T) {
	// Simulate error bubbling: innermost field added first.
	err := NewSszError(ErrUnexpectedEOF, "need 4 bytes")
	wrapped := ErrorWithPath(err, "Slot")
	wrapped = ErrorWithPath(wrapped, "Body")
	wrapped = ErrorWithPath(wrapped, "Block")

	expected := "Block.Body.Slot: unexpected end of SSZ: need 4 bytes"
	if wrapped.Error() != expected {
		t.Errorf("got %q, want %q", wrapped.Error(), expected)
	}
}

func TestSszError_ErrorWithIndexSegment(t *testing.T) {
	err := NewSszError(ErrOffset, "consumed 10, expected 12")
	wrapped := ErrorWithPath(err, "[3]")
	wrapped = ErrorWithPath(wrapped, "Attestations")
	wrapped = ErrorWithPath(wrapped, "Body")

	expected := "Body.Attestations[3]: incorrect offset: consumed 10, expected 12"
	if wrapped.Error() != expected {
		t.Errorf("got %q, want %q", wrapped.Error(), expected)
	}
}

func TestSszError_UnwrapErrorsIs(t *testing.T) {
	err := NewSszError(ErrUnexpectedEOF, "detail")
	wrapped := ErrorWithPath(err, "Field")

	if !errors.Is(wrapped, ErrUnexpectedEOF) {
		t.Error("errors.Is should match ErrUnexpectedEOF")
	}

	if errors.Is(wrapped, ErrOffset) {
		t.Error("errors.Is should not match ErrOffset")
	}
}

func TestNewSszErrorf(t *testing.T) {
	err := NewSszErrorf(ErrVectorLength, "got %d, want %d", 5, 10)
	expected := "incorrect vector length: got 5, want 10"
	if err.Error() != expected {
		t.Errorf("got %q, want %q", err.Error(), expected)
	}
}

func TestSszError_EmptyPath(t *testing.T) {
	err := NewSszError(ErrInvalidValueRange, "")
	expected := "invalid value range"
	if err.Error() != expected {
		t.Errorf("got %q, want %q", err.Error(), expected)
	}
}

func TestSszError_AliasBackwardCompat(t *testing.T) {
	// ErrBitlistNotTerminated and ErrInvalidUnionVariant are aliases for ErrInvalidValueRange.
	if !errors.Is(ErrBitlistNotTerminated, ErrInvalidValueRange) {
		t.Error("ErrBitlistNotTerminated should match ErrInvalidValueRange")
	}

	if !errors.Is(ErrInvalidUnionVariant, ErrInvalidValueRange) {
		t.Error("ErrInvalidUnionVariant should match ErrInvalidValueRange")
	}
}

func TestSszError_PlatformOverflow(t *testing.T) {
	err := NewSszError(ErrPlatformOverflow, "size exceeds int")
	expected := "value exceeds platform integer range: size exceeds int"
	if err.Error() != expected {
		t.Errorf("got %q, want %q", err.Error(), expected)
	}

	if !errors.Is(err, ErrPlatformOverflow) {
		t.Error("errors.Is should match ErrPlatformOverflow")
	}
}

func TestSszError_IntrospectionSentinels(t *testing.T) {
	tests := []struct {
		name     string
		sentinel error
		msg      string
		wantBase string
	}{
		{"ErrUnsupportedType", ErrUnsupportedType, "maps are not supported", "unsupported type"},
		{"ErrTypeMismatch", ErrTypeMismatch, "uint64 on bool", "type mismatch"},
		{"ErrInvalidTag", ErrInvalidTag, "bad ssz-type value", "invalid tag"},
		{"ErrInvalidConstraint", ErrInvalidConstraint, "ssz-size:2 on bool", "invalid constraint"},
		{"ErrExtendedTypeDisabled", ErrExtendedTypeDisabled, "signed ints disabled", "extended type not enabled"},
		{"ErrMissingInterface", ErrMissingInterface, "no GetDescriptorType", "missing interface"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewSszError(tt.sentinel, tt.msg)

			if !errors.Is(err, tt.sentinel) {
				t.Errorf("errors.Is should match %s", tt.name)
			}

			expected := tt.wantBase + ": " + tt.msg
			if err.Error() != expected {
				t.Errorf("got %q, want %q", err.Error(), expected)
			}
		})
	}
}

func TestSszError_IntrospectionSentinelsAreDistinct(t *testing.T) {
	sentinels := []error{
		ErrUnsupportedType,
		ErrTypeMismatch,
		ErrInvalidTag,
		ErrInvalidConstraint,
		ErrExtendedTypeDisabled,
		ErrMissingInterface,
	}

	// Ensure introspection sentinels don't match each other.
	for i, a := range sentinels {
		for j, b := range sentinels {
			if i != j && errors.Is(a, b) {
				t.Errorf("sentinel %d should not match sentinel %d", i, j)
			}
		}
	}

	// Ensure introspection sentinels don't match runtime sentinels.
	runtimeSentinels := []error{
		ErrUnexpectedEOF, ErrOffset, ErrInvalidValueRange,
		ErrVectorLength, ErrListTooBig, ErrNotImplemented, ErrPlatformOverflow,
	}
	for _, intro := range sentinels {
		for _, rt := range runtimeSentinels {
			if errors.Is(intro, rt) || errors.Is(rt, intro) {
				t.Errorf("introspection sentinel %v should not match runtime sentinel %v", intro, rt)
			}
		}
	}
}

func TestSszError_IntrospectionWithPath(t *testing.T) {
	// Simulate a type analysis error bubbling through container fields.
	err := NewSszError(ErrTypeMismatch, "uint64 can only be represented by uint64 types, got bool")
	wrapped := ErrorWithPath(err, "Slot")
	wrapped = ErrorWithPath(wrapped, "Header")
	wrapped = ErrorWithPath(wrapped, "Block")

	expected := "Block.Header.Slot: type mismatch: uint64 can only be represented by uint64 types, got bool"
	if wrapped.Error() != expected {
		t.Errorf("got %q, want %q", wrapped.Error(), expected)
	}

	if !errors.Is(wrapped, ErrTypeMismatch) {
		t.Error("errors.Is should match ErrTypeMismatch through path wrapping")
	}
}

func TestSszError_PathMethod(t *testing.T) {
	err := NewSszError(ErrOffset, "detail")
	wrapped := ErrorWithPath(err, "Inner")
	wrapped = ErrorWithPath(wrapped, "Outer")

	var se *sszError
	if !errors.As(wrapped, &se) {
		t.Fatal("expected sszError type")
	}

	path := se.Path()
	if len(path) != 2 || path[0] != "Inner" || path[1] != "Outer" {
		t.Errorf("unexpected path: %v", path)
	}
}

func TestSszError_MessageMethod(t *testing.T) {
	err := NewSszError(ErrUnexpectedEOF, "need 8 bytes for uint64")

	var se *sszError
	if !errors.As(err, &se) {
		t.Fatal("expected sszError type")
	}

	msg := se.Message()
	if msg != "need 8 bytes for uint64" {
		t.Errorf("unexpected message: %q", msg)
	}
}

func TestSszError_PathMethodEmpty(t *testing.T) {
	err := NewSszError(ErrOffset, "detail")

	var se *sszError
	if !errors.As(err, &se) {
		t.Fatal("expected sszError type")
	}

	path := se.Path()
	if len(path) != 0 {
		t.Errorf("expected empty path, got: %v", path)
	}
}

func TestSszError_MessageMethodEmpty(t *testing.T) {
	err := NewSszError(ErrOffset, "")

	var se *sszError
	if !errors.As(err, &se) {
		t.Fatal("expected sszError type")
	}

	msg := se.Message()
	if msg != "" {
		t.Errorf("expected empty message, got: %q", msg)
	}
}

func TestErrorConstructorFunctions(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		sentinel     error
		wantContains string
	}{
		// ErrUnexpectedEOF constructors
		{"ErrFixedFieldsEOFFn", ErrFixedFieldsEOFFn(10, 20), ErrUnexpectedEOF, "not enough data for fixed fields (have 10, needed 20)"},
		{"ErrNeedBytesFn_singular", ErrNeedBytesFn(1, "bool"), ErrUnexpectedEOF, "need 1 byte for bool"},
		{"ErrNeedBytesFn_plural", ErrNeedBytesFn(4, "uint32"), ErrUnexpectedEOF, "need 4 bytes for uint32"},
		{"ErrByteVectorEOFFn", ErrByteVectorEOFFn(5, 32), ErrUnexpectedEOF, "not enough data for byte vector (have 5, needed 32)"},
		{"ErrVectorElementsEOFFn", ErrVectorElementsEOFFn(8, 16), ErrUnexpectedEOF, "not enough data for vector elements (have 8, needed 16)"},
		{"ErrVectorOffsetsEOFFn", ErrVectorOffsetsEOFFn(4, 12), ErrUnexpectedEOF, "not enough data for vector offsets (have 4, needed 12)"},
		{"ErrListOffsetsEOFFn", ErrListOffsetsEOFFn(0, 8), ErrUnexpectedEOF, "not enough data for list offsets (have 0, needed 8)"},
		{"ErrListNotAlignedFn", ErrListNotAlignedFn(13, 4), ErrUnexpectedEOF, "list length 13 is not a multiple of element size 4"},
		{"ErrInvalidListStartOffsetFn", ErrInvalidListStartOffsetFn(99, 50), ErrUnexpectedEOF, "invalid list start offset 99 (length 50)"},
		{"ErrUnionSelectorEOFFn", ErrUnionSelectorEOFFn(), ErrUnexpectedEOF, "need 1 byte for union selector"},
		{"ErrUnionVariantEOFFn", ErrUnionVariantEOFFn(3, 10), ErrUnexpectedEOF, "not enough data for union variant (have 3, needed 10)"},
		{"ErrOptionalFlagEOFFn", ErrOptionalFlagEOFFn(), ErrUnexpectedEOF, "need 1 byte for optional presence flag"},
		{"ErrOptionalValueEOFFn", ErrOptionalValueEOFFn(), ErrUnexpectedEOF, "not enough data for optional value"},

		// ErrOffset constructors
		{"ErrFirstOffsetMismatchFn", ErrFirstOffsetMismatchFn(8, 12), ErrOffset, "first offset 8 does not match expected 12"},
		{"ErrOffsetOutOfRangeFn", ErrOffsetOutOfRangeFn(100, 50, 80), ErrOffset, "offset 100 out of range (prev 50, max 80)"},
		{"ErrFieldNotConsumedFn", ErrFieldNotConsumedFn(10, 12), ErrOffset, "field consumed to position 10, expected 12"},
		{"ErrTrailingDataFn", ErrTrailingDataFn(5), ErrOffset, "5 bytes trailing data"},
		{"ErrElementOffsetOutOfRangeFn", ErrElementOffsetOutOfRangeFn(99, 10, 50), ErrOffset, "element offset 99 out of range (start 10, max 50)"},
		{"ErrStaticElementNotConsumedFn", ErrStaticElementNotConsumedFn(6, 8), ErrOffset, "element consumed to position 6, expected 8"},

		// ErrVectorLength constructors
		{"ErrVectorLengthFn", ErrVectorLengthFn(10, 5), ErrVectorLength, "vector length 10 exceeds limit 5"},
		{"ErrVectorSizeExceedsArrayFn", ErrVectorSizeExceedsArrayFn(100, 50), ErrVectorLength, "dynamic vector size 100 exceeds array length 50"},

		// ErrInvalidValueRange constructors
		{"ErrBitvectorPaddingFn", ErrBitvectorPaddingFn(), ErrInvalidValueRange, "bitvector padding bits are not zero"},
		{"ErrBitlistNotTerminatedFn", ErrBitlistNotTerminatedFn(), ErrInvalidValueRange, "bitlist missing termination bit"},
		{"ErrInvalidBoolValueFn", ErrInvalidBoolValueFn(), ErrInvalidValueRange, "bool value must be 0 or 1"},
		{"ErrInvalidUnionVariantFn", ErrInvalidUnionVariantFn(), ErrInvalidValueRange, "invalid union variant selector"},
		{"ErrUnionTypeMismatchFn", ErrUnionTypeMismatchFn(), ErrInvalidValueRange, "union variant type mismatch"},
		{"ErrTimeTypeExpectedFn", ErrTimeTypeExpectedFn("MyStruct"), ErrInvalidValueRange, "time.Time type expected, got MyStruct"},
		{"ErrBigIntTypeExpectedFn", ErrBigIntTypeExpectedFn("NotBigInt"), ErrInvalidValueRange, "big.Int type expected, got NotBigInt"},
		{"ErrLargeUintLengthFn", ErrLargeUintLengthFn(17, 16), ErrInvalidValueRange, "large uint type does not have expected data length (17 != 16)"},

		// ErrListTooBig constructors
		{"ErrListLengthFn", ErrListLengthFn(100, 50), ErrListTooBig, "list length 100 exceeds maximum 50"},
		{"ErrBitlistLengthFn", ErrBitlistLengthFn(256, 128), ErrListTooBig, "bitlist length 256 exceeds maximum 128"},

		// ErrNotImplemented constructors
		{"ErrUnknownTypeFn", ErrUnknownTypeFn("mysteryType"), ErrNotImplemented, "unknown type: mysteryType"},
		{"ErrCustomTypeNotSupportedFn", ErrCustomTypeNotSupportedFn(), ErrNotImplemented, "custom type not supported"},

		// ErrPlatformOverflow constructors
		{"ErrPlatformOverflowFn", ErrPlatformOverflowFn("list count", 999999999), ErrPlatformOverflow, "list count 999999999 exceeds platform int max"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.sentinel) {
				t.Errorf("errors.Is should match sentinel %v", tt.sentinel)
			}

			got := tt.err.Error()
			if got != tt.sentinel.Error()+": "+tt.wantContains {
				t.Errorf("got %q, want %q", got, tt.sentinel.Error()+": "+tt.wantContains)
			}
		})
	}
}
