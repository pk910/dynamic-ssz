// dynssz: Dynamic SSZ encoding/decoding for Ethereum with fastssz efficiency.
// This file is part of the dynssz package.
// Copyright (c) 2024 by pk910. Refer to LICENSE for more information.
package dynssz

import (
	"fmt"
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

// UnionVariantInfo contains type and annotation information for a union variant
type UnionVariantInfo struct {
	Type         reflect.Type
	SizeHints    []SszSizeHint
	MaxSizeHints []SszMaxSizeHint
	TypeHints    []SszTypeHint
}

// ExtractUnionDescriptorInfo extracts variant information from a union descriptor type.
// This function is used by the type cache to extract variant information including SSZ annotations.
func ExtractUnionDescriptorInfo(descriptorType reflect.Type, dynssz *DynSsz) (map[uint8]UnionVariantInfo, error) {
	if descriptorType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("union descriptor must be a struct, got %v", descriptorType.Kind())
	}

	variantInfo := make(map[uint8]UnionVariantInfo)

	for i := 0; i < descriptorType.NumField(); i++ {
		field := descriptorType.Field(i)
		variantIndex := uint8(i) // Field order determines variant index

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

		variantInfo[variantIndex] = UnionVariantInfo{
			Type:         field.Type,
			SizeHints:    sizeHints,
			MaxSizeHints: maxSizeHints,
			TypeHints:    typeHints,
		}
	}

	if len(variantInfo) == 0 {
		return nil, fmt.Errorf("union descriptor struct has no fields")
	}

	return variantInfo, nil
}
