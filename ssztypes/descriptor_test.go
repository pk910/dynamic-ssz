// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"math/bits"
	"reflect"
	"testing"
)

// Pins the field grouping described on TypeDescriptor: a reorder keeps every
// other test passing and shows up only as a few percent on the reflection
// benchmarks.
func TestTypeDescriptorHotFieldsShareACacheLine(t *testing.T) {
	const cacheLine = 64

	// Read for every walked value, whatever its kind.
	universal := []string{"SszCompatFlags", "SszTypeFlags", "SszType", "GoTypeFlags"}
	// Read for every composite value, which is every interior node of a walk.
	composite := []string{"ElemDesc", "ContainerDesc", "Size", "Len", "Limit", "Type"}

	td := reflect.TypeOf(TypeDescriptor{})
	offset := func(name string) uintptr {
		f, ok := td.FieldByName(name)
		if !ok {
			t.Fatalf("TypeDescriptor has no field %q", name)
		}
		return f.Offset
	}

	for _, name := range universal {
		if got := offset(name); got >= cacheLine {
			t.Errorf("%s is at offset %d, outside the first cache line: the fields read for every "+
				"value must stay at the front of the struct", name, got)
		}
	}

	// Asserted on 64-bit, where the ten fields fill the line exactly; a 32-bit
	// build packs them into well under it, so the bound says nothing there.
	if bits.UintSize == 64 {
		for _, name := range composite {
			f, _ := td.FieldByName(name)
			if end := f.Offset + f.Type.Size(); end > cacheLine {
				t.Errorf("%s ends at %d, past the first cache line", name, end)
			}
		}
	}

	// Nothing the reflection engine never reads may sit in front of them.
	for _, name := range []string{"SchemaType", "SizeExpression", "MaxExpression"} {
		if got := offset(name); got < cacheLine {
			t.Errorf("%s is at offset %d: the reflection engine never reads it, so it must not "+
				"occupy the first cache line", name, got)
		}
	}
}

func TestRootIsChunk(t *testing.T) {
	basic := &TypeDescriptor{SszType: SszUint64Type, Size: 8}
	container := &TypeDescriptor{SszType: SszContainerType, Size: 16}

	tests := []struct {
		name string
		desc *TypeDescriptor
		want bool
	}{
		{"basic value", basic, true},
		{"uint256", &TypeDescriptor{SszType: SszUint256Type, Size: 32}, true},
		{"wrapped basic value", &TypeDescriptor{SszType: SszTypeWrapperType, ElemDesc: basic}, true},
		{"vector of basic values in one chunk", &TypeDescriptor{SszType: SszVectorType, Size: 32, ElemDesc: basic}, true},
		{"vector of basic values past one chunk", &TypeDescriptor{SszType: SszVectorType, Size: 40, ElemDesc: basic}, false},
		{"vector of containers", &TypeDescriptor{SszType: SszVectorType, Size: 16, ElemDesc: container}, false},
		{"vector without element", &TypeDescriptor{SszType: SszVectorType, Size: 16}, false},
		{"bitvector in one chunk", &TypeDescriptor{SszType: SszBitvectorType, Size: 32}, true},
		{"bitvector past one chunk", &TypeDescriptor{SszType: SszBitvectorType, Size: 33}, false},
		{"vector sized by an expression", &TypeDescriptor{SszType: SszVectorType, Size: 16, ElemDesc: basic, SszTypeFlags: SszTypeFlagHasSizeExpr}, false},
		{"custom type", &TypeDescriptor{SszType: SszCustomType, Size: 8}, false},
		{"small container", container, false},
		{"list", &TypeDescriptor{SszType: SszListType, SszTypeFlags: SszTypeFlagIsDynamic, ElemDesc: basic}, false},
	}
	for _, tt := range tests {
		if got := tt.desc.RootIsChunk(); got != tt.want {
			t.Errorf("%s: RootIsChunk = %v, want %v", tt.name, got, tt.want)
		}
	}
}
