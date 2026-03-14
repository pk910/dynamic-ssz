// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"errors"
	"fmt"
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
	err := &SszError{Err: ErrOffset}
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

	expected := ".Block.Body.Slot: unexpected end of SSZ: need 4 bytes"
	if wrapped.Error() != expected {
		t.Errorf("got %q, want %q", wrapped.Error(), expected)
	}
}

func TestSszError_ErrorWithIndexSegment(t *testing.T) {
	err := NewSszError(ErrOffset, "consumed 10, expected 12")
	wrapped := ErrorWithPath(err, "[3]")
	wrapped = ErrorWithPath(wrapped, "Attestations")
	wrapped = ErrorWithPath(wrapped, "Body")

	expected := ".Body.Attestations[3]: incorrect offset: consumed 10, expected 12"
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

func TestSszError_ErrorsAs(t *testing.T) {
	err := NewSszError(ErrListTooBig, "len=999, max=128")
	wrapped := ErrorWithPath(err, "Validators")

	var sszErr *SszError
	if !errors.As(wrapped, &sszErr) {
		t.Fatal("errors.As should match *SszError")
	}

	if sszErr.Err != ErrListTooBig {
		t.Errorf("got base %v, want ErrListTooBig", sszErr.Err)
	}

	if len(sszErr.Path) != 1 || sszErr.Path[0] != "Validators" {
		t.Errorf("got path %v, want [Validators]", sszErr.Path)
	}
}

func TestErrorWithPath_NonSszError(t *testing.T) {
	plain := fmt.Errorf("some other error")
	wrapped := ErrorWithPath(plain, "Field")

	var sszErr *SszError
	if !errors.As(wrapped, &sszErr) {
		t.Fatal("errors.As should match *SszError for wrapped non-SSZ error")
	}

	if sszErr.Err != plain {
		t.Errorf("got base %v, want original error", sszErr.Err)
	}

	if sszErr.Message != "" {
		t.Errorf("got message %q, want empty", sszErr.Message)
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
	err := NewSszError(ErrBitlistNotTerminated, "")
	expected := "bitlist misses mandatory termination bit"
	if err.Error() != expected {
		t.Errorf("got %q, want %q", err.Error(), expected)
	}
}
