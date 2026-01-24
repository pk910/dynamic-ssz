// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unsafe"
)

type dummyDynamicSpecs struct {
	specValues map[string]uint64
}

func (d *dummyDynamicSpecs) ResolveSpecValue(name string) (bool, uint64, error) {
	value, ok := d.specValues[name]
	return ok, value, nil
}

// Test error cases in TypeCache.GetTypeDescriptor
func TestTypeCache_ErrorCases(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	t.Run("UnsupportedTypes", func(t *testing.T) {
		unsupportedTypes := []struct {
			name     string
			typ      reflect.Type
			expected string
		}{
			{"int", reflect.TypeOf(int(0)), "signed integers are not supported"},
			{"int8", reflect.TypeOf(int8(0)), "signed integers are not supported"},
			{"int16", reflect.TypeOf(int16(0)), "signed integers are not supported"},
			{"int32", reflect.TypeOf(int32(0)), "signed integers are not supported"},
			{"int64", reflect.TypeOf(int64(0)), "signed integers are not supported"},
			{"float32", reflect.TypeOf(float32(0)), "floating-point numbers are not supported"},
			{"float64", reflect.TypeOf(float64(0)), "floating-point numbers are not supported"},
			{"complex64", reflect.TypeOf(complex64(0)), "complex numbers are not supported"},
			{"complex128", reflect.TypeOf(complex128(0)), "complex numbers are not supported"},
			{"map", reflect.TypeOf(map[string]int{}), "maps are not supported"},
			{"chan", reflect.TypeOf(make(chan int)), "channels are not supported"},
			{"func", reflect.TypeOf(func() {}), "functions are not supported"},
			{"interface", reflect.TypeOf((*interface{})(nil)).Elem(), "interfaces are not supported"},
			{"unsafe", reflect.TypeOf((*unsafe.Pointer)(nil)).Elem(), "unsafe pointers are not supported"},
			{"pointer", reflect.TypeOf((***uint64)(nil)).Elem(), "unsupported type kind: ptr"},
		}

		for _, tt := range unsupportedTypes {
			t.Run(tt.name, func(t *testing.T) {
				_, err := cache.GetTypeDescriptor(tt.typ, nil, nil, nil)
				if err == nil {
					t.Errorf("Expected error for type %s", tt.name)
					return
				}
				if !strings.Contains(err.Error(), tt.expected) {
					t.Errorf("Expected error containing '%s', got: %s", tt.expected, err.Error())
				}
			})
		}
	})

	t.Run("InvalidSizeHints", func(t *testing.T) {
		tests := []struct {
			name      string
			typ       reflect.Type
			hints     []SszSizeHint
			typeHints []SszTypeHint
			expected  string
		}{
			{
				name:     "bool with wrong size",
				typ:      reflect.TypeOf(bool(false)),
				hints:    []SszSizeHint{{Size: 2}},
				expected: "bool ssz type must be ssz-size:1",
			},
			{
				name:     "bool with bit size",
				typ:      reflect.TypeOf(bool(false)),
				hints:    []SszSizeHint{{Bits: true}},
				expected: "bool ssz type cannot be limited by bits, use regular size tag instead",
			},
			{
				name:     "uint8 with wrong size",
				typ:      reflect.TypeOf(uint8(0)),
				hints:    []SszSizeHint{{Size: 2}},
				expected: "uint8 ssz type must be ssz-size:1",
			},
			{
				name:     "uint8 with bit size",
				typ:      reflect.TypeOf(uint8(0)),
				hints:    []SszSizeHint{{Bits: true}},
				expected: "uint8 ssz type cannot be limited by bits, use regular size tag instead",
			},
			{
				name:     "uint16 with wrong size",
				typ:      reflect.TypeOf(uint16(0)),
				hints:    []SszSizeHint{{Size: 4}},
				expected: "uint16 ssz type must be ssz-size:2",
			},
			{
				name:     "uint16 with bit size",
				typ:      reflect.TypeOf(uint16(0)),
				hints:    []SszSizeHint{{Bits: true}},
				expected: "uint16 ssz type cannot be limited by bits, use regular size tag instead",
			},
			{
				name:     "uint32 with wrong size",
				typ:      reflect.TypeOf(uint32(0)),
				hints:    []SszSizeHint{{Size: 8}},
				expected: "uint32 ssz type must be ssz-size:4",
			},
			{
				name:     "uint32 with bit size",
				typ:      reflect.TypeOf(uint32(0)),
				hints:    []SszSizeHint{{Bits: true}},
				expected: "uint32 ssz type cannot be limited by bits, use regular size tag instead",
			},
			{
				name:     "uint64 with wrong size",
				typ:      reflect.TypeOf(uint64(0)),
				hints:    []SszSizeHint{{Size: 4}},
				expected: "uint64 ssz type must be ssz-size:8",
			},
			{
				name:     "uint64 with bit size",
				typ:      reflect.TypeOf(uint64(0)),
				hints:    []SszSizeHint{{Bits: true}},
				expected: "uint64 ssz type cannot be limited by bits, use regular size tag instead",
			},
			{
				name:      "uint128 with bit size",
				typ:       reflect.TypeOf([16]uint8{}),
				hints:     []SszSizeHint{{Bits: true}},
				typeHints: []SszTypeHint{{Type: SszUint128Type}},
				expected:  "uint128 ssz type cannot be limited by bits, use regular size tag instead",
			},
			{
				name:      "uint256 with bit size",
				typ:       reflect.TypeOf([32]uint8{}),
				hints:     []SszSizeHint{{Bits: true}},
				typeHints: []SszTypeHint{{Type: SszUint256Type}},
				expected:  "uint256 ssz type cannot be limited by bits, use regular size tag instead",
			},
			{
				name:     "other non bitvector type with bit size",
				typ:      reflect.TypeOf([16]uint8{}),
				hints:    []SszSizeHint{{Bits: true}},
				expected: "bit size tag is only allowed for bitvector or bitlist types",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := cache.GetTypeDescriptor(tt.typ, tt.hints, nil, tt.typeHints)
				if err == nil {
					t.Errorf("Expected error for %s", tt.name)
					return
				}
				if !strings.Contains(err.Error(), tt.expected) {
					t.Errorf("Expected error containing '%s', got: %s", tt.expected, err.Error())
				}
			})
		}
	})

	t.Run("InvalidTypeHints", func(t *testing.T) {
		tests := []struct {
			name     string
			typ      reflect.Type
			hints    []SszTypeHint
			expected string
		}{
			{
				name:     "bool with uint8 type hint",
				typ:      reflect.TypeOf(bool(false)),
				hints:    []SszTypeHint{{Type: SszUint8Type}},
				expected: "uint8 ssz type can only be represented by uint8 types",
			},
			{
				name:     "uint8 with bool type hint",
				typ:      reflect.TypeOf(uint8(0)),
				hints:    []SszTypeHint{{Type: SszBoolType}},
				expected: "bool ssz type can only be represented by bool types",
			},
			{
				name:     "uint16 with uint8 type hint",
				typ:      reflect.TypeOf(uint8(0)),
				hints:    []SszTypeHint{{Type: SszUint16Type}},
				expected: "uint16 ssz type can only be represented by uint16 types",
			},
			{
				name:     "uint32 with uint8 type hint",
				typ:      reflect.TypeOf(uint8(0)),
				hints:    []SszTypeHint{{Type: SszUint32Type}},
				expected: "uint32 ssz type can only be represented by uint32 types",
			},
			{
				name:     "string with uint64 type hint",
				typ:      reflect.TypeOf(""),
				hints:    []SszTypeHint{{Type: SszUint64Type}},
				expected: "uint64 ssz type can only be represented by uint64 or time.Time types",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := cache.GetTypeDescriptor(tt.typ, nil, nil, tt.hints)
				if err == nil {
					t.Errorf("Expected error for %s", tt.name)
					return
				}
				if !strings.Contains(err.Error(), tt.expected) {
					t.Errorf("Expected error containing '%s', got: %s", tt.expected, err.Error())
				}
			})
		}
	})

	t.Run("Uint128Errors", func(t *testing.T) {
		tests := []struct {
			name     string
			typ      reflect.Type
			hints    []SszTypeHint
			expected string
		}{
			{
				name:     "uint128 with wrong base type",
				typ:      reflect.TypeOf(uint32(0)),
				hints:    []SszTypeHint{{Type: SszUint128Type}},
				expected: "uint128 ssz type can only be represented by slice or array types",
			},
			{
				name:     "uint128 with wrong element type",
				typ:      reflect.TypeOf([]uint32{}),
				hints:    []SszTypeHint{{Type: SszUint128Type}},
				expected: "uint128 ssz type can only be represented by slices or arrays of uint8 or uint64",
			},
			{
				name:     "uint128 array too small",
				typ:      reflect.TypeOf([2]uint8{}),
				hints:    []SszTypeHint{{Type: SszUint128Type}},
				expected: "uint128 ssz type does not fit in array",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := cache.GetTypeDescriptor(tt.typ, nil, nil, tt.hints)
				if err == nil {
					t.Errorf("Expected error for %s", tt.name)
					return
				}
				if !strings.Contains(err.Error(), tt.expected) {
					t.Errorf("Expected error containing '%s', got: %s", tt.expected, err.Error())
				}
			})
		}
	})

	t.Run("Uint256Errors", func(t *testing.T) {
		tests := []struct {
			name     string
			typ      reflect.Type
			hints    []SszTypeHint
			expected string
		}{
			{
				name:     "uint256 with wrong base type",
				typ:      reflect.TypeOf(uint32(0)),
				hints:    []SszTypeHint{{Type: SszUint256Type}},
				expected: "uint256 ssz type can only be represented by slice or array types",
			},
			{
				name:     "uint256 with wrong element type",
				typ:      reflect.TypeOf([]uint32{}),
				hints:    []SszTypeHint{{Type: SszUint256Type}},
				expected: "uint256 ssz type can only be represented by slices or arrays of uint8 or uint64",
			},
			{
				name:     "uint256 array too small",
				typ:      reflect.TypeOf([4]uint8{}),
				hints:    []SszTypeHint{{Type: SszUint256Type}},
				expected: "uint256 ssz type does not fit in array",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := cache.GetTypeDescriptor(tt.typ, nil, nil, tt.hints)
				if err == nil {
					t.Errorf("Expected error for %s", tt.name)
					return
				}
				if !strings.Contains(err.Error(), tt.expected) {
					t.Errorf("Expected error containing '%s', got: %s", tt.expected, err.Error())
				}
			})
		}
	})

	t.Run("ContainerErrors", func(t *testing.T) {
		// Test container with wrong base type
		t.Run("wrong base type", func(t *testing.T) {
			_, err := cache.GetTypeDescriptor(reflect.TypeOf(uint32(0)), nil, nil, []SszTypeHint{{Type: SszContainerType}})
			if err == nil {
				t.Error("Expected error for container with wrong base type")
				return
			}
			if !strings.Contains(err.Error(), "container ssz type can only be represented by struct types") {
				t.Errorf("Unexpected error: %s", err.Error())
			}
		})
	})

	t.Run("VectorErrors", func(t *testing.T) {
		tests := []struct {
			name     string
			typ      reflect.Type
			sizeHint []SszSizeHint
			typeHint []SszTypeHint
			expected string
		}{
			{
				name:     "vector with wrong base type",
				typ:      reflect.TypeOf(uint32(0)),
				typeHint: []SszTypeHint{{Type: SszVectorType}},
				expected: "vector ssz type can only be represented by array or slice types",
			},
			{
				name:     "vector slice without size hint",
				typ:      reflect.TypeOf([]uint8{}),
				typeHint: []SszTypeHint{{Type: SszVectorType}},
				expected: "missing size hint for vector type",
			},
			{
				name:     "vector array size hint too large",
				typ:      reflect.TypeOf([2]uint8{}),
				sizeHint: []SszSizeHint{{Size: 5}},
				typeHint: []SszTypeHint{{Type: SszVectorType}},
				expected: "size hint for vector type is greater than the length of the array",
			},
			{
				name:     "bitvector with wrong element type",
				typ:      reflect.TypeOf([]uint32{}),
				typeHint: []SszTypeHint{{Type: SszBitvectorType}},
				sizeHint: []SszSizeHint{{Size: 4}},
				expected: "bitvector ssz type can only be represented by byte slices or arrays",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := cache.GetTypeDescriptor(tt.typ, tt.sizeHint, nil, tt.typeHint)
				if err == nil {
					t.Errorf("Expected error for %s", tt.name)
					return
				}
				if !strings.Contains(err.Error(), tt.expected) {
					t.Errorf("Expected error containing '%s', got: %s", tt.expected, err.Error())
				}
			})
		}
	})

	t.Run("ListErrors", func(t *testing.T) {
		tests := []struct {
			name     string
			typ      reflect.Type
			typeHint []SszTypeHint
			expected string
		}{
			{
				name:     "list with wrong base type",
				typ:      reflect.TypeOf(uint32(0)),
				typeHint: []SszTypeHint{{Type: SszListType}},
				expected: "list ssz type can only be represented by slice types",
			},
			{
				name:     "bitlist with wrong element type",
				typ:      reflect.TypeOf([]uint32{}),
				typeHint: []SszTypeHint{{Type: SszBitlistType}},
				expected: "bitlist ssz type can only be represented by byte slices",
			},
			{
				name:     "bitlist with string type",
				typ:      reflect.TypeOf(""),
				typeHint: []SszTypeHint{{Type: SszBitlistType}},
				expected: "bitlist ssz type can only be represented by byte slices, got string",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := cache.GetTypeDescriptor(tt.typ, nil, nil, tt.typeHint)
				if err == nil {
					t.Errorf("Expected error for %s", tt.name)
					return
				}
				if !strings.Contains(err.Error(), tt.expected) {
					t.Errorf("Expected error containing '%s', got: %s", tt.expected, err.Error())
				}
			})
		}
	})

	t.Run("CustomTypeErrors", func(t *testing.T) {
		// Test custom type without required interfaces
		type CustomType struct{}

		_, err := cache.GetTypeDescriptor(reflect.TypeOf(CustomType{}), nil, nil, []SszTypeHint{{Type: SszCustomType}})
		if err == nil {
			t.Error("Expected error for custom type without interfaces")
			return
		}
		if !strings.Contains(err.Error(), "custom ssz type requires fastssz marshaler") {
			t.Errorf("Unexpected error: %s", err.Error())
		}
	})
}

