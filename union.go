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
//
// The values are stored verbatim: the selector, the descriptor shape and the
// data type are validated by the marshal, unmarshal, size and hash operations,
// not here. Data must be exactly the declared variant type (not a pointer to
// it). The returned error is always nil and remains for API stability.
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
//
// The values are stored verbatim: the selector, the descriptor shape and the
// data type are validated by the marshal, unmarshal, size and hash operations,
// not here. Data must be exactly the declared variant type (not a pointer to
// it). The returned error is always nil and remains for API stability.
func NewUnion[T any](variantIndex uint8, data interface{}) (*Union[T], error) {
	return &Union[T]{
		Variant: variantIndex,
		Data:    data,
	}, nil
}

// GetDescriptorType returns the reflect.Type of the descriptor struct T.
// This allows external code to access the descriptor type information.
func (u *Union[T]) GetDescriptorType() reflect.Type {
	var zero *T
	return reflect.TypeOf(zero).Elem()
}
