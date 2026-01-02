// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"reflect"
)

// TypeWrapper represents a wrapper type that can provide SSZ annotations for non-struct types.
// It uses Go generics where D is a WrapperDescriptor struct that must have exactly 1 field,
// and T is the actual value type. The descriptor struct is never instantiated but provides
// type information with annotations.
//
// The wrapper stores:
// - data: the actual value of type T
//
// Usage:
//
//	type ByteSliceDescriptor struct {
//	    Data []byte `ssz-size:"32"`
//	}
//	type WrappedByteSlice = dynssz.TypeWrapper[ByteSliceDescriptor, []byte]
//
//	// Use in a struct or standalone
//	wrapped := WrappedByteSlice{}
//	wrapped.Set([]byte{1, 2, 3, 4})
//	data := wrapped.Get() // returns []byte
type TypeWrapper[D, T any] struct {
	Data T
}

// NewTypeWrapper creates a new TypeWrapper with the specified data.
func NewTypeWrapper[D, T any](data T) (*TypeWrapper[D, T], error) {
	return &TypeWrapper[D, T]{
		Data: data,
	}, nil
}

// Get returns the wrapped value.
func (w *TypeWrapper[D, T]) Get() T {
	return w.Data
}

// Set sets the wrapped value.
func (w *TypeWrapper[D, T]) Set(value T) {
	w.Data = value
}

// GetDescriptorType returns the reflect.Type of the descriptor struct D.
// This allows external code to access the descriptor type information.
func (w *TypeWrapper[D, T]) GetDescriptorType() reflect.Type {
	var zero *D
	return reflect.TypeOf(zero).Elem()
}