// Test progressive container error cases
func TestTypeCache_ProgressiveContainerErrors(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	t.Run("MixedIndexTags", func(t *testing.T) {
		// This would need to be tested with actual struct types that have mixed ssz-index tags
		// Since we can't easily create such types at runtime, we'll test the validation logic
		// through the existing error paths in buildContainerDescriptor

		// For now, we'll create a simple test that exercises the progressive container detection
		type TestStruct struct {
			Field1 uint32
			Field2 uint32
		}

		// This should work fine (no index tags)
		_, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
		if err != nil {
			t.Errorf("Unexpected error for normal struct: %s", err.Error())
		}
	})

	t.Run("DuplicateIndexTags", func(t *testing.T) {
		type TestStruct struct {
			Field1 uint32 `ssz-index:"0"`
			Field2 uint32 `ssz-index:"0"`
		}

		_, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
		if err == nil {
			t.Error("Expected error for duplicate ssz-index")
			return
		}
		if !strings.Contains(err.Error(), "duplicate ssz-index 0 found in field Field2") {
			t.Errorf("Unexpected error: %s", err.Error())
		}
	})

	t.Run("InvalidIndexTag", func(t *testing.T) {
		type TestStruct struct {
			Field1 uint32 `ssz-index:"invalid"`
			Field2 uint32 `ssz-index:"1"`
		}

		// This should work fine (no index tags)
		_, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
		if err == nil {
			t.Error("Expected error for invalid ssz-index")
			return
		}
		if !strings.Contains(err.Error(), "parsing \"invalid\": invalid syntax") {
			t.Errorf("Unexpected error: %s", err.Error())
		}
	})

	t.Run("MissingIndexTag", func(t *testing.T) {
		type TestStruct struct {
			Field1 uint32 `ssz-index:"0"`
			Field2 uint32 // mising index tag
		}

		// This should work fine (no index tags)
		_, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
		if err == nil {
			t.Error("Expected error for missing ssz-index")
			return
		}
		if !strings.Contains(err.Error(), "progressive container field Field2 missing ssz-index tag") {
			t.Errorf("Unexpected error: %s", err.Error())
		}
	})

	t.Run("DecreasingIndexTag", func(t *testing.T) {
		type TestStruct struct {
			Field1 uint32 `ssz-index:"3"`
			Field2 uint32 `ssz-index:"1"`
		}

		// This should work fine (no index tags)
		_, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
		if err == nil {
			t.Error("Expected error for decreasing ssz-index")
			return
		}
		if !strings.Contains(err.Error(), "progressive container requires increasing ssz-index values") {
			t.Errorf("Unexpected error: %s", err.Error())
		}
	})
}

