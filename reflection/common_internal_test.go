// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection

import (
	"reflect"
	"testing"
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
