// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package ssztypes

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/pk910/dynamic-ssz/sszutils"
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
			{"int", reflect.TypeOf(int(0)), "signed or unsigned integers with unspecified size are not supported"},
			{"uint", reflect.TypeOf(uint(0)), "signed or unsigned integers with unspecified size are not supported"},
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

	hash1 := desc.GetTypeHash()
	hash2 := desc.GetTypeHash()

	if hash1 != hash2 {
		t.Error("Type hash should be consistent")
	}

	// Test with different descriptor
	desc2, err := cache.GetTypeDescriptor(reflect.TypeOf(uint64(0)), nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to get second type descriptor: %v", err)
	}

	hash3 := desc2.GetTypeHash()

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

// Test extended type error cases
func TestTypeCache_ExtendedTypes(t *testing.T) {
	ds := &dummyDynamicSpecs{}

	t.Run("DisabledExtendedTypes", func(t *testing.T) {
		cache := NewTypeCache(ds)
		// ExtendedTypes defaults to false

		tests := []struct {
			name     string
			typ      reflect.Type
			hints    []SszTypeHint
			expected string
		}{
			{
				name:     "int8 disabled",
				typ:      reflect.TypeOf(int8(0)),
				hints:    []SszTypeHint{{Type: SszInt8Type}},
				expected: "signed integers are not supported in SSZ",
			},
			{
				name:     "int16 disabled",
				typ:      reflect.TypeOf(int16(0)),
				hints:    []SszTypeHint{{Type: SszInt16Type}},
				expected: "signed integers are not supported in SSZ",
			},
			{
				name:     "int32 disabled",
				typ:      reflect.TypeOf(int32(0)),
				hints:    []SszTypeHint{{Type: SszInt32Type}},
				expected: "signed integers are not supported in SSZ",
			},
			{
				name:     "int64 disabled",
				typ:      reflect.TypeOf(int64(0)),
				hints:    []SszTypeHint{{Type: SszInt64Type}},
				expected: "signed integers are not supported in SSZ",
			},
			{
				name:     "float32 disabled",
				typ:      reflect.TypeOf(float32(0)),
				hints:    []SszTypeHint{{Type: SszFloat32Type}},
				expected: "floating-point numbers are not supported in SSZ",
			},
			{
				name:     "float64 disabled",
				typ:      reflect.TypeOf(float64(0)),
				hints:    []SszTypeHint{{Type: SszFloat64Type}},
				expected: "floating-point numbers are not supported in SSZ",
			},
			{
				name:     "optional disabled",
				typ:      reflect.TypeOf((*int16)(nil)),
				hints:    []SszTypeHint{{Type: SszOptionalType}},
				expected: "optional types are not supported in SSZ",
			},
			{
				name:     "bigint disabled",
				typ:      reflect.TypeOf(struct{}{}),
				hints:    []SszTypeHint{{Type: SszBigIntType}},
				expected: "big integers are not supported in SSZ",
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

	t.Run("EnabledExtendedTypes", func(t *testing.T) {
		cache := NewTypeCache(ds)
		cache.ExtendedTypes = true

		successTests := []struct {
			name string
			typ  reflect.Type
		}{
			{"int8", reflect.TypeOf(int8(0))},
			{"int16", reflect.TypeOf(int16(0))},
			{"int32", reflect.TypeOf(int32(0))},
			{"int64", reflect.TypeOf(int64(0))},
			{"float32", reflect.TypeOf(float32(0))},
			{"float64", reflect.TypeOf(float64(0))},
		}

		for _, tt := range successTests {
			t.Run(tt.name, func(t *testing.T) {
				desc, err := cache.GetTypeDescriptor(tt.typ, nil, nil, nil)
				if err != nil {
					t.Errorf("Unexpected error for %s: %v", tt.name, err)
					return
				}
				if desc == nil {
					t.Errorf("Expected descriptor for %s", tt.name)
				}
			})
		}
	})

	t.Run("WrongKindErrors", func(t *testing.T) {
		cache := NewTypeCache(ds)
		cache.ExtendedTypes = true

		tests := []struct {
			name     string
			typ      reflect.Type
			hints    []SszTypeHint
			expected string
		}{
			{
				name:     "int8 with wrong kind",
				typ:      reflect.TypeOf(uint8(0)),
				hints:    []SszTypeHint{{Type: SszInt8Type}},
				expected: "int8 ssz type can only be represented by int8 types",
			},
			{
				name:     "int16 with wrong kind",
				typ:      reflect.TypeOf(uint16(0)),
				hints:    []SszTypeHint{{Type: SszInt16Type}},
				expected: "int16 ssz type can only be represented by int16 types",
			},
			{
				name:     "int32 with wrong kind",
				typ:      reflect.TypeOf(uint32(0)),
				hints:    []SszTypeHint{{Type: SszInt32Type}},
				expected: "int32 ssz type can only be represented by int32 types",
			},
			{
				name:     "int64 with wrong kind",
				typ:      reflect.TypeOf(uint64(0)),
				hints:    []SszTypeHint{{Type: SszInt64Type}},
				expected: "int64 ssz type can only be represented by int64 types",
			},
			{
				name:     "float32 with wrong kind",
				typ:      reflect.TypeOf(uint32(0)),
				hints:    []SszTypeHint{{Type: SszFloat32Type}},
				expected: "float32 ssz type can only be represented by float32 types",
			},
			{
				name:     "float64 with wrong kind",
				typ:      reflect.TypeOf(uint64(0)),
				hints:    []SszTypeHint{{Type: SszFloat64Type}},
				expected: "float64 ssz type can only be represented by float64 types",
			},
			{
				name:     "optional with non-pointer",
				typ:      reflect.TypeOf(int16(0)),
				hints:    []SszTypeHint{{Type: SszOptionalType}},
				expected: "optional ssz type can only be represented by pointer types",
			},
			{
				name:     "bigint with non-struct",
				typ:      reflect.TypeOf(uint64(0)),
				hints:    []SszTypeHint{{Type: SszBigIntType}},
				expected: "bigint type can only be represented by struct types",
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

	t.Run("OptionalDescriptor", func(t *testing.T) {
		cache := NewTypeCache(ds)
		cache.ExtendedTypes = true

		// optional pointer to uint16
		desc, err := cache.GetTypeDescriptor(
			reflect.TypeOf((*uint16)(nil)),
			nil, nil,
			[]SszTypeHint{{Type: SszOptionalType}},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszType != SszOptionalType {
			t.Errorf("expected SszOptionalType, got %v", desc.SszType)
		}
		if desc.SszTypeFlags&SszTypeFlagIsDynamic == 0 {
			t.Error("expected optional to be dynamic")
		}
		if desc.ElemDesc == nil {
			t.Error("expected ElemDesc to be set")
		}
	})

	t.Run("OptionalDescriptorWithHints", func(t *testing.T) {
		cache := NewTypeCache(ds)
		cache.ExtendedTypes = true

		// optional pointer to uint16 with extra hints that get forwarded
		desc, err := cache.GetTypeDescriptor(
			reflect.TypeOf((*uint16)(nil)),
			[]SszSizeHint{{Size: 0}, {Size: 2}},
			[]SszMaxSizeHint{{Size: 0}, {Size: 2}},
			[]SszTypeHint{{Type: SszOptionalType}, {Type: SszUint16Type}},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszType != SszOptionalType {
			t.Errorf("expected SszOptionalType, got %v", desc.SszType)
		}
		if desc.ElemDesc == nil {
			t.Error("expected ElemDesc to be set")
		}
	})

	t.Run("BigIntDescriptor", func(t *testing.T) {
		cache := NewTypeCache(ds)
		cache.ExtendedTypes = true

		type BigIntLike struct {
			// big.Int is a struct, so we test with a struct type
		}

		desc, err := cache.GetTypeDescriptor(
			reflect.TypeOf(BigIntLike{}),
			nil, nil,
			[]SszTypeHint{{Type: SszBigIntType}},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszType != SszBigIntType {
			t.Errorf("expected SszBigIntType, got %v", desc.SszType)
		}
		if desc.SszTypeFlags&SszTypeFlagIsDynamic == 0 {
			t.Error("expected bigint to be dynamic")
		}
	})
}

// Test ParseSszType for all types
func TestParseSszType(t *testing.T) {
	tests := []struct {
		input    string
		expected SszType
	}{
		// auto/unspecified
		{"?", SszUnspecifiedType},
		{"auto", SszUnspecifiedType},
		{"custom", SszCustomType},
		{"wrapper", SszTypeWrapperType},
		{"type-wrapper", SszTypeWrapperType},

		// basic types
		{"bool", SszBoolType},
		{"uint8", SszUint8Type},
		{"uint16", SszUint16Type},
		{"uint32", SszUint32Type},
		{"uint64", SszUint64Type},
		{"uint128", SszUint128Type},
		{"uint256", SszUint256Type},

		// complex types
		{"container", SszContainerType},
		{"list", SszListType},
		{"vector", SszVectorType},
		{"bitlist", SszBitlistType},
		{"bitvector", SszBitvectorType},
		{"progressive-list", SszProgressiveListType},
		{"progressive-bitlist", SszProgressiveBitlistType},
		{"progressive-container", SszProgressiveContainerType},
		{"compatible-union", SszCompatibleUnionType},
		{"union", SszCompatibleUnionType},

		// extended types
		{"int8", SszInt8Type},
		{"int16", SszInt16Type},
		{"int32", SszInt32Type},
		{"int64", SszInt64Type},
		{"float32", SszFloat32Type},
		{"float64", SszFloat64Type},
		{"bigint", SszBigIntType},
		{"optional", SszOptionalType},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ParseSszType(tt.input)
			if err != nil {
				t.Errorf("unexpected error for '%s': %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}

	// Test invalid type
	_, err := ParseSszType("invalidtype")
	if err == nil {
		t.Error("expected error for invalid type")
	}
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

func TestListWithFixedSizeRejected(t *testing.T) {
	// Bug fix: ssz-type:"list" combined with a fixed ssz-size is invalid.
	// Lists use ssz-max, not ssz-size. This should return an error.
	cache := NewTypeCache(nil)

	type InvalidListWithSize struct {
		Field []uint16 `ssz-type:"list" ssz-size:"4"`
	}

	_, err := cache.GetTypeDescriptor(reflect.TypeOf(InvalidListWithSize{}), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for list with fixed ssz-size, got nil")
	}
	if !strings.Contains(err.Error(), "list types cannot have a fixed ssz-size") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// errorDynamicSpecs returns an error from ResolveSpecValue for testing error paths.
type errorDynamicSpecs struct {
	err error
}

func (d *errorDynamicSpecs) ResolveSpecValue(name string) (bool, uint64, error) {
	return false, 0, d.err
}

func makeField(name string, typ reflect.Type, tag reflect.StructTag) *reflect.StructField {
	return &reflect.StructField{
		Name: name,
		Type: typ,
		Tag:  tag,
	}
}

func TestGetSszSizeTagBitsizeParseError(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	field := makeField("Bad", reflect.TypeOf(uint32(0)), `ssz-bitsize:"notanumber"`)

	_, err := getSszSizeTag(ds, field)
	if err == nil {
		t.Fatal("expected error for invalid ssz-bitsize")
	}
	if !strings.Contains(err.Error(), "error parsing ssz-bitsize tag") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetSszSizeTagDynSszDynamic(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	// dynssz-size:"?" should set Dynamic=true
	field := makeField("Dyn", reflect.TypeOf([]byte{}), `ssz-size:"32" dynssz-size:"?"`)

	sizes, err := getSszSizeTag(ds, field)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sizes) != 1 {
		t.Fatalf("expected 1 size hint, got %d", len(sizes))
	}
	// The dynssz-size:"?" case: sizeExpr == "?" → sszSize.Dynamic = true
	// But since ssz-size:"32" was parsed first, the size remains 32
	// and the dynssz loop processes "?" which sets Dynamic on the new hint,
	// but then i < len(sszSizes) so it checks if sizes differ.
	// Actually: sszSize has Dynamic=true, Size=0. sszSizes[0] has Size=32.
	// 0 != 32, so it updates sszSizes[0] to the dynamic version.
	if !sizes[0].Dynamic {
		t.Fatal("expected Dynamic=true for dynssz-size:\"?\"")
	}
}

func TestGetSszSizeTagDynSszResolveError(t *testing.T) {
	ds := &errorDynamicSpecs{err: fmt.Errorf("resolve failed")}
	// dynssz-size with an expression that triggers ResolveSpecValue error
	field := makeField("Expr", reflect.TypeOf([]byte{}), `ssz-size:"32" dynssz-size:"SOME_EXPR"`)

	_, err := getSszSizeTag(ds, field)
	if err == nil {
		t.Fatal("expected error for failed spec resolution")
	}
	if !strings.Contains(err.Error(), "error parsing dynssz-size tag") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetSszSizeTagDynSszExtraDimension(t *testing.T) {
	ds := &dummyDynamicSpecs{specValues: map[string]uint64{"X": 100}}
	// dynssz-size has more dimensions than ssz-size → i >= len(sszSizes)
	field := makeField("Extra", reflect.TypeOf([]byte{}), `dynssz-size:"X"`)

	sizes, err := getSszSizeTag(ds, field)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sizes) != 1 {
		t.Fatalf("expected 1 size hint, got %d", len(sizes))
	}
	if sizes[0].Size != 100 {
		t.Fatalf("expected size 100, got %d", sizes[0].Size)
	}
}

func TestGetSszMaxSizeTagDynSszMaxDynamic(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	// dynssz-max:"?" should set NoValue=true
	field := makeField("Dyn", reflect.TypeOf([]byte{}), `ssz-max:"100" dynssz-max:"?"`)

	maxSizes, err := getSszMaxSizeTag(ds, field)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(maxSizes) != 1 {
		t.Fatalf("expected 1 max size hint, got %d", len(maxSizes))
	}
}

func TestGetSszMaxSizeTagDynSszMaxNumeric(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	// dynssz-max:"200" with numeric value
	field := makeField("Num", reflect.TypeOf([]byte{}), `ssz-max:"100" dynssz-max:"200"`)

	maxSizes, err := getSszMaxSizeTag(ds, field)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(maxSizes) != 1 {
		t.Fatalf("expected 1 max size hint, got %d", len(maxSizes))
	}
	if maxSizes[0].Size != 200 {
		t.Fatalf("expected max size 200, got %d", maxSizes[0].Size)
	}
}

func TestGetSszMaxSizeTagDynSszMaxResolveError(t *testing.T) {
	ds := &errorDynamicSpecs{err: fmt.Errorf("resolve failed")}
	// dynssz-max with expression that triggers ResolveSpecValue error
	field := makeField("Expr", reflect.TypeOf([]byte{}), `ssz-max:"100" dynssz-max:"BAD_EXPR"`)

	_, err := getSszMaxSizeTag(ds, field)
	if err == nil {
		t.Fatal("expected error for failed spec resolution")
	}
	if !strings.Contains(err.Error(), "error parsing dynssz-max tag") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetSszMaxSizeTagDynSszMaxExtraDimension(t *testing.T) {
	ds := &dummyDynamicSpecs{specValues: map[string]uint64{"Y": 500}}
	// dynssz-max has more dimensions than ssz-max → i >= len(sszMaxSizes)
	field := makeField("Extra", reflect.TypeOf([]byte{}), `dynssz-max:"Y"`)

	maxSizes, err := getSszMaxSizeTag(ds, field)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(maxSizes) != 1 {
		t.Fatalf("expected 1 max size hint, got %d", len(maxSizes))
	}
	if maxSizes[0].Size != 500 {
		t.Fatalf("expected max size 500, got %d", maxSizes[0].Size)
	}
}

// TypeWrapper descriptor tests

type testWrapperNoReturn struct{}

func (t *testWrapperNoReturn) GetDescriptorType() {}

type testWrapperWrongReturn struct{}

func (t *testWrapperWrongReturn) GetDescriptorType() string { return "not a type" }

func TestBuildTypeWrapperNoResults(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})
	typeHints := []SszTypeHint{{Type: SszTypeWrapperType}}

	_, err := cache.GetTypeDescriptor(reflect.TypeOf(testWrapperNoReturn{}), nil, nil, typeHints)
	if err == nil {
		t.Fatal("expected error for GetDescriptorType with no return values")
	}
	if !strings.Contains(err.Error(), "GetDescriptorType returned no results") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildTypeWrapperWrongReturnType(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})
	typeHints := []SszTypeHint{{Type: SszTypeWrapperType}}

	_, err := cache.GetTypeDescriptor(reflect.TypeOf(testWrapperWrongReturn{}), nil, nil, typeHints)
	if err == nil {
		t.Fatal("expected error for GetDescriptorType with wrong return type")
	}
	if !strings.Contains(err.Error(), "GetDescriptorType did not return a reflect.Type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Container descriptor error tests

type containerBadSszSize struct {
	Field []byte `ssz-size:"abc"`
}

func TestBuildContainerSszSizeTagError(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	_, err := cache.GetTypeDescriptor(reflect.TypeOf(containerBadSszSize{}), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid ssz-size tag")
	}
}

type containerBadSszMax struct {
	Field []byte `ssz-max:"abc"`
}

func TestBuildContainerSszMaxTagError(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	_, err := cache.GetTypeDescriptor(reflect.TypeOf(containerBadSszMax{}), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid ssz-max tag")
	}
}

type containerBadSszType struct {
	Field uint32 `ssz-type:"invalidtype"`
}

func TestBuildContainerSszTypeTagError(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	_, err := cache.GetTypeDescriptor(reflect.TypeOf(containerBadSszType{}), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid ssz-type tag")
	}
}

func TestListWithDynamicSizeAccepted(t *testing.T) {
	// ssz-type:"list" with ssz-size:"?" (dynamic) should be accepted
	cache := NewTypeCache(nil)

	type ValidListWithDynamicSize struct {
		Field []uint16 `ssz-type:"list" ssz-size:"?" ssz-max:"10"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(ValidListWithDynamicSize{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error for valid list: %v", err)
	}
	if desc == nil {
		t.Fatal("descriptor should not be nil")
	}
}

// Test numeric dynssz-size override to cover getSszSizeTag line 276 (err==nil branch)
func TestTypeCache_NumericDynSszSizeOverride(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	type TestStruct struct {
		Data []byte `ssz-size:"32" dynssz-size:"64"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	if field.Type.Len != 64 {
		t.Errorf("expected Len 64 from dynssz-size override, got %d", field.Type.Len)
	}
	if field.Type.SszType != SszVectorType {
		t.Errorf("expected SszVectorType, got %v", field.Type.SszType)
	}
}

// Test annotation with expression-based dynssz-size resolved by TypeCache specs
// (covers typecache.go lines 152-156: annotation Expr resolution)
func TestTypeCache_AnnotationSizeExprResolution(t *testing.T) {
	type annotatedVec []byte
	sszutils.Annotate[annotatedVec](`ssz-size:"32" dynssz-size:"TEST_ANN_SIZE"`)

	ds := &dummyDynamicSpecs{
		specValues: map[string]uint64{
			"TEST_ANN_SIZE": 64,
		},
	}
	cache := NewTypeCache(ds)

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(annotatedVec{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.Len != 64 {
		t.Errorf("expected Len 64 from annotation expr resolution, got %d", desc.Len)
	}
	if desc.SszTypeFlags&SszTypeFlagHasDynamicSize == 0 {
		t.Error("expected SszTypeFlagHasDynamicSize to be set")
	}
	if desc.SszTypeFlags&SszTypeFlagHasSizeExpr == 0 {
		t.Error("expected SszTypeFlagHasSizeExpr to be set")
	}
}

// Test annotation with expression-based dynssz-max resolved by TypeCache specs
// (covers typecache.go lines 161-166: annotation max Expr resolution)
func TestTypeCache_AnnotationMaxExprResolution(t *testing.T) {
	type annotatedMaxList []byte
	sszutils.Annotate[annotatedMaxList](`ssz-max:"100" dynssz-max:"TEST_ANN_MAX"`)

	ds := &dummyDynamicSpecs{
		specValues: map[string]uint64{
			"TEST_ANN_MAX": 200,
		},
	}
	cache := NewTypeCache(ds)

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(annotatedMaxList{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.Limit != 200 {
		t.Errorf("expected Limit 200 from annotation max expr resolution, got %d", desc.Limit)
	}
	if desc.SszTypeFlags&SszTypeFlagHasDynamicMax == 0 {
		t.Error("expected SszTypeFlagHasDynamicMax to be set")
	}
}

func TestParseTags_DynMaxNumericOverride(t *testing.T) {
	// dynssz-max with a numeric value that differs from ssz-max
	_, _, maxHints, err := ParseTags(`ssz-max:"10" dynssz-max:"20"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(maxHints) != 1 {
		t.Fatalf("expected 1 max hint, got %d", len(maxHints))
	}

	if maxHints[0].Size != 20 {
		t.Fatalf("expected dynssz-max override to 20, got %d", maxHints[0].Size)
	}
}

// Test dynssz-max expression without corresponding ssz-max tag
// (covers the fixed continue placement in ParseTags dynssz-max)
func TestParseTags_DynMaxExprWithoutSszMax(t *testing.T) {
	_, _, maxHints, err := ParseTags(`dynssz-max:"SOME_MAX_EXPR"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(maxHints) != 1 {
		t.Fatalf("expected 1 max hint, got %d", len(maxHints))
	}

	if maxHints[0].Expr != "SOME_MAX_EXPR" {
		t.Errorf("expected Expr 'SOME_MAX_EXPR', got %q", maxHints[0].Expr)
	}
	if !maxHints[0].Custom {
		t.Error("expected Custom to be set")
	}
}

func TestTypeCache_AnnotationRegistryLookup(t *testing.T) {
	type annotatedSlice []uint32

	sszutils.Annotate[annotatedSlice](`ssz-max:"50"`)

	cache := NewTypeCache(nil)
	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(annotatedSlice{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.SszTypeFlags&SszTypeFlagHasLimit == 0 {
		t.Fatal("expected SszTypeFlagHasLimit to be set from annotation")
	}

	if desc.Limit != 50 {
		t.Fatalf("expected limit 50, got %d", desc.Limit)
	}
}

func TestGetWellKnownExternalType(t *testing.T) {
	tests := []struct {
		pkgPath  string
		name     string
		expected SszType
	}{
		{"time", "Time", SszUint64Type},
		{"math/big", "Int", SszBigIntType},
		{"github.com/holiman/uint256", "Int", SszUint256Type},
		{"github.com/prysmaticlabs/go-bitfield", "Bitlist", SszBitlistType},
		{"github.com/OffchainLabs/go-bitfield", "Bitlist", SszBitlistType},
		{"github.com/pk910/dynamic-ssz", "CompatibleUnion[Foo]", SszCompatibleUnionType},
		{"github.com/pk910/dynamic-ssz", "TypeWrapper[Bar]", SszTypeWrapperType},
		{"unknown/pkg", "Unknown", SszUnspecifiedType},
	}

	for _, tt := range tests {
		t.Run(tt.pkgPath+"."+tt.name, func(t *testing.T) {
			got := getWellKnownExternalType(tt.pkgPath, tt.name)
			if got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

// Test that buildUintDescriptor propagates element descriptor errors
// (covers the err != nil branch in getTypeDescriptor for elem type)
func TestTypeCache_UintDescriptorElemError(t *testing.T) {
	type badElemUint64 uint64
	sszutils.Annotate[badElemUint64](`ssz-size:"notanumber"`)

	cache := NewTypeCache(nil)

	_, err := cache.GetTypeDescriptor(reflect.TypeOf([2]badElemUint64{}), nil, nil, []SszTypeHint{{Type: SszUint128Type}})
	if err == nil {
		t.Fatal("expected error from bad element annotation")
	}
	if !strings.Contains(err.Error(), "notanumber") {
		t.Errorf("expected error about bad annotation, got: %s", err.Error())
	}
}

func TestTypeCache_AnnotationRegistryInvalidTag(t *testing.T) {
	type badAnnotatedSlice []uint32

	sszutils.Annotate[badAnnotatedSlice](`ssz-size:"notanumber"`)

	cache := NewTypeCache(nil)
	_, err := cache.GetTypeDescriptor(reflect.TypeOf(badAnnotatedSlice{}), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid annotation tag")
	}
}

// --- ParseTags comprehensive tests ---

func TestParseTags_Comprehensive(t *testing.T) {
	t.Run("EmptyTag", func(t *testing.T) {
		typeHints, sizeHints, maxHints, err := ParseTags("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if typeHints != nil || sizeHints != nil || maxHints != nil {
			t.Fatal("expected all nil for empty tag")
		}
	})

	t.Run("SszTypeContainer", func(t *testing.T) {
		typeHints, _, _, err := ParseTags(`ssz-type:"container"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(typeHints) != 1 || typeHints[0].Type != SszContainerType {
			t.Fatalf("expected SszContainerType, got %v", typeHints)
		}
	})

	t.Run("SszTypeParseError", func(t *testing.T) {
		_, _, _, err := ParseTags(`ssz-type:"invalidtype"`)
		if err == nil {
			t.Fatal("expected error for invalid ssz-type")
		}
		if !strings.Contains(err.Error(), "error parsing ssz-type tag") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("SszBitsize", func(t *testing.T) {
		_, sizeHints, _, err := ParseTags(`ssz-bitsize:"32"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Fatalf("expected 1 size hint, got %d", len(sizeHints))
		}
		if !sizeHints[0].Bits {
			t.Fatal("expected Bits=true")
		}
		if sizeHints[0].Size != 32 {
			t.Fatalf("expected Size=32, got %d", sizeHints[0].Size)
		}
	})

	t.Run("SszBitsizeLarger", func(t *testing.T) {
		_, sizeHints, _, err := ParseTags(`ssz-bitsize:"128"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sizeHints) != 1 || sizeHints[0].Size != 128 || !sizeHints[0].Bits {
			t.Fatalf("unexpected size hints: %+v", sizeHints)
		}
	})

	t.Run("SszBitsizeInvalid", func(t *testing.T) {
		_, _, _, err := ParseTags(`ssz-bitsize:"abc"`)
		if err == nil {
			t.Fatal("expected error for invalid ssz-bitsize")
		}
		if !strings.Contains(err.Error(), "error parsing ssz-size tag") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("NoSizeNoBitsize_Dynamic", func(t *testing.T) {
		// Both ssz-size and ssz-bitsize absent but at least one dimension requested
		// through an ssz-size:"?" tag
		_, sizeHints, _, err := ParseTags(`ssz-size:"?"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sizeHints) != 1 || !sizeHints[0].Dynamic {
			t.Fatalf("expected Dynamic=true, got %+v", sizeHints)
		}
	})

	t.Run("DynSszBitsize", func(t *testing.T) {
		_, sizeHints, _, err := ParseTags(`ssz-bitsize:"32" dynssz-bitsize:"64"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Fatalf("expected 1 size hint, got %d", len(sizeHints))
		}
		// dynssz-bitsize:"64" overrides ssz-bitsize:"32"
		if sizeHints[0].Size != 64 {
			t.Fatalf("expected Size=64 from dynssz-bitsize override, got %d", sizeHints[0].Size)
		}
		if !sizeHints[0].Bits {
			t.Fatal("expected Bits=true from dynssz-bitsize")
		}
	})

	t.Run("DynSszBitsizeOverrideSszBitsize", func(t *testing.T) {
		_, sizeHints, _, err := ParseTags(`ssz-bitsize:"64" dynssz-bitsize:"128"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sizeHints) != 1 || sizeHints[0].Size != 128 || !sizeHints[0].Bits {
			t.Fatalf("expected Size=128 Bits=true, got %+v", sizeHints[0])
		}
	})

	t.Run("DynSszSizeDynamic", func(t *testing.T) {
		_, sizeHints, _, err := ParseTags(`ssz-size:"32" dynssz-size:"?"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Fatalf("expected 1 size hint, got %d", len(sizeHints))
		}
		if !sizeHints[0].Dynamic {
			t.Fatal("expected Dynamic=true for dynssz-size:\"?\"")
		}
	})

	t.Run("DynSszSizeNumericOverride", func(t *testing.T) {
		_, sizeHints, _, err := ParseTags(`ssz-size:"32" dynssz-size:"64"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sizeHints) != 1 || sizeHints[0].Size != 64 {
			t.Fatalf("expected Size=64, got %+v", sizeHints)
		}
	})

	t.Run("DynSszSizeExprWithExistingHint", func(t *testing.T) {
		// Expression that can't be parsed as numeric, sets Expr on existing hint
		_, sizeHints, _, err := ParseTags(`ssz-size:"32" dynssz-size:"SOME_EXPR"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Fatalf("expected 1 size hint, got %d", len(sizeHints))
		}
		if sizeHints[0].Expr != "SOME_EXPR" {
			t.Fatalf("expected Expr='SOME_EXPR', got %q", sizeHints[0].Expr)
		}
	})

	t.Run("DynSszSizeExprWithoutExistingHint", func(t *testing.T) {
		// Expression without ssz-size base - appends new hint
		_, sizeHints, _, err := ParseTags(`dynssz-size:"SOME_EXPR"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Fatalf("expected 1 size hint, got %d", len(sizeHints))
		}
		if !sizeHints[0].Dynamic {
			t.Fatal("expected Dynamic=true")
		}
		if !sizeHints[0].Custom {
			t.Fatal("expected Custom=true")
		}
	})

	t.Run("SszMaxDynamic", func(t *testing.T) {
		_, _, maxHints, err := ParseTags(`ssz-max:"?"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(maxHints) != 1 || !maxHints[0].NoValue {
			t.Fatalf("expected NoValue=true, got %+v", maxHints)
		}
	})

	t.Run("SszMaxParseError", func(t *testing.T) {
		_, _, _, err := ParseTags(`ssz-max:"abc"`)
		if err == nil {
			t.Fatal("expected error for invalid ssz-max")
		}
		if !strings.Contains(err.Error(), "error parsing ssz-max tag") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("DynSszMaxDynamic", func(t *testing.T) {
		_, _, maxHints, err := ParseTags(`ssz-max:"100" dynssz-max:"?"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(maxHints) != 1 || !maxHints[0].NoValue {
			t.Fatalf("expected NoValue=true, got %+v", maxHints)
		}
	})

	t.Run("DynSszSizeExprSetsExprOnExistingSizeHint", func(t *testing.T) {
		// When i < len(sizeHints) and expr is not numeric, it sets Expr on existing
		// hint and continues (lines 557-560 in ssztags.go ParseTags)
		_, sizeHints, _, err := ParseTags(`ssz-size:"32" dynssz-size:"MY_EXPR"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Fatalf("expected 1 size hint, got %d", len(sizeHints))
		}
		if sizeHints[0].Expr != "MY_EXPR" {
			t.Fatalf("expected Expr='MY_EXPR', got %q", sizeHints[0].Expr)
		}
		// The size should remain from the original ssz-size since expr isn't resolved
		if sizeHints[0].Size != 32 {
			t.Fatalf("expected Size=32 (original), got %d", sizeHints[0].Size)
		}
	})

	t.Run("DynSszSizeExprNewHint", func(t *testing.T) {
		// When i >= len(sizeHints), it appends a new hint with Dynamic+Custom flags
		// (lines 564-568 in ssztags.go ParseTags)
		_, sizeHints, _, err := ParseTags(`dynssz-size:"MY_NEW_EXPR"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Fatalf("expected 1 size hint, got %d", len(sizeHints))
		}
		if !sizeHints[0].Dynamic || !sizeHints[0].Custom {
			t.Fatal("expected Dynamic=true and Custom=true for new expr hint")
		}
	})

	t.Run("DynSszMaxExprWithExistingHint", func(t *testing.T) {
		// Expression-based dynssz-max with existing ssz-max (lines 612-615)
		_, _, maxHints, err := ParseTags(`ssz-max:"100" dynssz-max:"SOME_MAX_EXPR"`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(maxHints) != 1 {
			t.Fatalf("expected 1 max hint, got %d", len(maxHints))
		}
		if maxHints[0].Expr != "SOME_MAX_EXPR" {
			t.Fatalf("expected Expr='SOME_MAX_EXPR', got %q", maxHints[0].Expr)
		}
	})

	t.Run("SszSizeParseError", func(t *testing.T) {
		_, _, _, err := ParseTags(`ssz-size:"notanumber"`)
		if err == nil {
			t.Fatal("expected error for invalid ssz-size")
		}
		if !strings.Contains(err.Error(), "error parsing ssz-size tag") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// --- View descriptor runtime/schema type mismatch tests ---

func TestTypeCache_ViewDescriptorKindMismatch(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	t.Run("RuntimeSchemaKindMismatch", func(t *testing.T) {
		// runtime is struct, schema is uint32 => kind mismatch
		runtimeType := reflect.TypeOf(struct{ A uint32 }{})
		schemaType := reflect.TypeOf(uint32(0))

		_, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaType, nil, nil, nil)
		if err == nil {
			t.Fatal("expected error for kind mismatch")
		}
		if !strings.Contains(err.Error(), "incompatible types") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("PointerKindMismatch", func(t *testing.T) {
		// Both are pointers but underlying types have different kinds
		runtimeType := reflect.TypeOf((*uint32)(nil))
		schemaType := reflect.TypeOf((*uint64)(nil))

		_, err := cache.GetTypeDescriptorWithSchema(runtimeType, schemaType, nil, nil, nil)
		if err == nil {
			t.Fatal("expected error for pointer kind mismatch")
		}
		if !strings.Contains(err.Error(), "incompatible pointer types") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// --- FastSSZ interface compatibility tests ---

// testFastsszMarshaler implements fastssz marshaler + unmarshaler + hash root
type testFastsszMarshaler struct {
	Value uint32
}

func (t *testFastsszMarshaler) MarshalSSZTo(dst []byte) ([]byte, error) {
	return append(dst, 0, 0, 0, 0), nil
}
func (t *testFastsszMarshaler) MarshalSSZ() ([]byte, error) {
	return []byte{0, 0, 0, 0}, nil
}
func (t *testFastsszMarshaler) SizeSSZ() int { return 4 }
func (t *testFastsszMarshaler) UnmarshalSSZ(buf []byte) error {
	return nil
}
func (t *testFastsszMarshaler) HashTreeRoot() ([32]byte, error) {
	return [32]byte{}, nil
}

// testHashTreeRootWith implements HashTreeRootWith(hasher) error
type testHashTreeRootWith struct{}

func (t *testHashTreeRootWith) HashTreeRootWith(hh interface{}) error {
	return nil
}

func TestTypeCache_FastSSZInterfaceCompat(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	t.Run("FastSSZMarshaler", func(t *testing.T) {
		desc, err := cache.GetTypeDescriptor(
			reflect.TypeOf(testFastsszMarshaler{}), nil, nil, nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszCompatFlags&SszCompatFlagFastSSZMarshaler == 0 {
			t.Error("expected SszCompatFlagFastSSZMarshaler to be set")
		}
		if desc.SszCompatFlags&SszCompatFlagFastSSZHasher == 0 {
			t.Error("expected SszCompatFlagFastSSZHasher to be set")
		}
	})

	t.Run("HashTreeRootWithMethod", func(t *testing.T) {
		desc, err := cache.GetTypeDescriptor(
			reflect.TypeOf(testHashTreeRootWith{}), nil, nil, nil,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszCompatFlags&SszCompatFlagHashTreeRootWith == 0 {
			t.Error("expected SszCompatFlagHashTreeRootWith to be set")
		}
		if desc.HashTreeRootWithMethod == nil {
			t.Error("expected HashTreeRootWithMethod to be non-nil")
		}
	})
}

// --- Dynamic interface compatibility tests ---

// testDynamicMarshaler implements DynamicMarshaler
type testDynamicMarshaler struct{}

func (t *testDynamicMarshaler) MarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return buf, nil
}

// testDynamicUnmarshaler implements DynamicUnmarshaler
type testDynamicUnmarshaler struct{}

func (t *testDynamicUnmarshaler) UnmarshalSSZDyn(ds sszutils.DynamicSpecs, buf []byte) error {
	return nil
}

// testDynamicEncoder implements DynamicEncoder
type testDynamicEncoder struct{}

func (t *testDynamicEncoder) MarshalSSZEncoder(ds sszutils.DynamicSpecs, encoder sszutils.Encoder) error {
	return nil
}

// testDynamicDecoder implements DynamicDecoder
type testDynamicDecoder struct{}

func (t *testDynamicDecoder) UnmarshalSSZDecoder(ds sszutils.DynamicSpecs, decoder sszutils.Decoder) error {
	return nil
}

// testDynamicSizer implements DynamicSizer
type testDynamicSizer struct{}

func (t *testDynamicSizer) SizeSSZDyn(ds sszutils.DynamicSpecs) int { return 0 }

// testDynamicHashRoot implements DynamicHashRoot
type testDynamicHashRoot struct{}

func (t *testDynamicHashRoot) HashTreeRootWithDyn(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	return nil
}

func TestTypeCache_DynamicInterfaceCompat(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	t.Run("DynamicMarshaler", func(t *testing.T) {
		desc, err := cache.GetTypeDescriptor(reflect.TypeOf(testDynamicMarshaler{}), nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszCompatFlags&SszCompatFlagDynamicMarshaler == 0 {
			t.Error("expected SszCompatFlagDynamicMarshaler to be set")
		}
	})

	t.Run("DynamicUnmarshaler", func(t *testing.T) {
		desc, err := cache.GetTypeDescriptor(reflect.TypeOf(testDynamicUnmarshaler{}), nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszCompatFlags&SszCompatFlagDynamicUnmarshaler == 0 {
			t.Error("expected SszCompatFlagDynamicUnmarshaler to be set")
		}
	})

	t.Run("DynamicEncoder", func(t *testing.T) {
		desc, err := cache.GetTypeDescriptor(reflect.TypeOf(testDynamicEncoder{}), nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszCompatFlags&SszCompatFlagDynamicEncoder == 0 {
			t.Error("expected SszCompatFlagDynamicEncoder to be set")
		}
	})

	t.Run("DynamicDecoder", func(t *testing.T) {
		desc, err := cache.GetTypeDescriptor(reflect.TypeOf(testDynamicDecoder{}), nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszCompatFlags&SszCompatFlagDynamicDecoder == 0 {
			t.Error("expected SszCompatFlagDynamicDecoder to be set")
		}
	})

	t.Run("DynamicSizer", func(t *testing.T) {
		desc, err := cache.GetTypeDescriptor(reflect.TypeOf(testDynamicSizer{}), nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszCompatFlags&SszCompatFlagDynamicSizer == 0 {
			t.Error("expected SszCompatFlagDynamicSizer to be set")
		}
	})

	t.Run("DynamicHashRoot", func(t *testing.T) {
		desc, err := cache.GetTypeDescriptor(reflect.TypeOf(testDynamicHashRoot{}), nil, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszCompatFlags&SszCompatFlagDynamicHashRoot == 0 {
			t.Error("expected SszCompatFlagDynamicHashRoot to be set")
		}
	})
}

// --- Dynamic View interface compatibility tests ---

// testDynViewMarshaler implements DynamicViewMarshaler
type testDynViewMarshaler struct{}

func (t *testDynViewMarshaler) MarshalSSZDynView(view any) func(ds sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
	return nil
}

// testDynViewUnmarshaler implements DynamicViewUnmarshaler
type testDynViewUnmarshaler struct{}

func (t *testDynViewUnmarshaler) UnmarshalSSZDynView(view any) func(ds sszutils.DynamicSpecs, buf []byte) error {
	return nil
}

// testDynViewEncoder implements DynamicViewEncoder
type testDynViewEncoder struct{}

func (t *testDynViewEncoder) MarshalSSZEncoderView(view any) func(ds sszutils.DynamicSpecs, encoder sszutils.Encoder) error {
	return nil
}

// testDynViewDecoder implements DynamicViewDecoder
type testDynViewDecoder struct{}

func (t *testDynViewDecoder) UnmarshalSSZDecoderView(view any) func(ds sszutils.DynamicSpecs, decoder sszutils.Decoder) error {
	return nil
}

// testDynViewSizer implements DynamicViewSizer
type testDynViewSizer struct{}

func (t *testDynViewSizer) SizeSSZDynView(view any) func(ds sszutils.DynamicSpecs) int {
	return nil
}

// testDynViewHashRoot implements DynamicViewHashRoot
type testDynViewHashRoot struct{}

func (t *testDynViewHashRoot) HashTreeRootWithDynView(view any) func(ds sszutils.DynamicSpecs, hh sszutils.HashWalker) error {
	return nil
}

func TestTypeCache_DynamicViewInterfaceCompat(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	tests := []struct {
		name     string
		typ      reflect.Type
		expected SszCompatFlag
	}{
		{"DynamicViewMarshaler", reflect.TypeOf(testDynViewMarshaler{}), SszCompatFlagDynamicViewMarshaler},
		{"DynamicViewUnmarshaler", reflect.TypeOf(testDynViewUnmarshaler{}), SszCompatFlagDynamicViewUnmarshaler},
		{"DynamicViewEncoder", reflect.TypeOf(testDynViewEncoder{}), SszCompatFlagDynamicViewEncoder},
		{"DynamicViewDecoder", reflect.TypeOf(testDynViewDecoder{}), SszCompatFlagDynamicViewDecoder},
		{"DynamicViewSizer", reflect.TypeOf(testDynViewSizer{}), SszCompatFlagDynamicViewSizer},
		{"DynamicViewHashRoot", reflect.TypeOf(testDynViewHashRoot{}), SszCompatFlagDynamicViewHashRoot},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc, err := cache.GetTypeDescriptor(tt.typ, nil, nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if desc.SszCompatFlags&tt.expected == 0 {
				t.Errorf("expected compat flag %d to be set", tt.expected)
			}
		})
	}
}

// --- time.Time detection test ---

func TestTypeCache_TimeTimeDetection(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	type TestStructWithTime struct {
		Timestamp time.Time
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStructWithTime{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	if field.Type.GoTypeFlags&GoTypeFlagIsTime == 0 {
		t.Error("expected GoTypeFlagIsTime to be set for time.Time field")
	}
	if field.Type.SszType != SszUint64Type {
		t.Errorf("expected SszUint64Type for time.Time, got %v", field.Type.SszType)
	}
}

// --- Progressive container test ---

func TestTypeCache_ProgressiveContainerSuccess(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	type progressiveStruct struct {
		A uint32 `ssz-index:"0"`
		B uint64 `ssz-index:"1"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(progressiveStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.SszType != SszProgressiveContainerType {
		t.Errorf("expected SszProgressiveContainerType, got %v", desc.SszType)
	}
}

// --- TypeWrapper descriptor tests ---

// testWrapperDescriptor is a descriptor struct with one field for TypeWrapper
type testWrapperDescriptor struct {
	Data []byte `ssz-size:"32"`
}

// testTypeWrapper mimics dynssz.TypeWrapper[testWrapperDescriptor, []byte]
type testTypeWrapper struct {
	Data []byte
}

func (w *testTypeWrapper) GetDescriptorType() reflect.Type {
	return reflect.TypeOf(testWrapperDescriptor{})
}

func TestTypeCache_TypeWrapperDescriptor(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	t.Run("TypeWrapperSuccess", func(t *testing.T) {
		desc, err := cache.GetTypeDescriptor(
			reflect.TypeOf(testTypeWrapper{}),
			nil, nil,
			[]SszTypeHint{{Type: SszTypeWrapperType}},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszType != SszTypeWrapperType {
			t.Errorf("expected SszTypeWrapperType, got %v", desc.SszType)
		}
		if desc.ElemDesc == nil {
			t.Fatal("expected ElemDesc to be set")
		}
		if desc.ElemDesc.SszType != SszVectorType {
			t.Errorf("expected wrapped type to be SszVectorType, got %v", desc.ElemDesc.SszType)
		}
		if desc.Size != 32 {
			t.Errorf("expected Size 32, got %d", desc.Size)
		}
	})

	t.Run("TypeWrapperWithDynamicWrapped", func(t *testing.T) {
		desc, err := cache.GetTypeDescriptor(
			reflect.TypeOf(testDynTypeWrapper{}),
			nil, nil,
			[]SszTypeHint{{Type: SszTypeWrapperType}},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszTypeFlags&SszTypeFlagIsDynamic == 0 {
			t.Error("expected SszTypeFlagIsDynamic for dynamic wrapped type")
		}
	})
}

type testDynWrapperDescriptor struct {
	Data []byte `ssz-max:"100"`
}

type testDynTypeWrapper struct {
	Data []byte
}

func (w *testDynTypeWrapper) GetDescriptorType() reflect.Type {
	return reflect.TypeOf(testDynWrapperDescriptor{})
}

// --- TypeWrapper view descriptor tests ---

type testSchemaWrapperDescriptor struct {
	Data []byte `ssz-size:"64"`
}

type testSchemaTypeWrapper struct {
	Data []byte
}

func (w *testSchemaTypeWrapper) GetDescriptorType() reflect.Type {
	return reflect.TypeOf(testSchemaWrapperDescriptor{})
}

func TestTypeCache_TypeWrapperViewDescriptor(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	runtimeType := reflect.TypeOf(testTypeWrapper{})
	schemaType := reflect.TypeOf(testSchemaTypeWrapper{})

	desc, err := cache.GetTypeDescriptorWithSchema(
		runtimeType, schemaType,
		nil, nil,
		[]SszTypeHint{{Type: SszTypeWrapperType}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.SszType != SszTypeWrapperType {
		t.Errorf("expected SszTypeWrapperType, got %v", desc.SszType)
	}
	// Schema has ssz-size:"64", so the wrapper should report size 64
	if desc.Size != 64 {
		t.Errorf("expected Size 64 from schema wrapper, got %d", desc.Size)
	}
}

// --- CompatibleUnion descriptor tests ---

// testUnionDescriptor is the descriptor struct for a CompatibleUnion
type testUnionDescriptor struct {
	VariantA uint32
	VariantB uint64
}

// testCompatibleUnion mimics dynssz.CompatibleUnion[testUnionDescriptor]
type testCompatibleUnion struct {
	Variant uint8
	Data    interface{}
}

func (u *testCompatibleUnion) GetDescriptorType() reflect.Type {
	return reflect.TypeOf(testUnionDescriptor{})
}

func TestTypeCache_CompatibleUnionDescriptor(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	t.Run("CompatibleUnionSuccess", func(t *testing.T) {
		desc, err := cache.GetTypeDescriptor(
			reflect.TypeOf(testCompatibleUnion{}),
			nil, nil,
			[]SszTypeHint{{Type: SszCompatibleUnionType}},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if desc.SszType != SszCompatibleUnionType {
			t.Errorf("expected SszCompatibleUnionType, got %v", desc.SszType)
		}
		if desc.SszTypeFlags&SszTypeFlagIsDynamic == 0 {
			t.Error("expected SszTypeFlagIsDynamic for union type")
		}
		if desc.UnionVariants == nil {
			t.Fatal("expected UnionVariants to be set")
		}
		if len(desc.UnionVariants) != 2 {
			t.Fatalf("expected 2 union variants, got %d", len(desc.UnionVariants))
		}
		// Variant 0 should be uint32
		if desc.UnionVariants[0].SszType != SszUint32Type {
			t.Errorf("variant 0: expected SszUint32Type, got %v", desc.UnionVariants[0].SszType)
		}
		// Variant 1 should be uint64
		if desc.UnionVariants[1].SszType != SszUint64Type {
			t.Errorf("variant 1: expected SszUint64Type, got %v", desc.UnionVariants[1].SszType)
		}
	})
}

// --- CompatibleUnion view descriptor tests ---

type testSchemaUnionDescriptor struct {
	VariantB uint64
}

type testSchemaCompatibleUnion struct {
	Variant uint8
	Data    interface{}
}

func (u *testSchemaCompatibleUnion) GetDescriptorType() reflect.Type {
	return reflect.TypeOf(testSchemaUnionDescriptor{})
}

func TestTypeCache_CompatibleUnionViewDescriptor(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	runtimeType := reflect.TypeOf(testCompatibleUnion{})
	schemaType := reflect.TypeOf(testSchemaCompatibleUnion{})

	desc, err := cache.GetTypeDescriptorWithSchema(
		runtimeType, schemaType,
		nil, nil,
		[]SszTypeHint{{Type: SszCompatibleUnionType}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.SszType != SszCompatibleUnionType {
		t.Errorf("expected SszCompatibleUnionType, got %v", desc.SszType)
	}
	// Schema only has VariantB (uint64)
	if len(desc.UnionVariants) != 1 {
		t.Fatalf("expected 1 union variant from schema, got %d", len(desc.UnionVariants))
	}
	if desc.UnionVariants[0].SszType != SszUint64Type {
		t.Errorf("variant 0: expected SszUint64Type, got %v", desc.UnionVariants[0].SszType)
	}
}

// --- CompatibleUnion missing variant in runtime ---

type testRuntimeUnionMissingVariant struct {
	Variant uint8
	Data    interface{}
}

type testRuntimeUnionMissingDescriptor struct {
	OnlyA uint32
}

func (u *testRuntimeUnionMissingVariant) GetDescriptorType() reflect.Type {
	return reflect.TypeOf(testRuntimeUnionMissingDescriptor{})
}

func TestTypeCache_CompatibleUnionMissingVariant(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Schema has VariantB but runtime only has OnlyA
	runtimeType := reflect.TypeOf(testRuntimeUnionMissingVariant{})
	schemaType := reflect.TypeOf(testCompatibleUnion{})

	_, err := cache.GetTypeDescriptorWithSchema(
		runtimeType, schemaType,
		nil, nil,
		[]SszTypeHint{{Type: SszCompatibleUnionType}},
	)
	if err == nil {
		t.Fatal("expected error for missing runtime variant")
	}
	if !strings.Contains(err.Error(), "runtime union missing variant") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- extractGenericTypeParameter tests (tested indirectly) ---

// testNoGetDescriptorType has no GetDescriptorType method
type testNoGetDescriptorType struct{}

func TestTypeCache_ExtractGenericTypeParameterNoMethod(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	_, err := cache.GetTypeDescriptor(
		reflect.TypeOf(testNoGetDescriptorType{}),
		nil, nil,
		[]SszTypeHint{{Type: SszCompatibleUnionType}},
	)
	if err == nil {
		t.Fatal("expected error for missing GetDescriptorType method")
	}
	if !strings.Contains(err.Error(), "GetDescriptorType method not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Nested hint extraction tests ---

func TestTypeCache_VectorWithNestedSizeHints(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Vector with nested size hints: [][]byte with ssz-size:"3,32"
	type TestStruct struct {
		Field [][]byte `ssz-size:"3,32"`
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
	if field.Type.Len != 3 {
		t.Errorf("expected Len 3, got %d", field.Type.Len)
	}
	// Element should also be a vector with size 32
	if field.Type.ElemDesc == nil {
		t.Fatal("expected element descriptor")
	}
	if field.Type.ElemDesc.SszType != SszVectorType {
		t.Errorf("expected element SszVectorType, got %v", field.Type.ElemDesc.SszType)
	}
	if field.Type.ElemDesc.Len != 32 {
		t.Errorf("expected element Len 32, got %d", field.Type.ElemDesc.Len)
	}
}

func TestTypeCache_ListWithNestedSizeHints(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// List with nested size hints: [][]byte with ssz-max:"10" ssz-size:"?,32"
	type TestStruct struct {
		Field [][]byte `ssz-max:"10" ssz-size:"?,32"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	if field.Type.SszType != SszListType {
		t.Errorf("expected SszListType, got %v", field.Type.SszType)
	}
	// Element should be a vector with size 32
	if field.Type.ElemDesc == nil {
		t.Fatal("expected element descriptor")
	}
	if field.Type.ElemDesc.SszType != SszVectorType {
		t.Errorf("expected element SszVectorType, got %v", field.Type.ElemDesc.SszType)
	}
	if field.Type.ElemDesc.Len != 32 {
		t.Errorf("expected element Len 32, got %d", field.Type.ElemDesc.Len)
	}
}

// --- Optional descriptor elem error test ---

func TestTypeCache_OptionalElemError(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)
	cache.ExtendedTypes = true

	// Optional with bad element type hints should propagate error
	_, err := cache.GetTypeDescriptor(
		reflect.TypeOf((*uint16)(nil)),
		[]SszSizeHint{{Size: 0}, {Size: 999}}, // child size hint 999 is wrong for uint16
		nil,
		[]SszTypeHint{{Type: SszOptionalType}},
	)
	if err == nil {
		t.Fatal("expected error for optional with invalid elem hints")
	}
}

// --- Vector elem descriptor error test ---

func TestTypeCache_VectorElemError(t *testing.T) {
	// Create a type whose elem descriptor will fail
	type badElemVec uint32
	sszutils.Annotate[badElemVec](`ssz-size:"notanumber"`)

	cache := NewTypeCache(&dummyDynamicSpecs{})

	type TestStruct struct {
		Field []badElemVec `ssz-size:"3"`
	}

	_, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error from bad element annotation in vector")
	}
}

// --- List elem descriptor error test ---

func TestTypeCache_ListElemError(t *testing.T) {
	type badElemList uint32
	sszutils.Annotate[badElemList](`ssz-size:"notanumber"`)

	cache := NewTypeCache(&dummyDynamicSpecs{})

	type TestStruct struct {
		Field []badElemList `ssz-max:"10"`
	}

	_, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err == nil {
		t.Fatal("expected error from bad element annotation in list")
	}
}

// --- getSszSizeTag/getSszMaxSizeTag fallback path tests ---

func TestGetSszSizeTagDynSszBitsize(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	// dynssz-bitsize:"64" as a numeric bitsize override
	field := makeField("BitField", reflect.TypeOf([]byte{}), `ssz-bitsize:"32" dynssz-bitsize:"64"`)

	sizes, err := getSszSizeTag(ds, field)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sizes) != 1 {
		t.Fatalf("expected 1 size hint, got %d", len(sizes))
	}
	if sizes[0].Size != 64 {
		t.Fatalf("expected Size 64 from dynssz-bitsize override, got %d", sizes[0].Size)
	}
	if !sizes[0].Bits {
		t.Fatal("expected Bits=true")
	}
}

func TestGetSszSizeTagUnknownSpecValue(t *testing.T) {
	// DynamicSpecs that returns ok=false (unknown spec value) with no error
	ds := &dummyDynamicSpecs{specValues: map[string]uint64{}} // empty - won't resolve anything

	// dynssz-size with expression that resolves ok=false => fallback to fastssz defaults
	field := makeField("UnknownSpec", reflect.TypeOf([]byte{}), `ssz-size:"32" dynssz-size:"UNKNOWN_SPEC"`)

	sizes, err := getSszSizeTag(ds, field)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sizes) != 1 {
		t.Fatalf("expected 1 size hint, got %d", len(sizes))
	}
	// Should fallback to original size and set Expr
	if sizes[0].Size != 32 {
		t.Fatalf("expected Size=32 (fallback), got %d", sizes[0].Size)
	}
	if sizes[0].Expr != "UNKNOWN_SPEC" {
		t.Fatalf("expected Expr='UNKNOWN_SPEC', got %q", sizes[0].Expr)
	}
}

func TestGetSszMaxSizeTagDynamic(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	// ssz-max:"?" should set NoValue=true
	field := makeField("DynMax", reflect.TypeOf([]byte{}), `ssz-max:"?"`)

	maxSizes, err := getSszMaxSizeTag(ds, field)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(maxSizes) != 1 {
		t.Fatalf("expected 1 max size hint, got %d", len(maxSizes))
	}
	if !maxSizes[0].NoValue {
		t.Fatal("expected NoValue=true for ssz-max:\"?\"")
	}
}

func TestGetSszMaxSizeTagUnknownSpecValue(t *testing.T) {
	ds := &dummyDynamicSpecs{specValues: map[string]uint64{}} // empty

	// dynssz-max with expression that resolves ok=false => fallback
	field := makeField("UnknownMax", reflect.TypeOf([]byte{}), `ssz-max:"100" dynssz-max:"UNKNOWN_MAX"`)

	maxSizes, err := getSszMaxSizeTag(ds, field)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(maxSizes) != 1 {
		t.Fatalf("expected 1 max size hint, got %d", len(maxSizes))
	}
	// Should fallback to original max size and set Expr
	if maxSizes[0].Size != 100 {
		t.Fatalf("expected Size=100 (fallback), got %d", maxSizes[0].Size)
	}
	if maxSizes[0].Expr != "UNKNOWN_MAX" {
		t.Fatalf("expected Expr='UNKNOWN_MAX', got %q", maxSizes[0].Expr)
	}
}

// --- List with nested max size hints ---

func TestTypeCache_ListWithNestedMaxSizeHints(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	type TestStruct struct {
		Field [][]byte `ssz-max:"10,32"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	if field.Type.SszType != SszListType {
		t.Errorf("expected SszListType, got %v", field.Type.SszType)
	}
	if field.Type.Limit != 10 {
		t.Errorf("expected Limit 10, got %d", field.Type.Limit)
	}
}

// --- BuildUintDescriptor success path ---

func TestTypeCache_BuildUintDescriptorSuccess(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// uint128 with correct array size
	desc, err := cache.GetTypeDescriptor(
		reflect.TypeOf([16]uint8{}),
		nil, nil,
		[]SszTypeHint{{Type: SszUint128Type}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.Size != 16 {
		t.Errorf("expected Size 16, got %d", desc.Size)
	}

	// uint128 with uint64 array elements
	desc, err = cache.GetTypeDescriptor(
		reflect.TypeOf([2]uint64{}),
		nil, nil,
		[]SszTypeHint{{Type: SszUint128Type}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.Size != 16 {
		t.Errorf("expected Size 16, got %d", desc.Size)
	}
	if desc.GoTypeFlags&GoTypeFlagIsByteArray != 0 {
		t.Error("expected GoTypeFlagIsByteArray to NOT be set for uint64 array")
	}

	// uint256 with correct array size
	desc, err = cache.GetTypeDescriptor(
		reflect.TypeOf([32]uint8{}),
		nil, nil,
		[]SszTypeHint{{Type: SszUint256Type}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.Size != 32 {
		t.Errorf("expected Size 32, got %d", desc.Size)
	}
}

// --- CustomType with size hint ---

// testCustomWithSize implements fastssz marshaler + unmarshaler + hasher
type testCustomWithSize struct{}

func (t *testCustomWithSize) MarshalSSZTo(dst []byte) ([]byte, error) {
	return append(dst, make([]byte, 8)...), nil
}
func (t *testCustomWithSize) MarshalSSZ() ([]byte, error) { return make([]byte, 8), nil }
func (t *testCustomWithSize) SizeSSZ() int                { return 8 }
func (t *testCustomWithSize) UnmarshalSSZ(buf []byte) error {
	return nil
}
func (t *testCustomWithSize) HashTreeRoot() ([32]byte, error) {
	return [32]byte{}, nil
}

func TestTypeCache_CustomTypeWithSizeHint(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Custom type with size hint - covers line 512-513
	desc, err := cache.GetTypeDescriptor(
		reflect.TypeOf(testCustomWithSize{}),
		[]SszSizeHint{{Size: 8}}, nil,
		[]SszTypeHint{{Type: SszCustomType}},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.Size != 8 {
		t.Errorf("expected Size 8, got %d", desc.Size)
	}
}

// --- TypeWrapper error paths in view descriptor mode ---

// testWrapperBadDescriptor returns a descriptor type that is not a struct
type testWrapperBadDescriptor struct {
	Data []byte
}

func (w *testWrapperBadDescriptor) GetDescriptorType() reflect.Type {
	// Return a non-struct type -> extractWrapperDescriptorInfo will fail
	return reflect.TypeOf(uint32(0))
}

func TestTypeCache_TypeWrapperSchemaDescriptorError(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	// Schema wrapper returns non-struct descriptor -> error in extractWrapperDescriptorInfo
	_, err := cache.GetTypeDescriptor(
		reflect.TypeOf(testWrapperBadDescriptor{}),
		nil, nil,
		[]SszTypeHint{{Type: SszTypeWrapperType}},
	)
	if err == nil {
		t.Fatal("expected error for bad schema descriptor type")
	}
	if !strings.Contains(err.Error(), "wrapper descriptor must be a struct") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTypeCache_TypeWrapperViewRuntimeBadDescriptor(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	// Runtime wrapper returns a non-struct descriptor type
	_, err := cache.GetTypeDescriptorWithSchema(
		reflect.TypeOf(testWrapperBadDescriptor{}),
		reflect.TypeOf(testTypeWrapper{}),
		nil, nil,
		[]SszTypeHint{{Type: SszTypeWrapperType}},
	)
	if err == nil {
		t.Fatal("expected error for bad runtime descriptor type")
	}
	if !strings.Contains(err.Error(), "wrapper descriptor must be a struct") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// testWrapperBadWrappedType has a descriptor whose wrapped type will fail
type testWrapperBadWrappedDescriptor struct {
	Data []byte `ssz-size:"notanumber"`
}

type testWrapperWithBadWrapped struct {
	Data []byte
}

func (w *testWrapperWithBadWrapped) GetDescriptorType() reflect.Type {
	return reflect.TypeOf(testWrapperBadWrappedDescriptor{})
}

func TestTypeCache_TypeWrapperWrappedDescBuildError(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	// Wrapped type descriptor build will fail due to invalid ssz-size tag
	_, err := cache.GetTypeDescriptor(
		reflect.TypeOf(testWrapperWithBadWrapped{}),
		nil, nil,
		[]SszTypeHint{{Type: SszTypeWrapperType}},
	)
	if err == nil {
		t.Fatal("expected error from bad wrapped type descriptor")
	}
	if !strings.Contains(err.Error(), "(wrapper)") {
		t.Fatalf("expected error path to contain '(wrapper)', got: %v", err)
	}
}

// --- TypeWrapper view: GetDescriptorType returns no results / wrong type ---

type testWrapperNoReturnSchema struct{}

func (t *testWrapperNoReturnSchema) GetDescriptorType() {}

type testWrapperWrongReturnSchema struct{}

func (t *testWrapperWrongReturnSchema) GetDescriptorType() string { return "bad" }

func TestTypeCache_TypeWrapperSchemaNoResults(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	_, err := cache.GetTypeDescriptor(
		reflect.TypeOf(testWrapperNoReturnSchema{}),
		nil, nil,
		[]SszTypeHint{{Type: SszTypeWrapperType}},
	)
	if err == nil {
		t.Fatal("expected error for GetDescriptorType with no results")
	}
	if !strings.Contains(err.Error(), "GetDescriptorType returned no results") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTypeCache_TypeWrapperSchemaWrongReturnType(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	_, err := cache.GetTypeDescriptor(
		reflect.TypeOf(testWrapperWrongReturnSchema{}),
		nil, nil,
		[]SszTypeHint{{Type: SszTypeWrapperType}},
	)
	if err == nil {
		t.Fatal("expected error for GetDescriptorType returning wrong type")
	}
	if !strings.Contains(err.Error(), "GetDescriptorType did not return a reflect.Type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// View wrapper runtime no results / wrong return type
func TestTypeCache_TypeWrapperViewRuntimeNoResults(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	_, err := cache.GetTypeDescriptorWithSchema(
		reflect.TypeOf(testWrapperNoReturnSchema{}),
		reflect.TypeOf(testTypeWrapper{}),
		nil, nil,
		[]SszTypeHint{{Type: SszTypeWrapperType}},
	)
	if err == nil {
		t.Fatal("expected error for runtime GetDescriptorType with no results")
	}
	if !strings.Contains(err.Error(), "GetDescriptorType returned no results for runtime") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTypeCache_TypeWrapperViewRuntimeWrongReturnType(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	_, err := cache.GetTypeDescriptorWithSchema(
		reflect.TypeOf(testWrapperWrongReturnSchema{}),
		reflect.TypeOf(testTypeWrapper{}),
		nil, nil,
		[]SszTypeHint{{Type: SszTypeWrapperType}},
	)
	if err == nil {
		t.Fatal("expected error for runtime GetDescriptorType returning wrong type")
	}
	if !strings.Contains(err.Error(), "GetDescriptorType did not return a reflect.Type for runtime") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Test view wrapper where the wrapped type descriptor build fails (line 737)
// Schema wrapper describes a map type (unsupported), runtime wrapper describes []byte.
type testSchemaWrapperBadWrappedDescriptor struct {
	Data map[string]int
}

type testSchemaWrapperBadWrapped struct {
	Data []byte
}

func (w *testSchemaWrapperBadWrapped) GetDescriptorType() reflect.Type {
	return reflect.TypeOf(testSchemaWrapperBadWrappedDescriptor{})
}

func TestTypeCache_TypeWrapperViewWrappedBuildError(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	// Schema wrapper wraps a map type which will fail
	_, err := cache.GetTypeDescriptorWithSchema(
		reflect.TypeOf(testTypeWrapper{}),
		reflect.TypeOf(testSchemaWrapperBadWrapped{}),
		nil, nil,
		[]SszTypeHint{{Type: SszTypeWrapperType}},
	)
	if err == nil {
		t.Fatal("expected error from bad wrapped type in view descriptor")
	}
	if !strings.Contains(err.Error(), "(wrapper)") {
		t.Fatalf("expected error path to contain '(wrapper)', got: %v", err)
	}
}

// --- CompatibleUnion error paths ---

func TestTypeCache_CompatibleUnionSchemaExtractError(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	// Schema union returns non-struct descriptor (will fail in extractUnionDescriptorInfo)
	_, err := cache.GetTypeDescriptor(
		reflect.TypeOf(testWrapperBadDescriptor{}), // returns uint32 from GetDescriptorType
		nil, nil,
		[]SszTypeHint{{Type: SszCompatibleUnionType}},
	)
	if err == nil {
		t.Fatal("expected error for bad union schema descriptor")
	}
	if !strings.Contains(err.Error(), "union descriptor must be a struct") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// testUnionBadVariantDescriptor returns a descriptor where a variant has bad annotations
// that will fail at extractUnionDescriptorInfo level
type testUnionBadVariantDescriptor struct {
	Bad []byte `ssz-size:"notanumber"`
}

type testUnionWithBadVariant struct {
	Variant uint8
	Data    interface{}
}

func (u *testUnionWithBadVariant) GetDescriptorType() reflect.Type {
	return reflect.TypeOf(testUnionBadVariantDescriptor{})
}

func TestTypeCache_CompatibleUnionVariantAnnotationError(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	_, err := cache.GetTypeDescriptor(
		reflect.TypeOf(testUnionWithBadVariant{}),
		nil, nil,
		[]SszTypeHint{{Type: SszCompatibleUnionType}},
	)
	if err == nil {
		t.Fatal("expected error from bad union variant annotation")
	}
	if !strings.Contains(err.Error(), "(union)") {
		t.Fatalf("expected error path to contain '(union)', got: %v", err)
	}
}

// testUnionVariantBuildFailDescriptor has valid tags but the variant type itself
// will fail during descriptor building (maps are unsupported SSZ types)
type testUnionVariantBuildFailDescriptor struct {
	Bad map[string]int
}

type testUnionWithBuildFailVariant struct {
	Variant uint8
	Data    interface{}
}

func (u *testUnionWithBuildFailVariant) GetDescriptorType() reflect.Type {
	return reflect.TypeOf(testUnionVariantBuildFailDescriptor{})
}

func TestTypeCache_CompatibleUnionVariantBuildError(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	_, err := cache.GetTypeDescriptor(
		reflect.TypeOf(testUnionWithBuildFailVariant{}),
		nil, nil,
		[]SszTypeHint{{Type: SszCompatibleUnionType}},
	)
	if err == nil {
		t.Fatal("expected error from bad union variant type")
	}
	if !strings.Contains(err.Error(), "variant") {
		t.Fatalf("expected error path to contain 'variant', got: %v", err)
	}
}

// CompatibleUnion view descriptor: runtime extractGenericTypeParameter error
func TestTypeCache_CompatibleUnionViewRuntimeExtractError(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	// Runtime has no GetDescriptorType, schema has it
	_, err := cache.GetTypeDescriptorWithSchema(
		reflect.TypeOf(testNoGetDescriptorType{}),
		reflect.TypeOf(testCompatibleUnion{}),
		nil, nil,
		[]SszTypeHint{{Type: SszCompatibleUnionType}},
	)
	if err == nil {
		t.Fatal("expected error for missing runtime GetDescriptorType")
	}
	if !strings.Contains(err.Error(), "GetDescriptorType method not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// CompatibleUnion view descriptor: runtime extractUnionDescriptorInfo error
func TestTypeCache_CompatibleUnionViewRuntimeUnionInfoError(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	// Runtime returns non-struct from GetDescriptorType
	_, err := cache.GetTypeDescriptorWithSchema(
		reflect.TypeOf(testWrapperBadDescriptor{}), // returns uint32
		reflect.TypeOf(testCompatibleUnion{}),
		nil, nil,
		[]SszTypeHint{{Type: SszCompatibleUnionType}},
	)
	if err == nil {
		t.Fatal("expected error for bad runtime union descriptor info")
	}
	if !strings.Contains(err.Error(), "union descriptor must be a struct") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- extractGenericTypeParameter error paths ---

func TestTypeCache_ExtractGenericTypeParameterNoResults(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	// Use testWrapperNoReturn (GetDescriptorType returns nothing)
	_, err := cache.GetTypeDescriptor(
		reflect.TypeOf(testWrapperNoReturn{}),
		nil, nil,
		[]SszTypeHint{{Type: SszCompatibleUnionType}},
	)
	if err == nil {
		t.Fatal("expected error for GetDescriptorType returning no results")
	}
	if !strings.Contains(err.Error(), "GetDescriptorType returned no results") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTypeCache_ExtractGenericTypeParameterWrongType(t *testing.T) {
	cache := NewTypeCache(&dummyDynamicSpecs{})

	// Use testWrapperWrongReturn (GetDescriptorType returns string)
	_, err := cache.GetTypeDescriptor(
		reflect.TypeOf(testWrapperWrongReturn{}),
		nil, nil,
		[]SszTypeHint{{Type: SszCompatibleUnionType}},
	)
	if err == nil {
		t.Fatal("expected error for GetDescriptorType returning non-Type")
	}
	if !strings.Contains(err.Error(), "GetDescriptorType did not return a reflect.Type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Vector/List with multiple type/max hints (nested forwarding) ---

func TestTypeCache_VectorWithNestedMaxAndTypeHints(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// Vector of vectors: [][]uint32 with ssz-size:"3" and
	// nested maxSizeHints and typeHints forwarded to child
	type TestStruct struct {
		Field [][]uint32 `ssz-size:"3" ssz-max:"?,10" ssz-type:"?,list"`
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
	// Element should be a list type
	if field.Type.ElemDesc == nil {
		t.Fatal("expected element descriptor")
	}
	if field.Type.ElemDesc.SszType != SszListType {
		t.Errorf("expected element SszListType, got %v", field.Type.ElemDesc.SszType)
	}
}

func TestTypeCache_ListWithNestedTypeHints(t *testing.T) {
	ds := &dummyDynamicSpecs{}
	cache := NewTypeCache(ds)

	// List of vectors: [][]uint32 with ssz-max:"10" and nested type hints
	type TestStruct struct {
		Field [][]uint32 `ssz-max:"10" ssz-size:"?,5" ssz-type:"?,vector"`
	}

	desc, err := cache.GetTypeDescriptor(reflect.TypeOf(TestStruct{}), nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if desc.ContainerDesc == nil || len(desc.ContainerDesc.Fields) == 0 {
		t.Fatal("expected container with fields")
	}

	field := desc.ContainerDesc.Fields[0]
	if field.Type.SszType != SszListType {
		t.Errorf("expected SszListType, got %v", field.Type.SszType)
	}
	// Element should be a vector type with size 5
	if field.Type.ElemDesc == nil {
		t.Fatal("expected element descriptor")
	}
	if field.Type.ElemDesc.SszType != SszVectorType {
		t.Errorf("expected element SszVectorType, got %v", field.Type.ElemDesc.SszType)
	}
	if field.Type.ElemDesc.Len != 5 {
		t.Errorf("expected element Len 5, got %d", field.Type.ElemDesc.Len)
	}
}

// --- Suppress the unused import error for errors package ---
var _ = errors.New