// Test TypeWrapper error cases
func TestTypeCache_TypeWrapperErrors(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	t.Run("TypeWrapperWithWrongBaseType", func(t *testing.T) {
		_, err := cache.GetTypeDescriptor(reflect.TypeOf(uint32(0)), nil, nil, []SszTypeHint{{Type: SszTypeWrapperType}})
		if err == nil {
			t.Error("Expected error for TypeWrapper with wrong base type")
			return
		}
		if !strings.Contains(err.Error(), "TypeWrapper ssz type can only be represented by struct types") {
			t.Errorf("Unexpected error: %s", err.Error())
		}
	})
}

// Test cache management functions
func TestTypeCache_CacheManagement(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Add some types to cache
	testTypes := []reflect.Type{
		reflect.TypeOf(uint32(0)),
		reflect.TypeOf(bool(false)),
		reflect.TypeOf(""),
	}

	for _, typ := range testTypes {
		_, err := cache.GetTypeDescriptor(typ, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to cache type %s: %v", typ, err)
		}
	}

	t.Run("GetAllTypes", func(t *testing.T) {
		allTypes := cache.GetAllTypes()
		if len(allTypes) < len(testTypes) {
			t.Errorf("Expected at least %d types, got %d", len(testTypes), len(allTypes))
		}
	})

	t.Run("RemoveType", func(t *testing.T) {
		typeToRemove := reflect.TypeOf(uint32(0))
		cache.RemoveType(typeToRemove)

		// Verify it's removed by checking if it gets recomputed
		_, err := cache.GetTypeDescriptor(typeToRemove, nil, nil, nil)
		if err != nil {
			t.Errorf("Failed to recompute removed type: %v", err)
		}
	})

	t.Run("RemoveTypePointer", func(t *testing.T) {
		// Test removing pointer type
		ptrType := reflect.TypeOf((*uint32)(nil))
		cache.RemoveType(ptrType)

		_, err := cache.GetTypeDescriptor(ptrType, nil, nil, nil)
		if err != nil {
			t.Errorf("Failed to recompute removed pointer type: %v", err)
		}
	})

	t.Run("RemoveAllTypes", func(t *testing.T) {
		cache.RemoveAllTypes()

		allTypes := cache.GetAllTypes()
		if len(allTypes) != 0 {
			t.Errorf("Expected 0 types after RemoveAllTypes, got %d", len(allTypes))
		}
	})
}

