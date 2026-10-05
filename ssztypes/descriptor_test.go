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
