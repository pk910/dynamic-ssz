// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"reflect"

	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// CompatibleUnion represents a union type that can hold one of several possible types.
// It uses Go generics where T is a descriptor struct that defines the union's possible types.
// The descriptor struct is never instantiated but provides type information through its fields.
//
// EIP-8016 expects the variants to share a compatible merkleization: the same
// tree shape, with containers carrying the same field names in the same order,
// collections the same capacity, and only byte/uint8 differing among basics.
// That is a rule for designing the schema; the library does not verify it
// (a check may be added later). Diverging variants serialize and hash — the
// selector is mixed into the root — but no conforming implementation will
// accept the type.
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
//	        Variant: 1,
//	        Data: ExecutionPayload{
//	            ...
//	        },
//	    },
//	}
type CompatibleUnion[T any] struct {
	Variant uint8
	Data    interface{}
}

// NewCompatibleUnion creates a new CompatibleUnion with the specified variant selector and data.
// Selectors follow the descriptor struct's field order starting at 1, or the
// fields' ssz-index tags when present (valid range 1..127 per EIP-8016).
func NewCompatibleUnion[T any](variantIndex uint8, data interface{}) (*CompatibleUnion[T], error) {
	var zero *T
	normalized, err := validateUnionVariant(reflect.TypeOf(zero).Elem(), variantIndex, data, true)
	if err != nil {
		return nil, err
	}
	return &CompatibleUnion[T]{
		Variant: variantIndex,
		Data:    normalized,
	}, nil
}

// GetDescriptorType returns the reflect.Type of the descriptor struct T.
// This allows external code to access the descriptor type information.
func (u *CompatibleUnion[T]) GetDescriptorType() reflect.Type {
	var zero *T
	return reflect.TypeOf(zero).Elem()
}

// None marks the empty option of a classic SSZ Union. Declared as the FIRST
// field of a Union descriptor struct, it makes selector 0 the None variant:
//
//	type MaybePayload = dynssz.Union[struct {
//	    None dynssz.None // selector 0: no value
//	    Full ExecutionPayload
//	}]
//
// A None-valued union serializes as the single byte 0x00 and hashes as
// mix_in_selector(Bytes32(), 0). The SSZ spec only allows None as the first
// option, and a union declaring it must offer at least one other variant.
type None struct{}

// Union represents a classic SSZ union: an ordered set of variants addressed
// by 0-based positional selectors, following the consensus-spec Union rules.
// T is a descriptor struct whose fields define the variants in order; it is
// never instantiated and only provides type information. Selectors above 127
// are reserved by the spec and rejected.
//
// The union stores the active selector and the value. A union whose descriptor
// declares dynssz.None as its first field represents the empty option as
// {Variant: 0, Data: nil}.
//
// Variants carry no merkleization expectations (CompatibleUnion's EIP-8016
// compatibility rule does not apply here) and the selector is assigned by
// position alone (ssz-index tags are not allowed).
type Union[T any] struct {
	Variant uint8
	Data    interface{}
}

// NewUnion creates a new Union with the specified variant selector and data.
// Selectors are the 0-based positions of the descriptor struct's fields; when
// the descriptor declares dynssz.None first, selector 0 with nil data is the
// empty option.
func NewUnion[T any](variantIndex uint8, data interface{}) (*Union[T], error) {
	var zero *T
	normalized, err := validateUnionVariant(reflect.TypeOf(zero).Elem(), variantIndex, data, false)
	if err != nil {
		return nil, err
	}
	return &Union[T]{
		Variant: variantIndex,
		Data:    normalized,
	}, nil
}

// GetDescriptorType returns the reflect.Type of the descriptor struct T.
// This allows external code to access the descriptor type information.
func (u *Union[T]) GetDescriptorType() reflect.Type {
	var zero *T
	return reflect.TypeOf(zero).Elem()
}

// validateUnionVariant checks a selector against the descriptor struct and
// the data value against the selected variant's field type, and returns the
// data in the exact declared type: every engine matches the variant type
// exactly, so a pointer to the declared value type is dereferenced here
// rather than failing at the first marshal. For a compatible union the
// selector mapping is the shared UnionVariantTypes authority (field order
// from 1, or ssz-index tags); a classic union numbers by position from 0,
// with a leading None field keeping selector 0 as the empty option.
func validateUnionVariant(descriptorType reflect.Type, variantIndex uint8, data interface{}, compatible bool) (interface{}, error) {
	if descriptorType.Kind() != reflect.Struct || descriptorType.NumField() == 0 {
		return nil, sszutils.NewSszErrorf(sszutils.ErrTypeMismatch, "union descriptor must be a struct with at least one field, got %v", descriptorType)
	}

	noneType := reflect.TypeOf(None{})
	var fieldType reflect.Type
	if compatible {
		variantTypes, err := ssztypes.UnionVariantTypes(descriptorType)
		if err != nil {
			return nil, err
		}
		fieldType = variantTypes[variantIndex]
		if fieldType == nil {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidUnionVariant, "variant selector %d is not declared by %v", variantIndex, descriptorType)
		}
	} else {
		if variantIndex == 0 && descriptorType.Field(0).Type == noneType {
			if data != nil {
				return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidUnionVariant, "the None variant carries no data, got %T", data)
			}
			return nil, nil
		}
		if int(variantIndex) >= descriptorType.NumField() {
			return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidUnionVariant, "variant selector %d is not declared by %v", variantIndex, descriptorType)
		}
		fieldType = descriptorType.Field(int(variantIndex)).Type
	}

	if fieldType == noneType {
		return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidUnionVariant, "selector %d is the None variant and carries no data", variantIndex)
	}
	if data == nil {
		return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidUnionVariant, "variant %d (%v) needs a value, got nil", variantIndex, fieldType)
	}
	dataType := reflect.TypeOf(data)
	if dataType == fieldType {
		return data, nil
	}
	if dataType.Kind() == reflect.Pointer && dataType.Elem() == fieldType {
		return reflect.ValueOf(data).Elem().Interface(), nil
	}
	return nil, sszutils.NewSszErrorf(sszutils.ErrInvalidUnionVariant, "variant %d expects %v, got %T", variantIndex, fieldType, data)
}
