// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection

import (
	"reflect"
	"testing"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

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
	ctx := NewReflectionCtx(nil, nil, false, false)

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
