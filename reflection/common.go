// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package reflection provides runtime reflection-based SSZ encoding, decoding,
// and hash tree root computation.
//
// It inspects Go struct types at runtime using type descriptors from the
// ssztypes package to perform SSZ operations without requiring code generation.
// This approach supports dynamic field sizes resolved through specification
// values, making it suitable for types whose SSZ layout varies across
// configurations (e.g., Ethereum mainnet vs. minimal presets).
package reflection

import (
	"reflect"

	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// float32Type is the plain float32 reflect.Type, used to read/write a float32
// field's raw bits without the float64 round-trip that reflect.Value.Float and
// SetFloat perform (that widening normalizes a signaling NaN's payload, diverging
// from the generated code which assigns the float32 directly).
var float32Type = reflect.TypeOf(float32(0))

// ReflectionCtx holds the configuration for reflection-based SSZ operations.
// It wraps a DynamicSpecs provider for resolving dynamic field sizes, along
// with options controlling fastssz fallback behavior and logging.
type ReflectionCtx struct {
	ds           sszutils.DynamicSpecs
	logCb        func(format string, args ...any)
	verbose      bool
	noFastSsz    bool
	noDelegation bool

	// maxDepth bounds how deeply a walk may nest.
	maxDepth int
}

// defaultMaxNestingDepth bounds how deeply a value may nest while being
// encoded, decoded or hashed.
//
// Only a recursive type can nest to a depth the input controls, and each level
// costs a handful of wire bytes, so without a bound a small payload can exhaust
// the goroutine stack. Go aborts the process on stack exhaustion and recover()
// cannot catch it, so the bound turns that abort into an ordinary error.
//
// 1024 is far deeper than any practical schema nests (Ethereum consensus types
// stay under 20) while costing well under a megabyte of stack.
const defaultMaxNestingDepth = 1024

// NewReflectionCtx creates a new ReflectionCtx with the given configuration.
//
// Parameters:
//   - ds: provides dynamic specification values for resolving field sizes
//   - logCb: callback for debug logging (may be nil)
//   - verbose: enables verbose logging output
//   - noFastSsz: when true, disables fastssz fallback for types that implement
//     fastssz interfaces, forcing all operations through reflection
//   - noDelegation: when true, disables delegation to a type's own generated
//     Dynamic* SSZ methods, forcing the generic reflection walk. Custom types
//     (which have no reflection representation) always delegate regardless.
//   - maxDepth: bounds how deeply a value may nest before the walk fails with
//     sszutils.ErrMaxDepthExceeded. A non-positive value selects
//     defaultMaxNestingDepth. In practice only a recursive type can reach it.
func NewReflectionCtx(ds sszutils.DynamicSpecs, logCb func(format string, args ...any), verbose, noFastSsz, noDelegation bool, maxDepth int) *ReflectionCtx {
	if maxDepth <= 0 {
		maxDepth = defaultMaxNestingDepth
	}

	return &ReflectionCtx{
		ds:           ds,
		logCb:        logCb,
		verbose:      verbose,
		noFastSsz:    noFastSsz,
		noDelegation: noDelegation,
		maxDepth:     maxDepth,
	}
}

// newZeroElem constructs the zero element used when a vector's Go value is
// shorter than its declared length. Pointer elements need an allocated
// (non-nil) value so the padding element is sized, serialized, and hashed as
// a present zero element by every engine pass; a nil pointer would be treated
// as an absent optional and give the passes different byte layouts.
func newZeroElem(fieldType *ssztypes.TypeDescriptor) reflect.Value {
	if fieldType.GoTypeFlags&ssztypes.GoTypeFlagIsPointer != 0 {
		return reflect.New(fieldType.Type.Elem())
	}

	return reflect.New(fieldType.Type).Elem()
}

func getPtr(v reflect.Value) reflect.Value {
	if v.Kind() == reflect.Ptr {
		return v
	}

	if v.CanAddr() {
		return v.Addr()
	}

	ptr := reflect.New(v.Type())
	ptr.Elem().Set(v)

	return ptr
}

// SizeSSZ computes the SSZ-encoded byte size of targetValue using its type
// descriptor.
func (ctx *ReflectionCtx) SizeSSZ(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value) (uint32, error) {
	if targetType == nil {
		return 0, sszutils.NewSszError(sszutils.ErrInvalidValueRange, "target type must not be nil")
	}
	return ctx.getSszValueSize(targetType, targetValue, 0)
}

// MarshalSSZ encodes targetValue into SSZ format using the provided encoder
// and type descriptor.
func (ctx *ReflectionCtx) MarshalSSZ(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, encoder sszutils.Encoder) error {
	if targetType == nil {
		return sszutils.NewSszError(sszutils.ErrInvalidValueRange, "target type must not be nil")
	}
	if encoder == nil {
		return sszutils.NewSszError(sszutils.ErrInvalidValueRange, "encoder must not be nil")
	}
	return ctx.marshalType(targetType, targetValue, encoder, 0)
}

// UnmarshalSSZ decodes SSZ data from the provided decoder into targetValue
// using the type descriptor.
func (ctx *ReflectionCtx) UnmarshalSSZ(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder) error {
	if targetType == nil {
		return sszutils.NewSszError(sszutils.ErrInvalidValueRange, "target type must not be nil")
	}
	if decoder == nil {
		return sszutils.NewSszError(sszutils.ErrInvalidValueRange, "decoder must not be nil")
	}
	return ctx.unmarshalType(targetType, targetValue, decoder, 0)
}

// HashTreeRoot computes the SSZ hash tree root of targetValue using the
// provided HashWalker and type descriptor.
func (ctx *ReflectionCtx) HashTreeRoot(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, hh sszutils.HashWalker) error {
	if targetType == nil {
		return sszutils.NewSszError(sszutils.ErrInvalidValueRange, "target type must not be nil")
	}
	if hh == nil {
		return sszutils.NewSszError(sszutils.ErrInvalidValueRange, "hash walker must not be nil")
	}
	return ctx.buildRootFromType(targetType, targetValue, hh, false, 0)
}
