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

// ---------------------------------------------------------------------------
// Error constructor functions
//
// Each function creates a fresh sszError wrapping the appropriate sentinel.
// Using dedicated constructors instead of inline NewSszError/NewSszErrorf
// keeps error messages consistent across codegen and reflection, and
// reduces boilerplate in generated code.
// ---------------------------------------------------------------------------

// --- ErrUnexpectedEOF constructors ---

// ErrFixedFieldsEOFFn is returned when the buffer is too short for
// the fixed-size portion of a container.
func ErrFixedFieldsEOFFn(have, needed any) error {
	return &sszError{
		err:     ErrUnexpectedEOF,
		message: fmt.Sprintf("not enough data for fixed fields (have %v, needed %v)", have, needed),
	}
}

// ErrNeedBytesFn is returned when a primitive type cannot be decoded
// because the buffer is too short (e.g. "need 4 bytes for uint32").
func ErrNeedBytesFn(needed int, typeName string) error {
	if needed == 1 {
		return &sszError{
			err:     ErrUnexpectedEOF,
			message: fmt.Sprintf("need 1 byte for %s", typeName),
		}
	}
	return &sszError{
		err:     ErrUnexpectedEOF,
		message: fmt.Sprintf("need %d bytes for %s", needed, typeName),
	}
}

// ErrByteVectorEOFFn is returned when the buffer is too short for a
// byte vector / byte array.
func ErrByteVectorEOFFn(have, needed any) error {
	return &sszError{
		err:     ErrUnexpectedEOF,
		message: fmt.Sprintf("not enough data for byte vector (have %v, needed %v)", have, needed),
	}
}

// ErrVectorElementsEOFFn is returned when the buffer is too short for
// static vector elements.
func ErrVectorElementsEOFFn(have, needed any) error {
	return &sszError{
		err:     ErrUnexpectedEOF,
		message: fmt.Sprintf("not enough data for vector elements (have %v, needed %v)", have, needed),
	}
}

// ErrVectorOffsetsEOFFn is returned when the buffer is too short for
// the vector offset table.
func ErrVectorOffsetsEOFFn(have, needed any) error {
	return &sszError{
		err:     ErrUnexpectedEOF,
		message: fmt.Sprintf("not enough data for vector offsets (have %v, needed %v)", have, needed),
	}
}

// ErrListOffsetsEOFFn is returned when the buffer is too short for the
// list offset table.
func ErrListOffsetsEOFFn(have, needed any) error {
	return &sszError{
		err:     ErrUnexpectedEOF,
		message: fmt.Sprintf("not enough data for list offsets (have %v, needed %v)", have, needed),
	}
}

// ErrListNotAlignedFn is returned when a list's byte length is not an
// exact multiple of the element size.
func ErrListNotAlignedFn(length, elemSize any) error {
	return &sszError{
		err:     ErrUnexpectedEOF,
		message: fmt.Sprintf("list length %v is not a multiple of element size %v", length, elemSize),
	}
}

// ErrInvalidListStartOffsetFn is returned when the first offset in a
// dynamic list is malformed (not a multiple of 4 or out of range).
func ErrInvalidListStartOffsetFn(offset, bufLen any) error {
	return &sszError{
		err:     ErrUnexpectedEOF,
		message: fmt.Sprintf("invalid list start offset %v (length %v)", offset, bufLen),
	}
}

// ErrUnionSelectorEOFFn is returned when there is no byte available to
// read the union selector.
func ErrUnionSelectorEOFFn() error {
	return &sszError{err: ErrUnexpectedEOF, message: "need 1 byte for union selector"}
}

// ErrUnionVariantEOFFn is returned when the buffer is too short for
// a union variant value.
func ErrUnionVariantEOFFn(have, needed any) error {
	return &sszError{
		err:     ErrUnexpectedEOF,
		message: fmt.Sprintf("not enough data for union variant (have %v, needed %v)", have, needed),
	}
}

// ErrOptionalFlagEOFFn is returned when there is no byte available to
// read the optional presence flag.
func ErrOptionalFlagEOFFn() error {
	return &sszError{err: ErrUnexpectedEOF, message: "need 1 byte for optional presence flag"}
}

// ErrOptionalValueEOFFn is returned when the buffer is too short for
// an optional value.
func ErrOptionalValueEOFFn() error {
	return &sszError{err: ErrUnexpectedEOF, message: "not enough data for optional value"}
}

// --- ErrOffset constructors ---

// ErrFirstOffsetMismatchFn is returned when the first dynamic field's
// offset does not equal the expected static-part length.
func ErrFirstOffsetMismatchFn(offset, expected any) error {
	return &sszError{
		err:     ErrOffset,
		message: fmt.Sprintf("first offset %v does not match expected %v", offset, expected),
	}
}

// ErrOffsetOutOfRangeFn is returned when a field offset is not
// monotonically increasing or exceeds the data bounds.
func ErrOffsetOutOfRangeFn(offset, prev, limit any) error {
	return &sszError{
		err:     ErrOffset,
		message: fmt.Sprintf("offset %v out of range (prev %v, max %v)", offset, prev, limit),
	}
}

// ErrFieldNotConsumedFn is returned when a static-size field did not
// advance the decoder position by exactly the expected amount.
func ErrFieldNotConsumedFn(pos, expected any) error {
	return &sszError{
		err:     ErrOffset,
		message: fmt.Sprintf("field consumed to position %v, expected %v", pos, expected),
	}
}

// ErrTrailingDataFn is returned when a dynamic field or element has
// unconsumed bytes after decoding.
func ErrTrailingDataFn(trailing any) error {
	return &sszError{
		err:     ErrOffset,
		message: fmt.Sprintf("%v bytes trailing data", trailing),
	}
}