// Test TypeDescriptor.GetTypeHash
func TestTypeDescriptor_GetTypeHash(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(uint32(0)), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get type descriptor: %v", err)
	}

	hash1, err := desc.GetTypeHash()
	if err != nil {
		t.Errorf("Failed to get type hash: %v", err)
	}

	hash2, err := desc.GetTypeHash()
	if err != nil {
		t.Errorf("Failed to get type hash second time: %v", err)
	}

	if hash1 != hash2 {
		t.Error("Type hash should be consistent")
	}

	// Test with different descriptor
	desc2, err := cache.GetTypeDescriptor(reflect.TypeOf(uint64(0)), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get second type descriptor: %v", err)
	}

	hash3, err := desc2.GetTypeHash()
	if err != nil {
		t.Errorf("Failed to get second type hash: %v", err)
	}

	if hash1 == hash3 {
		t.Error("Different types should have different hashes")
	}
}

// Test getCompatFlag function
func TestTypeCache_GetCompatFlag(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Test with empty compat flags
	flag := cache.getCompatFlag(reflect.TypeOf(uint32(0)), reflect.TypeOf(uint32(0)))
	if flag != 0 {
		t.Errorf("Expected 0 compat flag for uint32, got %d", flag)
	}

	// Test with pointer type
	ptrType := reflect.TypeOf((*uint32)(nil))
	flag = cache.getCompatFlag(ptrType, ptrType)
	if flag != 0 {
		t.Errorf("Expected 0 compat flag for pointer type, got %d", flag)
	}

	// Add a compat flag and test
	cache.CompatFlags["uint32"] = SszCompatFlagFastSSZMarshaler
	flag = cache.getCompatFlag(reflect.TypeOf(uint32(0)), reflect.TypeOf(uint32(0)))
	if flag != SszCompatFlagFastSSZMarshaler {
		t.Errorf("Expected SszCompatFlagFastSSZMarshaler, got %d", flag)
	}
}

