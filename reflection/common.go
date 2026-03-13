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
	"math"
	"reflect"

	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// platformMaxInt is the maximum int value for the current platform.
// It is a variable (not constant) to allow testing overflow checks on 64-bit systems.
var platformMaxInt = int64(math.MaxInt)

// ReflectionCtx holds the configuration for reflection-based SSZ operations.
// It wraps a DynamicSpecs provider for resolving dynamic field sizes, along
// with options controlling fastssz fallback behavior and logging.
type ReflectionCtx struct {
	ds        sszutils.DynamicSpecs
	logCb     func(format string, args ...any)
	verbose   bool
	noFastSsz bool
}

// NewReflectionCtx creates a new ReflectionCtx with the given configuration.
//
// Parameters:
//   - ds: provides dynamic specification values for resolving field sizes
//   - logCb: callback for debug logging (may be nil)
//   - verbose: enables verbose logging output
//   - noFastSsz: when true, disables fastssz fallback for types that implement
//     fastssz interfaces, forcing all operations through reflection
func NewReflectionCtx(ds sszutils.DynamicSpecs, logCb func(format string, args ...any), verbose, noFastSsz bool) *ReflectionCtx {
	return &ReflectionCtx{
		ds:        ds,
		logCb:     logCb,
		verbose:   verbose,
		noFastSsz: noFastSsz,
	}
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
	return ctx.getSszValueSize(targetType, targetValue)
}

// MarshalSSZ encodes targetValue into SSZ format using the provided encoder
// and type descriptor.
func (ctx *ReflectionCtx) MarshalSSZ(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, encoder sszutils.Encoder) error {
	return ctx.marshalType(targetType, targetValue, encoder, 0)
}

// UnmarshalSSZ decodes SSZ data from the provided decoder into targetValue
// using the type descriptor.
func (ctx *ReflectionCtx) UnmarshalSSZ(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, decoder sszutils.Decoder) error {
	return ctx.unmarshalType(targetType, targetValue, decoder, 0)
}

// HashTreeRoot computes the SSZ hash tree root of targetValue using the
// provided HashWalker and type descriptor.
func (ctx *ReflectionCtx) HashTreeRoot(targetType *ssztypes.TypeDescriptor, targetValue reflect.Value, hh sszutils.HashWalker) error {
	return ctx.buildRootFromType(targetType, targetValue, hh, false, 0)
}
