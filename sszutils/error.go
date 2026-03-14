// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors for SSZ operations. Downstream consumers can check error
// categories via errors.Is(err, sszutils.ErrOffset) etc.
var (
	// ErrUnexpectedEOF is returned when the SSZ input is shorter than the
	// type requires (e.g. not enough bytes to decode a uint64).
	ErrUnexpectedEOF = fmt.Errorf("unexpected end of SSZ")

	// ErrOffset is returned when an SSZ offset is out of range, does not
	// monotonically increase, or a field does not consume exactly the
	// byte range its offset pair implied.
	ErrOffset = fmt.Errorf("incorrect offset")

	// ErrInvalidValueRange is returned when an SSZ value is outside the
	// valid domain for its type (e.g. non-zero padding bits in a
	// bitvector, unterminated bitlist, or invalid union selector).
	ErrInvalidValueRange = fmt.Errorf("invalid value range")

	// ErrVectorLength is returned when a vector or fixed-length byte
	// array has a length that does not match the schema.
	ErrVectorLength = fmt.Errorf("incorrect vector length")

	// ErrListTooBig is returned when a list's length exceeds its declared
	// SSZ maximum.
	ErrListTooBig = fmt.Errorf("list length is higher than max value")

	// ErrNotImplemented is returned when the SSZ codec encounters a Go
	// type or feature it does not support.
	ErrNotImplemented = fmt.Errorf("not implemented")

	// ErrPlatformOverflow is returned when a SSZ length or count exceeds
	// the platform's integer range (>31-bit sizes on 32-bit platforms).
	ErrPlatformOverflow = fmt.Errorf("value exceeds platform integer range")

	// ErrBitlistNotTerminated is an alias for ErrInvalidValueRange,
	// retained for backward compatibility. New code should use
	// ErrInvalidValueRange with a descriptive message instead.
	ErrBitlistNotTerminated = ErrInvalidValueRange

	// ErrInvalidUnionVariant is an alias for ErrInvalidValueRange,
	// retained for backward compatibility. New code should use
	// ErrInvalidValueRange with a descriptive message instead.
	ErrInvalidUnionVariant = ErrInvalidValueRange
)

// SszError is a structured error type for SSZ operations. It wraps a base
// sentinel error (e.g. ErrUnexpectedEOF) with a detail message and a field
// path that is built up as the error bubbles through the call stack.
//
// Downstream consumers can use errors.Is(err, sszutils.ErrUnexpectedEOF) to
// check the error category, and errors.As(err, &sszErr) to inspect the path.
type SszError struct {
	// Err is the underlying sentinel error (e.g. ErrUnexpectedEOF, ErrOffset).
	Err error

	// Message provides additional context about the error.
	Message string

	// Path holds field segments collected while the error bubbles up.
	// Segments are appended at each level (innermost first), then reversed
	// in Error() to produce a jq-style path like ".Block.Body.Attestations[3]".
	Path []string
}

// Error builds a human-readable error string with the full field path.
func (e *SszError) Error() string {
	var b strings.Builder

	if len(e.Path) > 0 {
		// Path is stored innermost-first, so iterate in reverse for jq-style output.
		for i := len(e.Path) - 1; i >= 0; i-- {
			seg := e.Path[i]
			if seg != "" && seg[0] == '[' {
				b.WriteString(seg)
			} else {
				b.WriteByte('.')
				b.WriteString(seg)
			}
		}

		b.WriteString(": ")
	}

	b.WriteString(e.Err.Error())

	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}

	return b.String()
}

// Unwrap returns the base sentinel error, enabling errors.Is() checks.
func (e *SszError) Unwrap() error {
	return e.Err
}

// NewSszError creates a new SszError with the given sentinel and detail message.
func NewSszError(base error, msg string) *SszError {
	return &SszError{Err: base, Message: msg}
}

// NewSszErrorf creates a new SszError with a formatted detail message.
func NewSszErrorf(base error, format string, args ...any) *SszError {
	return &SszError{Err: base, Message: fmt.Sprintf(format, args...)}
}

// ErrorWithPath appends a path segment to an SszError as it bubbles up.
// If err is not already an SszError, it is wrapped in one.
// Segments are collected innermost-first and reversed when formatting.
func ErrorWithPath(err error, segment string) error {
	var sszErr *SszError
	if errors.As(err, &sszErr) {
		sszErr.Path = append(sszErr.Path, segment)
		return sszErr
	}

	return &SszError{Err: err, Path: []string{segment}}
}

// ErrorWithPathf appends a formatted path segment to an SszError as it bubbles up.
// If err is not already an SszError, it is wrapped in one.
// Segments are collected innermost-first and reversed when formatting.
func ErrorWithPathf(err error, format string, args ...any) error {
	return ErrorWithPath(err, fmt.Sprintf(format, args...))
}