// Test concurrent access to cache
func TestTypeCache_ConcurrentAccess(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Test concurrent GetTypeDescriptor calls
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()

			_, err := cache.GetTypeDescriptor(reflect.TypeOf(uint32(0)), nil, nil, nil)
			if err != nil {
				t.Errorf("Concurrent GetTypeDescriptor failed: %v", err)
			}
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Test size hint expressions using dynssz-size tag
func TestTypeCache_SizeHintExpressions(t *testing.T) {
	// Create DynSsz with spec value resolver
	ds := &dummyDynamicSpecs{
		specValues: map[string]uint64{
			"MAX_VALIDATORS_PER_COMMITTEE": 4096,
		},
	}
	cache := NewTypeCache(ds)

	// Test with size hint containing expression via dynssz-size tag
	type TestStruct struct {
		Data []byte `ssz-size:"32" dynssz-size:"MAX_VALIDATORS_PER_COMMITTEE"`
	}

	// This should use the expression (treated as dynamic size)
	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the field has the expression stored
	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	if field.Type.Len != 4096 {
		t.Errorf("expected Len 4096, got %d", field.Type.Len)
	}
	// Should have dynamic size flag set
	if field.Type.SszTypeFlags&SszTypeFlagHasDynamicSize == 0 {
		t.Error("expected SszTypeFlagHasDynamicSize to be set")
	}
}

// Test max hint expressions using dynssz-max tag
func TestTypeCache_MaxHintExpressions(t *testing.T) {
	// Create DynSsz with spec value resolver
	ds := &dummyDynamicSpecs{
		specValues: map[string]uint64{
			"MAX_VALIDATORS": 65536,
		},
	}
	cache := NewTypeCache(ds)

	// Test with max hint containing expression via dynssz-max tag
	type TestStruct struct {
		Data []byte `ssz-max:"100" dynssz-max:"MAX_VALIDATORS"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the limit was set
	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	if field.Type.SszTypeFlags&SszTypeFlagHasLimit == 0 {
		t.Error("expected SszTypeFlagHasLimit to be set")
	}
	// Should have dynamic max flag set
	if field.Type.SszTypeFlags&SszTypeFlagHasDynamicMax == 0 {
		t.Error("expected SszTypeFlagHasDynamicMax to be set")
	}
}

// Test list with string type for coverage
func TestTypeCache_ListWithString(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Test list with string type
	type TestStruct struct {
		Name string `ssz-max:"100"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify string flag is set on field
	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	if field.Type.GoTypeFlags&GoTypeFlagIsString == 0 {
		t.Error("expected GoTypeFlagIsString to be set")
	}
}

// Test vector with string type for coverage
func TestTypeCache_VectorWithString(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Test vector with string type
	type TestStruct struct {
		Name string `ssz-size:"32"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	if field.Type.SszType != SszVectorType {
		t.Errorf("expected SszVectorType, got %v", field.Type.SszType)
	}
}

// Test list with size hint
func TestTypeCache_ListWithSizeHint(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Test list with size and max hints
	type TestStruct struct {
		Data []uint32 `ssz-size:"10" ssz-max:"100"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	// With ssz-size hint, it becomes a vector, not a list
	if field.Type.SszType != SszVectorType {
		t.Errorf("expected SszVectorType, got %v", field.Type.SszType)
	}
}

// Test list with dynamic size hint
func TestTypeCache_ListWithDynamicSizeHint(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Test list with dynamic size hint
	type TestStruct struct {
		Data []uint32 `ssz-size:"?" ssz-max:"100"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	// With dynamic size hint (?), it should be a list
	if field.Type.SszType != SszListType {
		t.Errorf("expected SszListType, got %v", field.Type.SszType)
	}
}

// Test bitvector with bit size hint
func TestTypeCache_BitvectorWithBitSize(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Test bitvector with bit size hint
	type TestStruct struct {
		Flags []byte `ssz-type:"bitvector" ssz-bitsize:"32"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	if field.Type.BitSize != 32 {
		t.Errorf("expected BitSize 32, got %d", field.Type.BitSize)
	}
}

// Test bitlist from type name detection
func TestTypeCache_BitlistFromTypeNameDetection(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Test bitlist from type name detection
	type TestBitlist []uint8

	type TestStruct struct {
		Flags TestBitlist
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	if field.Type.SszType != SszBitlistType {
		t.Errorf("expected SszBitlistType, got %v", field.Type.SszType)
	}
}

// Test container with embedded struct
func TestTypeCache_EmbeddedStruct(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	type Inner struct {
		A uint32
	}

	type Outer struct {
		B uint64
		C Inner
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(Outer{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) != 2 {
		t.Fatal("expected container with 2 fields")
	}

	// Second field should be a container
	field := desc.ContainerDesc.Fields[1]
	if field.Type.SszType != SszContainerType {
		t.Errorf("expected SszContainerType for inner struct, got %v", field.Type.SszType)
	}
}

// Test vector with nested dynamic elements
func TestTypeCache_VectorWithNestedDynamic(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Test vector with dynamic nested type
	type Inner struct {
		Data []byte `ssz-max:"100"`
	}

	type Outer struct {
		Items []Inner `ssz-size:"3"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(Outer{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	// Vector with dynamic elements should still be a vector but with IsDynamic flag
	if field.Type.SszType != SszVectorType {
		t.Errorf("expected SszVectorType, got %v", field.Type.SszType)
	}
	if field.Type.SszTypeFlags&SszTypeFlagIsDynamic == 0 {
		t.Error("expected SszTypeFlagIsDynamic to be set for vector with dynamic elements")
	}
}

type TestTypeWithInvalidHashTreeRootWith1 struct{}

func (t *TestTypeWithInvalidHashTreeRootWith1) HashTreeRootWith() error {
	return errors.New("test HashTreeRootWith error")
}

type TestTypeWithInvalidHashTreeRootWith2 struct{}

func (t *TestTypeWithInvalidHashTreeRootWith2) HashTreeRootWith(in1 uint64) uint64 {
	return in1
}

// Test types with invalid HashTreeRootWith method
func TestTypeCache_InvalidHashTreeRootWith(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestTypeWithInvalidHashTreeRootWith1{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.HashTreeRootWithMethod != nil {
		t.Errorf("expected HashTreeRootWithMethod to be nil, got %v", desc.HashTreeRootWithMethod)
	}

	desc, err = cache.GetTypeDescriptor(reflect.TypeOf(TestTypeWithInvalidHashTreeRootWith2{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.HashTreeRootWithMethod != nil {
		t.Errorf("expected HashTreeRootWithMethod to be nil, got %v", desc.HashTreeRootWithMethod)
	}
}

// Test types for view descriptor tests
type runtimeContainer struct {
	FieldA uint64
	FieldB uint32
	FieldC []byte
}

type schemaContainerV1 struct {
	FieldA uint64
	FieldB uint32
}

type schemaContainerV2 struct {
	FieldB uint32
	FieldA uint64
}

type schemaContainerMissingField struct {
	FieldA uint64
	FieldX uint32 // This field doesn't exist in runtimeContainer
}

// Nested view descriptor types
type runtimeInner struct {
	Value uint64
	Data  []byte
}

type runtimeOuter struct {
	ID    uint32
	Inner runtimeInner
}

type schemaInnerV1 struct {
	Value uint64
}

type schemaOuterV1 struct {
	ID    uint32
	Inner schemaInnerV1
}

// Test GetTypeDescriptorWithSchema basic functionality
func TestTypeCache_GetTypeDescriptorWithSchema(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	t.Run("SameRuntimeAndSchema", func(t *testing.T) {
		runtimeType := reflect.TypeOf(runtimeContainer{})
		schemaType := runtimeType

		desc, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaType, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if desc.SszType != SszContainerType {
			t.Errorf("expected SszContainerType, got %v", desc.SszType)
		}

		// When runtime == schema, FieldIndex should match field order
		for i, field := range desc.ContainerDesc.Fields {
			if int(field.FieldIndex) != i {
				t.Errorf("field %s: expected FieldIndex %d, got %d", field.Name, i, field.FieldIndex)
			}
		}
	})

	t.Run("DifferentSchemaFieldOrder", func(t *testing.T) {
		runtimeType := reflect.TypeOf(runtimeContainer{})
		schemaType := reflect.TypeOf(schemaContainerV2{})

		desc, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaType, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Schema has FieldB first, then FieldA
		// FieldB should map to runtime index 1, FieldA to runtime index 0
		if len(desc.ContainerDesc.Fields) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(desc.ContainerDesc.Fields))
		}

		// First field in schema is FieldB -> runtime index 1
		if desc.ContainerDesc.Fields[0].Name != "FieldB" {
			t.Errorf("expected first field to be FieldB, got %s", desc.ContainerDesc.Fields[0].Name)
		}
		if desc.ContainerDesc.Fields[0].FieldIndex != 1 {
			t.Errorf("FieldB: expected FieldIndex 1, got %d", desc.ContainerDesc.Fields[0].FieldIndex)
		}

		// Second field in schema is FieldA -> runtime index 0
		if desc.ContainerDesc.Fields[1].Name != "FieldA" {
			t.Errorf("expected second field to be FieldA, got %s", desc.ContainerDesc.Fields[1].Name)
		}
		if desc.ContainerDesc.Fields[1].FieldIndex != 0 {
			t.Errorf("FieldA: expected FieldIndex 0, got %d", desc.ContainerDesc.Fields[1].FieldIndex)
		}
	})

	t.Run("SchemaWithFewerFields", func(t *testing.T) {
		runtimeType := reflect.TypeOf(runtimeContainer{})
		schemaType := reflect.TypeOf(schemaContainerV1{})

		desc, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaType, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Schema only has 2 fields (FieldA and FieldB)
		if len(desc.ContainerDesc.Fields) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(desc.ContainerDesc.Fields))
		}

		// Verify FieldIndex mapping
		if desc.ContainerDesc.Fields[0].Name != "FieldA" || desc.ContainerDesc.Fields[0].FieldIndex != 0 {
			t.Errorf("FieldA mapping incorrect: name=%s, index=%d", desc.ContainerDesc.Fields[0].Name, desc.ContainerDesc.Fields[0].FieldIndex)
		}
		if desc.ContainerDesc.Fields[1].Name != "FieldB" || desc.ContainerDesc.Fields[1].FieldIndex != 1 {
			t.Errorf("FieldB mapping incorrect: name=%s, index=%d", desc.ContainerDesc.Fields[1].Name, desc.ContainerDesc.Fields[1].FieldIndex)
		}
	})
}

// Test view descriptor error cases
func TestTypeCache_ViewDescriptorErrors(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	t.Run("SchemaFieldNotFoundInRuntime", func(t *testing.T) {
		runtimeType := reflect.TypeOf(runtimeContainer{})
		schemaType := reflect.TypeOf(schemaContainerMissingField{})

		_, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaType, nil, nil, nil)
		if err == nil {
			t.Error("expected error for schema field not found in runtime")
			return
		}
		if !strings.Contains(err.Error(), "schema field \"FieldX\" not found in runtime type") {
			t.Errorf("unexpected error: %s", err.Error())
		}
	})
}

// Test cache keying with (runtime, schema) pairs
func TestTypeCache_CacheKeyWithSchema(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	runtimeType := reflect.TypeOf(runtimeContainer{})
	schemaV1 := reflect.TypeOf(schemaContainerV1{})
	schemaV2 := reflect.TypeOf(schemaContainerV2{})

	// Get descriptor with schema V1
	descV1, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaV1, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error for V1: %v", err)
	}

	// Get descriptor with schema V2
	descV2, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaV2, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error for V2: %v", err)
	}

	// Descriptors should be different (different field order)
	if descV1 == descV2 {
		t.Error("expected different descriptors for different schemas")
	}

	// V1 has FieldA first, V2 has FieldB first
	if descV1.ContainerDesc.Fields[0].Name != "FieldA" {
		t.Errorf("V1 first field should be FieldA, got %s", descV1.ContainerDesc.Fields[0].Name)
	}
	if descV2.ContainerDesc.Fields[0].Name != "FieldB" {
		t.Errorf("V2 first field should be FieldB, got %s", descV2.ContainerDesc.Fields[0].Name)
	}

	// Getting the same (runtime, schema) pair again should return cached descriptor
	descV1Again, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaV1, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error for V1 again: %v", err)
	}
	if descV1 != descV1Again {
		t.Error("expected same descriptor for same (runtime, schema) pair")
	}
}

// Test nested view descriptors
func TestTypeCache_NestedViewDescriptors(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	t.Run("NestedContainerWithViewDescriptor", func(t *testing.T) {
		runtimeType := reflect.TypeOf(runtimeOuter{})
		schemaType := reflect.TypeOf(schemaOuterV1{})

		desc, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaType, nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(desc.ContainerDesc.Fields) != 2 {
			t.Fatalf("expected 2 fields, got %d", len(desc.ContainerDesc.Fields))
		}

		// Check the Inner field
		innerField := desc.ContainerDesc.Fields[1]
		if innerField.Name != "Inner" {
			t.Errorf("expected second field to be Inner, got %s", innerField.Name)
		}

		// The inner container should have been built with the nested schema
		innerDesc := innerField.Type
		if innerDesc.SszType != SszContainerType {
			t.Errorf("expected Inner to be SszContainerType, got %v", innerDesc.SszType)
		}

		// schemaInnerV1 only has Value field, not Data
		if len(innerDesc.ContainerDesc.Fields) != 1 {
			t.Errorf("expected inner container to have 1 field, got %d", len(innerDesc.ContainerDesc.Fields))
		}

		if innerDesc.ContainerDesc.Fields[0].Name != "Value" {
			t.Errorf("expected inner field to be Value, got %s", innerDesc.ContainerDesc.Fields[0].Name)
		}

		// Verify FieldIndex points to correct runtime field (Value is at index 0 in runtimeInner)
		if innerDesc.ContainerDesc.Fields[0].FieldIndex != 0 {
			t.Errorf("expected Value FieldIndex 0, got %d", innerDesc.ContainerDesc.Fields[0].FieldIndex)
		}
	})
}

// Test view descriptor with dynamic fields
func TestTypeCache_ViewDescriptorWithDynamicFields(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	type runtimeWithDynamic struct {
		StaticField  uint64
		DynamicField []byte
	}

	type schemaWithDynamic struct {
		DynamicField []byte `ssz-max:"100"`
		StaticField  uint64
	}

	runtimeType := reflect.TypeOf(runtimeWithDynamic{})
	schemaType := reflect.TypeOf(schemaWithDynamic{})

	desc, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaType, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Schema has DynamicField first (index 0), StaticField second (index 1)
	// Runtime has StaticField first (index 0), DynamicField second (index 1)
	if len(desc.ContainerDesc.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(desc.ContainerDesc.Fields))
	}

	// First schema field is DynamicField -> runtime index 1
	if desc.ContainerDesc.Fields[0].Name != "DynamicField" {
		t.Errorf("expected first field to be DynamicField, got %s", desc.ContainerDesc.Fields[0].Name)
	}
	if desc.ContainerDesc.Fields[0].FieldIndex != 1 {
		t.Errorf("DynamicField: expected FieldIndex 1, got %d", desc.ContainerDesc.Fields[0].FieldIndex)
	}

	// Second schema field is StaticField -> runtime index 0
	if desc.ContainerDesc.Fields[1].Name != "StaticField" {
		t.Errorf("expected second field to be StaticField, got %s", desc.ContainerDesc.Fields[1].Name)
	}
	if desc.ContainerDesc.Fields[1].FieldIndex != 0 {
		t.Errorf("StaticField: expected FieldIndex 0, got %d", desc.ContainerDesc.Fields[1].FieldIndex)
	}

	// DynFields should reference correct runtime field index
	if len(desc.ContainerDesc.DynFields) != 1 {
		t.Fatalf("expected 1 dynamic field, got %d", len(desc.ContainerDesc.DynFields))
	}

	dynField := desc.ContainerDesc.DynFields[0]
	if dynField.Field.Name != "DynamicField" {
		t.Errorf("expected dynamic field to be DynamicField, got %s", dynField.Field.Name)
	}
	// DynField.Index should be runtime field index (1)
	if dynField.Index != 1 {
		t.Errorf("expected DynField.Index 1, got %d", dynField.Index)
	}
}

// Test view descriptor with vector containing nested schema
func TestTypeCache_ViewDescriptorWithVector(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	type runtimeElem struct {
		A uint64
		B uint32
	}

	type schemaElem struct {
		B uint32
		A uint64
	}

	type runtimeWithVector struct {
		Items []runtimeElem
	}

	type schemaWithVector struct {
		Items []schemaElem `ssz-max:"10"`
	}

	runtimeType := reflect.TypeOf(runtimeWithVector{})
	schemaType := reflect.TypeOf(schemaWithVector{})

	desc, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaType, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(desc.ContainerDesc.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(desc.ContainerDesc.Fields))
	}

	itemsField := desc.ContainerDesc.Fields[0]
	if itemsField.Type.SszType != SszListType {
		t.Errorf("expected SszListType, got %v", itemsField.Type.SszType)
	}

	// Element descriptor should reflect the schema's field order
	elemDesc := itemsField.Type.ElemDesc
	if elemDesc.SszType != SszContainerType {
		t.Errorf("expected element to be SszContainerType, got %v", elemDesc.SszType)
	}

	// Schema has B first, A second
	if len(elemDesc.ContainerDesc.Fields) != 2 {
		t.Fatalf("expected 2 element fields, got %d", len(elemDesc.ContainerDesc.Fields))
	}

	if elemDesc.ContainerDesc.Fields[0].Name != "B" {
		t.Errorf("expected first element field to be B, got %s", elemDesc.ContainerDesc.Fields[0].Name)
	}
	if elemDesc.ContainerDesc.Fields[1].Name != "A" {
		t.Errorf("expected second element field to be A, got %s", elemDesc.ContainerDesc.Fields[1].Name)
	}

	// FieldIndex should map to runtime element fields
	// runtimeElem has A at 0, B at 1
	if elemDesc.ContainerDesc.Fields[0].FieldIndex != 1 { // B in schema -> index 1 in runtime
		t.Errorf("B: expected FieldIndex 1, got %d", elemDesc.ContainerDesc.Fields[0].FieldIndex)
	}
	if elemDesc.ContainerDesc.Fields[1].FieldIndex != 0 { // A in schema -> index 0 in runtime
		t.Errorf("A: expected FieldIndex 0, got %d", elemDesc.ContainerDesc.Fields[1].FieldIndex)
	}
}

// Test concurrent access with view descriptors
func TestTypeCache_ConcurrentAccessWithSchema(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	runtimeType := reflect.TypeOf(runtimeContainer{})
	schemaV1 := reflect.TypeOf(schemaContainerV1{})
	schemaV2 := reflect.TypeOf(schemaContainerV2{})

	done := make(chan bool, 20)

	// Concurrent access with different schemas
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			_, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaV1, nil, nil, nil)
			if err != nil {
				t.Errorf("concurrent GetTypeDescriptorWithSchema V1 failed: %v", err)
			}
		}()
		go func() {
			defer func() { done <- true }()
			_, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaV2, nil, nil, nil)
			if err != nil {
				t.Errorf("concurrent GetTypeDescriptorWithSchema V2 failed: %v", err)
			}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}
}
