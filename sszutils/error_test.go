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
