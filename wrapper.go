// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
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

// WrapperDescriptorInfo contains type and annotation information for a wrapper
type WrapperDescriptorInfo struct {
	Type         reflect.Type
	SizeHints    []SszSizeHint
	MaxSizeHints []SszMaxSizeHint
	TypeHints    []SszTypeHint
}

// ExtractWrapperDescriptorInfo extracts wrapper information from a wrapper descriptor type.
// This function validates that the descriptor has exactly one field and extracts its annotations.
func ExtractWrapperDescriptorInfo(descriptorType reflect.Type, dynssz *DynSsz) (*WrapperDescriptorInfo, error) {
	if descriptorType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("wrapper descriptor must be a struct, got %v", descriptorType.Kind())
	}

	if descriptorType.NumField() != 1 {
		return nil, fmt.Errorf("wrapper descriptor must have exactly 1 field, got %d", descriptorType.NumField())
	}

	field := descriptorType.Field(0)

	// Extract SSZ annotations using existing DynSsz methods
	sizeHints, err := dynssz.getSszSizeTag(&field)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ssz-size tag for field %s: %w", field.Name, err)
	}

	maxSizeHints, err := dynssz.getSszMaxSizeTag(&field)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ssz-max tag for field %s: %w", field.Name, err)
	}

	typeHints, err := dynssz.getSszTypeTag(&field)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ssz-type tag for field %s: %w", field.Name, err)
	}

	return &WrapperDescriptorInfo{
		Type:         field.Type,
		SizeHints:    sizeHints,
		MaxSizeHints: maxSizeHints,
		TypeHints:    typeHints,
	}, nil
}
