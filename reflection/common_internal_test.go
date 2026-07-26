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