// ErrElementOffsetOutOfRangeFn is returned when a dynamic collection
// element's offset is out of the valid data range.
func ErrElementOffsetOutOfRangeFn(end, start, limit any) error {
	return &sszError{
		err:     ErrOffset,
		message: fmt.Sprintf("element offset %v out of range (start %v, max %v)", end, start, limit),
	}
}

// ErrStaticElementNotConsumedFn is returned when a static collection
// element did not advance the decoder position by exactly the expected
// amount.
func ErrStaticElementNotConsumedFn(pos, expected any) error {
	return &sszError{
		err:     ErrOffset,
		message: fmt.Sprintf("element consumed to position %v, expected %v", pos, expected),
	}
}

// --- ErrVectorLength constructors ---

// ErrVectorLengthFn is returned when a vector or fixed-size byte array
// has more elements than the schema allows.
func ErrVectorLengthFn(length, limit any) error {
	return &sszError{
		err:     ErrVectorLength,
		message: fmt.Sprintf("vector length %v exceeds limit %v", length, limit),
	}
}

// ErrVectorSizeExceedsArrayFn is returned when a dynamic size expression
// yields a vector size larger than the backing Go array.
func ErrVectorSizeExceedsArrayFn(dynamicSize, arrayLen any) error {
	return &sszError{
		err:     ErrVectorLength,
		message: fmt.Sprintf("dynamic vector size %v exceeds array length %v", dynamicSize, arrayLen),
	}
}

// --- ErrInvalidValueRange constructors ---

// ErrBitvectorPaddingFn is returned when a bitvector's padding bits
// (above the declared bit-size) are non-zero.
func ErrBitvectorPaddingFn() error {
	return &sszError{
		err:     ErrInvalidValueRange,
		message: "bitvector padding bits are not zero",
	}
}

// ErrBitlistNotTerminatedFn is returned when a bitlist is missing its
// mandatory termination bit.
func ErrBitlistNotTerminatedFn() error {
	return &sszError{
		err:     ErrInvalidValueRange,
		message: "bitlist missing termination bit",
	}
}

// ErrInvalidBoolValueFn is returned when a bool byte is neither 0 nor 1.
func ErrInvalidBoolValueFn() error {
	return &sszError{
		err:     ErrInvalidValueRange,
		message: "bool value must be 0 or 1",
	}
}

// ErrInvalidUnionVariantFn is returned when a union selector byte does
// not match any declared variant.
func ErrInvalidUnionVariantFn() error {
	return &sszError{
		err:     ErrInvalidValueRange,
		message: "invalid union variant selector",
	}
}

// ErrUnionTypeMismatchFn is returned when the concrete type stored in
// a union's Data field does not match the expected variant type.
func ErrUnionTypeMismatchFn() error {
	return &sszError{
		err:     ErrInvalidValueRange,
		message: "union variant type mismatch",
	}
}

// ErrTimeTypeExpectedFn is returned when a uint64-backed time field
// does not hold a time.Time value.
func ErrTimeTypeExpectedFn(got any) error {
	return &sszError{
		err:     ErrInvalidValueRange,
		message: fmt.Sprintf("time.Time type expected, got %v", got),
	}
}

// ErrBigIntTypeExpectedFn is returned when a BigInt SSZ field does not
// hold a big.Int value.
func ErrBigIntTypeExpectedFn(got any) error {
	return &sszError{
		err:     ErrInvalidValueRange,
		message: fmt.Sprintf("big.Int type expected, got %v", got),
	}
}

// ErrLargeUintLengthFn is returned when a uint128/uint256 backing
// array has an unexpected byte length.
func ErrLargeUintLengthFn(got, expected any) error {
	return &sszError{
		err:     ErrInvalidValueRange,
		message: fmt.Sprintf("large uint type does not have expected data length (%v != %v)", got, expected),
	}
}

// --- ErrListTooBig constructors ---

// ErrListLengthFn is returned when a list's element count exceeds the
// declared maximum.
func ErrListLengthFn(length, limit any) error {
	return &sszError{
		err:     ErrListTooBig,
		message: fmt.Sprintf("list length %v exceeds maximum %v", length, limit),
	}
}

// ErrBitlistLengthFn is returned when a bitlist's bit count exceeds
// the declared maximum.
func ErrBitlistLengthFn(length, limit any) error {
	return &sszError{
		err:     ErrListTooBig,
		message: fmt.Sprintf("bitlist length %v exceeds maximum %v", length, limit),
	}
}

// --- ErrNotImplemented constructors ---

// ErrUnknownTypeFn is returned when the type dispatcher encounters a
// type it does not handle.
func ErrUnknownTypeFn(typeName any) error {
	return &sszError{
		err:     ErrNotImplemented,
		message: fmt.Sprintf("unknown type: %v", typeName),
	}
}

// ErrCustomTypeNotSupportedFn is returned when codegen encounters a
// custom type that cannot be handled by the code generator.
func ErrCustomTypeNotSupportedFn() error {
	return &sszError{err: ErrNotImplemented, message: "custom type not supported"}
}

// --- ErrPlatformOverflow constructors ---

// ErrPlatformOverflowFn is returned when a SSZ size or count exceeds
// the platform's integer range (e.g. >31 bits on 32-bit systems).
func ErrPlatformOverflowFn(description string, value any) error {
	return &sszError{
		err:     ErrPlatformOverflow,
		message: fmt.Sprintf("%s %v exceeds platform int max", description, value),
	}
}
