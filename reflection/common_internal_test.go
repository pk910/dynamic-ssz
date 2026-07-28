// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection

import (
	"errors"
	"reflect"
	"testing"

	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// The public marshal entry points size a value before encoding it, so
// marshalUnion's own validation errors are shadowed by the equivalent checks in
// the sizing pass. These tests drive marshalUnion directly to pin its guards.
func TestMarshalUnionValidationErrors(t *testing.T) {
	type unionValue struct {
		Variant uint8
		Data    interface{}
	}

	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType:      ssztypes.SszUnionType,
		Kind:         reflect.Struct,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic | ssztypes.SszTypeFlagHasNoneVariant,
		UnionVariants: map[uint8]*ssztypes.TypeDescriptor{
			1: {
				SszType: ssztypes.SszUint32Type,
				Kind:    reflect.Uint32,
				Type:    reflect.TypeOf(uint32(0)),
				Size:    4,
			},
		},
	}

	tests := []struct {
		name    string
		value   unionValue
		wantErr error
	}{
		{"noneWithData", unionValue{Variant: 0, Data: uint32(1)}, sszutils.ErrInvalidValueRange},
		{"invalidVariant", unionValue{Variant: 9, Data: uint32(1)}, sszutils.ErrInvalidValueRange},
		{"nilData", unionValue{Variant: 1, Data: nil}, sszutils.ErrInvalidValueRange},
		{"typeMismatch", unionValue{Variant: 1, Data: "not a uint32"}, sszutils.ErrInvalidValueRange},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc := sszutils.NewBufferEncoder(nil)
			err := ctx.marshalUnion(td, reflect.ValueOf(tt.value), enc, 0)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestGetPtrWithPointerValue(t *testing.T) {
	x := 42
	ptrVal := reflect.ValueOf(&x)
	result := getPtr(ptrVal)
	if result.Kind() != reflect.Ptr {
		t.Fatalf("expected Ptr, got %v", result.Kind())
	}
	if result.Elem().Int() != 42 {
		t.Fatalf("expected 42, got %v", result.Elem().Int())
	}
	if result.Pointer() != ptrVal.Pointer() {
		t.Fatal("expected same pointer to be returned")
	}
}

// The exported ReflectionCtx methods must reject nil descriptors and nil
// encoders/decoders/walkers with a clean error instead of panicking.
func TestReflectionCtxNilArgs(t *testing.T) {
	ctx := NewReflectionCtx(nil, nil, false, false, false)

	if _, err := ctx.SizeSSZ(nil, reflect.Value{}); err == nil {
		t.Error("SizeSSZ: expected error for nil target type")
	}
	if err := ctx.MarshalSSZ(nil, reflect.Value{}, nil); err == nil {
		t.Error("MarshalSSZ: expected error for nil target type")
	}
	if err := ctx.UnmarshalSSZ(nil, reflect.Value{}, nil); err == nil {
		t.Error("UnmarshalSSZ: expected error for nil target type")
	}
	if err := ctx.HashTreeRoot(nil, reflect.Value{}, nil); err == nil {
		t.Error("HashTreeRoot: expected error for nil target type")
	}

	tc := ssztypes.NewTypeCache(nil)
	desc, err := tc.GetTypeDescriptor(reflect.TypeOf(uint64(0)), nil, nil, nil)
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	val := reflect.ValueOf(uint64(1))
	if err := ctx.MarshalSSZ(desc, val, nil); err == nil {
		t.Error("MarshalSSZ: expected error for nil encoder")
	}
	if err := ctx.UnmarshalSSZ(desc, val, nil); err == nil {
		t.Error("UnmarshalSSZ: expected error for nil decoder")
	}
	if err := ctx.HashTreeRoot(desc, val, nil); err == nil {
		t.Error("HashTreeRoot: expected error for nil walker")
	}
}

// A dynamic-element vector longer than its declared length is rejected by
// marshalDynamicVector. The public marshal path sizes the value first (which
// now also rejects over-length vectors), so this guard is only reachable — and
// pinned — by driving the marshal directly.
func TestMarshalDynamicVectorOverLength(t *testing.T) {
	type dynElem struct {
		D []byte `ssz-max:"8"`
	}
	type container struct {
		V []dynElem `ssz-size:"2"`
	}
	tc := ssztypes.NewTypeCache(nil)
	desc, err := tc.GetTypeDescriptor(reflect.TypeOf(container{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	vecDesc := desc.ContainerDesc.Fields[0].Type

	ctx := newCtx()
	enc := sszutils.NewBufferEncoder(nil)
	over := reflect.ValueOf([]dynElem{{}, {}, {}}) // 3 > declared length 2
	if err := ctx.marshalDynamicVector(vecDesc, over, enc, 0); !errors.Is(err, sszutils.ErrVectorLength) {
		t.Fatalf("expected ErrVectorLength, got %v", err)
	}
}

// dynamicListPreallocation's degenerate inputs are unreachable through a decode:
// unmarshalDynamicList rejects a zero first offset before deriving the element
// count, so the count is always positive, and no SSZ-dynamic element has a
// zero-size Go representation. Both guards still matter — a negative count would
// panic reflect.MakeSlice — and the policy must stay identical to
// sszutils.decodeSlicePreallocation, which the generated decoders use for the
// same lists, so drive it directly.
func TestDynamicListPreallocation(t *testing.T) {
	const budgetBytes = 64 << 10

	tests := []struct {
		name     string
		count    int
		elemSize uint64
		want     int
	}{
		{name: "negative count", count: -1, elemSize: 8, want: 0},
		{name: "empty", count: 0, elemSize: 8, want: 0},
		{name: "zero sized element", count: 100_000, elemSize: 0, want: 100_000},
		{name: "within budget", count: 32, elemSize: 8, want: 32},
		{name: "clamped to byte budget", count: 10_000, elemSize: 8, want: budgetBytes / 8},
		{name: "one element exceeds budget", count: 100, elemSize: budgetBytes + 1, want: 1},
		{name: "large size cannot overflow", count: 100, elemSize: ^uint64(0), want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := dynamicListPreallocation(test.count, test.elemSize); got != test.want {
				t.Fatalf(
					"dynamicListPreallocation(%d, %d) = %d, want %d",
					test.count,
					test.elemSize,
					got,
					test.want,
				)
			}
		})
	}
}
