// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"reflect"
)

// CompatibleUnion represents a union type that can hold one of several possible types.
// It uses Go generics where T is a descriptor struct that defines the union's possible types.
// The descriptor struct is never instantiated but provides type information through its fields.
//
// The union stores:
// - unionType: uint8 field index indicating which variant is active
// - data: interface{} holding the actual value
//
// Usage:
//
//	type UnionExecutionPayload = dynssz.CompatibleUnion[struct {
//	    ExecutionPayload
//	    ExecutionPayloadWithBlobs
//	}]
//
//	type BlockWithPayload struct {
//	    Slot          uint64
//	    ExecutionData UnionExecutionPayload
//	}
//
//	block := BlockWithPayload{
//	    Slot: 123,
//	    ExecutionData: UnionExecutionPayload{
//	        Variant: 0,
//	        Data: ExecutionPayload{
//	            ...
//	        },
//	    },
//	}
type CompatibleUnion[T any] struct {
	Variant uint8
	Data    interface{}
}

// NewCompatibleUnion creates a new CompatibleUnion with the specified variant type and data.
// The variantIndex corresponds to the field index in the descriptor struct T.
func NewCompatibleUnion[T any](variantIndex uint8, data interface{}) (*CompatibleUnion[T], error) {
	return &CompatibleUnion[T]{
		Variant: variantIndex,
		Data:    data,
	}, nil
}

// GetDescriptorType returns the reflect.Type of the descriptor struct T.
// This allows external code to access the descriptor type information.
func (u *CompatibleUnion[T]) GetDescriptorType() reflect.Type {
	var zero *T
	return reflect.TypeOf(zero).Elem()
}
