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

// Sentinel errors for type introspection (type cache / descriptor building).
// These are returned during schema analysis of Go types, not at
// marshal/unmarshal time. Check with errors.Is(err, sszutils.ErrUnsupportedType) etc.
var (
	// ErrUnsupportedType is returned when a Go type kind is fundamentally
	// incompatible with SSZ (e.g. maps, channels, functions, interfaces).
	ErrUnsupportedType = fmt.Errorf("unsupported type")

	// ErrTypeMismatch is returned when the SSZ type assigned via tag or
	// auto-detection does not match the Go type's kind (e.g. ssz-type:"uint64"
	// on a bool field).
	ErrTypeMismatch = fmt.Errorf("type mismatch")

	// ErrInvalidTag is returned when a struct tag is malformed or contains
	// an unrecognized value (e.g. ssz-type:"foobar", non-numeric ssz-size).
	ErrInvalidTag = fmt.Errorf("invalid tag")

	// ErrInvalidConstraint is returned when tag values are syntactically
	// valid but semantically wrong (e.g. ssz-size:2 on a bool, bit-size on
	// a non-bitvector, vector size exceeding array length, duplicate ssz-index).
	ErrInvalidConstraint = fmt.Errorf("invalid constraint")

	// ErrExtendedTypeDisabled is returned when an extended SSZ type (signed
	// integers, floats, optionals, big.Int) is used without enabling the
	// ExtendedTypes flag on the TypeCache.
	ErrExtendedTypeDisabled = fmt.Errorf("extended type not enabled")

	// ErrMissingInterface is returned when a required method or interface
	// is not found on a type (e.g. missing GetDescriptorType method,
	// custom type without fastssz marshaler/hasher).
	ErrMissingInterface = fmt.Errorf("missing interface")
)

// sszError is an internal structured error type for SSZ operations. It wraps a
// base sentinel error (e.g. ErrUnexpectedEOF) with a detail message and a
// field path that is built up as the error bubbles through the call stack.
//
// Downstream consumers can use errors.Is(err, sszutils.ErrUnexpectedEOF) to
// check the error category, and the exported helper functions ErrorPath,
// ErrorMessage, and ErrorSentinel to inspect details.
type sszError struct {
	// err is the underlying sentinel error (e.g. ErrUnexpectedEOF, ErrOffset).
	err error

	// message provides additional context about the error.
	message string

	// path holds field segments collected while the error bubbles up.
	// Segments are appended at each level (innermost first), then reversed
	// in Error() to produce a jq-style path like "Block.Body.Attestations[3]".
	path []string
}

// Error builds a human-readable error string with the full field path.
func (e *sszError) Error() string {
	var b strings.Builder

	if len(e.path) > 0 {
		// path is stored innermost-first, so iterate in reverse for jq-style output.
		for i := len(e.path) - 1; i >= 0; i-- {
			seg := e.path[i]
			if i == len(e.path)-1 || (seg != "" && seg[0] == '[') {
				b.WriteString(seg)
			} else {
				b.WriteByte('.')
				b.WriteString(seg)
			}
		}

		b.WriteString(": ")
	}

	b.WriteString(e.err.Error())

	if e.message != "" {
		b.WriteString(": ")
		b.WriteString(e.message)
	}

	return b.String()
}

// Unwrap returns the base sentinel error, enabling errors.Is() checks.
func (e *sszError) Unwrap() error {
	return e.err
}

// Path returns the field path of the sszError.
func (e *sszError) Path() []string {
	return e.path
}

// Message returns the detail message of the sszError.
func (e *sszError) Message() string {
	return e.message
}

// NewSszError creates a new sszError with the given sentinel and detail message.
func NewSszError(base error, msg string) error {
	return &sszError{err: base, message: msg}
}

// NewSszErrorf creates a new sszError with a formatted detail message.
func NewSszErrorf(base error, format string, args ...any) error {
	return &sszError{err: base, message: fmt.Sprintf(format, args...)}
}

// ErrorWithPath appends a path segment to an sszError as it bubbles up.
// If err is not already an sszError, it is wrapped in one.
// Segments are collected innermost-first and reversed when formatting.
func ErrorWithPath(err error, segment string) error {
	var se *sszError
	if errors.As(err, &se) {
		se.path = append(se.path, segment)
		return se
	}

	return &sszError{err: err, path: []string{segment}}
}

// ErrorWithPathf appends a formatted path segment to an sszError as it bubbles up.
// If err is not already an sszError, it is wrapped in one.
// Segments are collected innermost-first and reversed when formatting.
func ErrorWithPathf(err error, format string, args ...any) error {
	return ErrorWithPath(err, fmt.Sprintf(format, args...))
}
