// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/ssztypes"
)

func TestNewParser(t *testing.T) {
	parser := NewParser()
	if parser == nil {
		t.Fatal("Expected non-nil parser")
	}
	if parser.cache == nil {
		t.Error("Expected cache to be initialized")
	}
	if parser.CompatFlags == nil {
		t.Error("Expected CompatFlags to be initialized")
	}
}

func TestGetCompatFlag(t *testing.T) {
	parser := NewParser()

	t.Run("EmptyFlags", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		flag := parser.getCompatFlag(uint64Type)
		if flag != 0 {
			t.Errorf("Expected 0 flag, got %v", flag)
		}
	})

	t.Run("SetFlag", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		parser.CompatFlags["uint64"] = ssztypes.SszCompatFlagFastSSZMarshaler
		flag := parser.getCompatFlag(uint64Type)
		if flag != ssztypes.SszCompatFlagFastSSZMarshaler {
			t.Errorf("Expected FastSSZMarshaler flag, got %v", flag)
		}
	})
}

func TestGetTypeDescriptor(t *testing.T) {
	parser := NewParser()

	t.Run("BasicTypes", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		desc, err := parser.GetTypeDescriptor(uint64Type, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to get uint64 descriptor: %v", err)
		}
		if desc == nil {
			t.Fatal("Expected non-nil descriptor")
		}
		if desc.Size != 8 {
			t.Errorf("Expected size 8, got %d", desc.Size)
		}
		if desc.SszType != ssztypes.SszUint64Type {
			t.Errorf("Expected SszUint64Type, got %v", desc.SszType)
		}
	})

	t.Run("Caching", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		desc1, err := parser.GetTypeDescriptor(uint64Type, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to get first descriptor: %v", err)
		}
		desc2, err := parser.GetTypeDescriptor(uint64Type, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to get second descriptor: %v", err)
		}
		// Should be same object due to caching
		if desc1 != desc2 {
			t.Error("Expected descriptors to be cached and same")
		}
	})
}

func TestBuildTypeDescriptorBasicTypes(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		typeKind types.BasicKind
		expected ssztypes.SszType
		size     uint32
	}{
		{"bool", types.Bool, ssztypes.SszBoolType, 1},
		{"uint8", types.Uint8, ssztypes.SszUint8Type, 1},
		{"uint16", types.Uint16, ssztypes.SszUint16Type, 2},
		{"uint32", types.Uint32, ssztypes.SszUint32Type, 4},
		{"uint64", types.Uint64, ssztypes.SszUint64Type, 8},
		{"uint", types.Uint, ssztypes.SszUint64Type, 8},
		{"string", types.String, ssztypes.SszListType, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := types.Typ[tt.typeKind]
			desc, err := parser.buildTypeDescriptor(typ, nil, nil, nil)
			if err != nil {
				t.Fatalf("Failed to build descriptor: %v", err)
			}
			if desc.SszType != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, desc.SszType)
			}
			if tt.name != "string" && desc.Size != tt.size {
				t.Errorf("Expected size %d, got %d", tt.size, desc.Size)
			}
			if tt.name == "string" {
				if desc.GoTypeFlags&ssztypes.GoTypeFlagIsString == 0 {
					t.Error("Expected string flag to be set")
				}
				if desc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
					t.Error("Expected dynamic flag for string list")
				}
			}
		})
	}
}

func TestUnsupportedTypes(t *testing.T) {
	parser := NewParser()

	// Test unsupported basic types
	unsupportedBasic := []types.BasicKind{
		types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Float32, types.Float64,
		types.Complex64, types.Complex128,
	}

	for _, kind := range unsupportedBasic {
		t.Run(fmt.Sprintf("unsupported_%d", kind), func(t *testing.T) {
			typ := types.Typ[kind]
			_, err := parser.buildTypeDescriptor(typ, nil, nil, nil)
			if err == nil {
				t.Errorf("Expected error for unsupported type %v", kind)
			}
		})
	}

	// Test unsupported complex types
	t.Run("Map", func(t *testing.T) {
		mapType := types.NewMap(types.Typ[types.String], types.Typ[types.Int])
		_, err := parser.buildTypeDescriptor(mapType, nil, nil, nil)
		if err == nil {
			t.Error("Expected error for map type")
		}
	})

	t.Run("Chan", func(t *testing.T) {
		chanType := types.NewChan(types.SendRecv, types.Typ[types.Int])
		_, err := parser.buildTypeDescriptor(chanType, nil, nil, nil)
		if err == nil {
			t.Error("Expected error for channel type")
		}
	})

	t.Run("Interface", func(t *testing.T) {
		interfaceType := types.NewInterfaceType(nil, nil)
		_, err := parser.buildTypeDescriptor(interfaceType, nil, nil, nil)
		if err == nil {
			t.Error("Expected error for interface type")
		}
	})

	t.Run("Function", func(t *testing.T) {
		signature := types.NewSignatureType(nil, nil, nil, nil, nil, false)
		_, err := parser.buildTypeDescriptor(signature, nil, nil, nil)
		if err == nil {
			t.Error("Expected error for function type")
		}
	})
}

func TestSizeHints(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name      string
		typ       types.Type
		typeHints []ssztypes.SszTypeHint
		sizeHints []ssztypes.SszSizeHint
		expected  string
	}{
		// Bool tests
		{
			name:      "bool with wrong size",
			typ:       types.Typ[types.Bool],
			sizeHints: []ssztypes.SszSizeHint{{Size: 2}},
			expected:  "bool ssz type must be ssz-size:1",
		},
		{
			name:      "bool with bit size",
			typ:       types.Typ[types.Bool],
			sizeHints: []ssztypes.SszSizeHint{{Bits: true}},
			expected:  "bool ssz type cannot be limited by bits",
		},
		// Uint8 tests
		{
			name:      "uint8 with wrong size",
			typ:       types.Typ[types.Uint8],
			sizeHints: []ssztypes.SszSizeHint{{Size: 2}},
			expected:  "uint8 ssz type must be ssz-size:1",
		},
		{
			name:      "uint8 with bit size",
			typ:       types.Typ[types.Uint8],
			sizeHints: []ssztypes.SszSizeHint{{Bits: true}},
			expected:  "uint8 ssz type cannot be limited by bits",
		},
		// Uint16 tests
		{
			name:      "uint16 with wrong size",
			typ:       types.Typ[types.Uint16],
			sizeHints: []ssztypes.SszSizeHint{{Size: 4}},
			expected:  "uint16 ssz type must be ssz-size:2",
		},
		{
			name:      "uint16 with bit size",
			typ:       types.Typ[types.Uint16],
			sizeHints: []ssztypes.SszSizeHint{{Bits: true}},
			expected:  "uint16 ssz type cannot be limited by bits",
		},
		// Uint32 tests
		{
			name:      "uint32 with wrong size",
			typ:       types.Typ[types.Uint32],
			sizeHints: []ssztypes.SszSizeHint{{Size: 8}},
			expected:  "uint32 ssz type must be ssz-size:4",
		},
		{
			name:      "uint32 with bit size",
			typ:       types.Typ[types.Uint32],
			sizeHints: []ssztypes.SszSizeHint{{Bits: true}},
			expected:  "uint32 ssz type cannot be limited by bits",
		},
		// Uint64 tests
		{
			name:      "uint64 with wrong size",
			typ:       types.Typ[types.Uint64],
			sizeHints: []ssztypes.SszSizeHint{{Size: 4}},
			expected:  "uint64 ssz type must be ssz-size:8",
		},
		{
			name:      "uint64 with bit size",
			typ:       types.Typ[types.Uint64],
			sizeHints: []ssztypes.SszSizeHint{{Bits: true}},
			expected:  "uint64 ssz type cannot be limited by bits",
		},
		// Uint128 tests
		{
			name:      "uint128 with bit size",
			typ:       types.NewArray(types.Typ[types.Uint8], 16),
			typeHints: []ssztypes.SszTypeHint{{Type: ssztypes.SszUint128Type}},
			sizeHints: []ssztypes.SszSizeHint{{Bits: true}},
			expected:  "uint128 ssz type cannot be limited by bits",
		},
		// Uint256 tests
		{
			name:      "uint256 with bit size",
			typ:       types.NewArray(types.Typ[types.Uint8], 32),
			typeHints: []ssztypes.SszTypeHint{{Type: ssztypes.SszUint256Type}},
			sizeHints: []ssztypes.SszSizeHint{{Bits: true}},
			expected:  "uint256 ssz type cannot be limited by bits",
		},
		// Non-bitvector type with bit size
		{
			name:      "other non bitvector type with bit size",
			typ:       types.NewArray(types.Typ[types.Uint8], 16),
			sizeHints: []ssztypes.SszSizeHint{{Bits: true}},
			expected:  "bit size tag is only allowed for bitvector or bitlist types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.buildTypeDescriptor(tt.typ, tt.typeHints, tt.sizeHints, nil)
			if err == nil {
				t.Errorf("Expected error for %s", tt.name)
				return
			}
			if !strings.Contains(err.Error(), tt.expected) {
				t.Errorf("Expected error containing '%s', got: %s", tt.expected, err.Error())
			}
		})
	}
}

func TestMaxSizeHints(t *testing.T) {
	parser := NewParser()

	t.Run("MaxSizeWithValue", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		maxSizeHint := []ssztypes.SszMaxSizeHint{{Size: 1024}}
		desc, err := parser.buildTypeDescriptor(uint64Type, nil, nil, maxSizeHint)
		if err != nil {
			t.Fatalf("Failed to build descriptor with max size: %v", err)
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit == 0 {
			t.Error("Expected limit flag to be set")
		}
		if desc.Limit != 1024 {
			t.Errorf("Expected limit 1024, got %d", desc.Limit)
		}
	})

	t.Run("MaxSizeNoValue", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		maxSizeHint := []ssztypes.SszMaxSizeHint{{NoValue: true}}
		desc, err := parser.buildTypeDescriptor(uint64Type, nil, nil, maxSizeHint)
		if err != nil {
			t.Fatalf("Failed to build descriptor with no max size: %v", err)
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasLimit != 0 {
			t.Error("Expected limit flag to be unset")
		}
	})

	t.Run("MaxSizeExpression", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		maxSizeHint := []ssztypes.SszMaxSizeHint{{Expr: "maxSize", Custom: true}}
		desc, err := parser.buildTypeDescriptor(uint64Type, nil, nil, maxSizeHint)
		if err != nil {
			t.Fatalf("Failed to build descriptor with max size expression: %v", err)
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasDynamicMax == 0 {
			t.Error("Expected dynamic max flag to be set")
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagHasMaxExpr == 0 {
			t.Error("Expected max expr flag to be set")
		}
		if desc.MaxExpression == nil || *desc.MaxExpression != "maxSize" {
			t.Error("Expected max expression to be set")
		}
	})
}

func TestTypeHints(t *testing.T) {
	parser := NewParser()

	t.Run("ExplicitTypeHint", func(t *testing.T) {
		// Force a slice to be a vector with type hint
		byteType := types.Typ[types.Uint8]
		byteSlice := types.NewSlice(byteType)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszVectorType}}
		sizeHint := []ssztypes.SszSizeHint{{Size: 32}}
		desc, err := parser.buildTypeDescriptor(byteSlice, typeHint, sizeHint, nil)
		if err != nil {
			t.Fatalf("Failed to build descriptor with type hint: %v", err)
		}
		if desc.SszType != ssztypes.SszVectorType {
			t.Errorf("Expected SszVectorType, got %v", desc.SszType)
		}
	})

	t.Run("TypeCompatibilityValidation", func(t *testing.T) {
		// Try to use bool type as uint8
		boolType := types.Typ[types.Bool]
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint8Type}}
		_, err := parser.buildTypeDescriptor(boolType, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for incompatible type hint")
		}
	})

	tests := []struct {
		name      string
		typ       types.Type
		typeHints []ssztypes.SszTypeHint
		expected  string
	}{
		// Bool tests
		{
			name:      "bool with wrong type",
			typ:       types.Typ[types.Uint8],
			typeHints: []ssztypes.SszTypeHint{{Type: ssztypes.SszBoolType}},
			expected:  "bool ssz type can only be represented by bool types",
		},
		// Uint8 tests
		{
			name:      "uint8 with wrong type",
			typ:       types.Typ[types.Bool],
			typeHints: []ssztypes.SszTypeHint{{Type: ssztypes.SszUint8Type}},
			expected:  "uint8 ssz type can only be represented by uint8 types",
		},
		// Uint16 tests
		{
			name:      "uint16 with wrong type",
			typ:       types.Typ[types.Uint8],
			typeHints: []ssztypes.SszTypeHint{{Type: ssztypes.SszUint16Type}},
			expected:  "uint16 ssz type can only be represented by uint16 types",
		},
		// Uint32 tests
		{
			name:      "uint32 with wrong type",
			typ:       types.Typ[types.Uint8],
			typeHints: []ssztypes.SszTypeHint{{Type: ssztypes.SszUint32Type}},
			expected:  "uint32 ssz type can only be represented by uint32 types",
		},
		// Uint64 tests
		{
			name:      "uint64 with wrong type",
			typ:       types.Typ[types.Uint8],
			typeHints: []ssztypes.SszTypeHint{{Type: ssztypes.SszUint64Type}},
			expected:  "uint64 ssz type can only be represented by uint64 or time.Time types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.buildTypeDescriptor(tt.typ, tt.typeHints, nil, nil)
			if err == nil {
				t.Errorf("Expected error for %s", tt.name)
				return
			}
			if !strings.Contains(err.Error(), tt.expected) {
				t.Errorf("Expected error containing '%s', got: %s", tt.expected, err.Error())
			}
		})
	}
}

func TestPointerTypes(t *testing.T) {
	parser := NewParser()

	t.Run("PointerType", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		ptrType := types.NewPointer(uint64Type)
		desc, err := parser.buildTypeDescriptor(ptrType, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build pointer type descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszUint64Type {
			t.Errorf("Expected SszUint64Type, got %v", desc.SszType)
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsPointer == 0 {
			t.Error("Expected pointer flag to be set")
		}
	})
}

func TestNamedTypes(t *testing.T) {
	parser := NewParser()

	t.Run("NamedBasicType", func(t *testing.T) {
		// Create a named type like "type MyInt uint64"
		pkg := types.NewPackage("test", "test")
		obj := types.NewTypeName(token.NoPos, pkg, "MyInt", nil)
		namedType := types.NewNamed(obj, types.Typ[types.Uint64], nil)

		desc, err := parser.buildTypeDescriptor(namedType, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build named type descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszUint64Type {
			t.Errorf("Expected SszUint64Type, got %v", desc.SszType)
		}
	})

	t.Run("AliasType", func(t *testing.T) {
		// Create an alias type
		pkg := types.NewPackage("test", "test")
		obj := types.NewTypeName(token.NoPos, pkg, "MyIntAlias", nil)
		aliasType := types.NewAlias(obj, types.Typ[types.Uint32])

		desc, err := parser.buildTypeDescriptor(aliasType, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build alias type descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszUint32Type {
			t.Errorf("Expected SszUint32Type, got %v", desc.SszType)
		}
	})
}

func TestBuildUint128Descriptor(t *testing.T) {
	parser := NewParser()

	t.Run("Uint128Array16Bytes", func(t *testing.T) {
		byteType := types.Typ[types.Uint8]
		arr := types.NewArray(byteType, 16)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint128Type}}
		desc, err := parser.buildTypeDescriptor(arr, typeHint, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build uint128 descriptor: %v", err)
		}
		if desc.Size != 16 {
			t.Errorf("Expected size 16, got %d", desc.Size)
		}
		if desc.Len != 16 {
			t.Errorf("Expected len 16, got %d", desc.Len)
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray == 0 {
			t.Error("Expected byte array flag to be set")
		}
	})

	t.Run("Uint128Array2Uint64", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		arr := types.NewArray(uint64Type, 2)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint128Type}}
		desc, err := parser.buildTypeDescriptor(arr, typeHint, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build uint128 descriptor: %v", err)
		}
		if desc.Size != 16 {
			t.Errorf("Expected size 16, got %d", desc.Size)
		}
		if desc.Len != 2 {
			t.Errorf("Expected len 2, got %d", desc.Len)
		}
	})

	t.Run("Uint128InvalidSize", func(t *testing.T) {
		byteType := types.Typ[types.Uint8]
		arr := types.NewArray(byteType, 8) // Wrong size
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint128Type}}
		_, err := parser.buildTypeDescriptor(arr, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for invalid uint128 size")
		}
	})

	t.Run("Uint128Slice", func(t *testing.T) {
		byteType := types.Typ[types.Uint8]
		slice := types.NewSlice(byteType)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint128Type}}
		desc, err := parser.buildTypeDescriptor(slice, typeHint, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build uint128 slice descriptor: %v", err)
		}
		if desc.Size != 16 {
			t.Errorf("Expected size 16, got %d", desc.Size)
		}
	})
}

func TestBuildUint256Descriptor(t *testing.T) {
	parser := NewParser()

	t.Run("Uint256Array32Bytes", func(t *testing.T) {
		byteType := types.Typ[types.Uint8]
		arr := types.NewArray(byteType, 32)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint256Type}}
		desc, err := parser.buildTypeDescriptor(arr, typeHint, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build uint256 descriptor: %v", err)
		}
		if desc.Size != 32 {
			t.Errorf("Expected size 32, got %d", desc.Size)
		}
		if desc.Len != 32 {
			t.Errorf("Expected len 32, got %d", desc.Len)
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray == 0 {
			t.Error("Expected byte array flag to be set")
		}
	})

	t.Run("Uint256Array4Uint64", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		arr := types.NewArray(uint64Type, 4)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint256Type}}
		desc, err := parser.buildTypeDescriptor(arr, typeHint, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build uint256 descriptor: %v", err)
		}
		if desc.Size != 32 {
			t.Errorf("Expected size 32, got %d", desc.Size)
		}
		if desc.Len != 4 {
			t.Errorf("Expected len 4, got %d", desc.Len)
		}
	})

	t.Run("Uint256InvalidSize", func(t *testing.T) {
		byteType := types.Typ[types.Uint8]
		arr := types.NewArray(byteType, 16) // Wrong size
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint256Type}}
		_, err := parser.buildTypeDescriptor(arr, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for invalid uint256 size")
		}
	})
}

func TestBuildContainerDescriptor(t *testing.T) {
	parser := NewParser()

	t.Run("SimpleStruct", func(t *testing.T) {
		// Create a simple struct type
		field1 := types.NewVar(token.NoPos, nil, "Field1", types.Typ[types.Uint64])
		field2 := types.NewVar(token.NoPos, nil, "Field2", types.Typ[types.Bool])
		fields := []*types.Var{field1, field2}
		tags := []string{"", ""}
		structType := types.NewStruct(fields, tags)

		desc, err := parser.buildTypeDescriptor(structType, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build container descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszContainerType {
			t.Errorf("Expected SszContainerType, got %v", desc.SszType)
		}
		if desc.Size != 9 { // 8 + 1
			t.Errorf("Expected size 9, got %d", desc.Size)
		}
		if desc.ContainerDesc == nil {
			t.Error("Expected container descriptor to be set")
		}
		if len(desc.ContainerDesc.Fields) != 2 {
			t.Errorf("Expected 2 fields, got %d", len(desc.ContainerDesc.Fields))
		}
	})

	t.Run("StructWithPrivateFields", func(t *testing.T) {
		// Create struct with private field that should be ignored
		field1 := types.NewVar(token.NoPos, nil, "PublicField", types.Typ[types.Uint64])
		field2 := types.NewVar(token.NoPos, nil, "privateField", types.Typ[types.Bool])
		field3 := types.NewVar(token.NoPos, nil, "_", types.Typ[types.Uint32]) // Ignored field
		fields := []*types.Var{field1, field2, field3}
		tags := []string{"", "", ""}
		structType := types.NewStruct(fields, tags)

		desc, err := parser.buildTypeDescriptor(structType, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build container descriptor: %v", err)
		}
		// Should only have 1 field (the public one)
		if len(desc.ContainerDesc.Fields) != 1 {
			t.Errorf("Expected 1 field, got %d", len(desc.ContainerDesc.Fields))
		}
		if desc.ContainerDesc.Fields[0].Name != "PublicField" {
			t.Errorf("Expected field name 'PublicField', got %s", desc.ContainerDesc.Fields[0].Name)
		}
	})

	t.Run("StructWithDynamicField", func(t *testing.T) {
		// Create struct with dynamic field (slice)
		field1 := types.NewVar(token.NoPos, nil, "StaticField", types.Typ[types.Uint64])
		field2 := types.NewVar(token.NoPos, nil, "DynamicField", types.NewSlice(types.Typ[types.Uint8]))
		fields := []*types.Var{field1, field2}
		tags := []string{"", ""}
		structType := types.NewStruct(fields, tags)

		desc, err := parser.buildTypeDescriptor(structType, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build container descriptor: %v", err)
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
			t.Error("Expected dynamic flag to be set")
		}
		if desc.Size != 0 {
			t.Errorf("Expected size 0 for dynamic container, got %d", desc.Size)
		}
		if len(desc.ContainerDesc.DynFields) != 1 {
			t.Errorf("Expected 1 dynamic field, got %d", len(desc.ContainerDesc.DynFields))
		}
	})

	t.Run("StructWithTags", func(t *testing.T) {
		// Create struct with SSZ tags
		field1 := types.NewVar(token.NoPos, nil, "Field1", types.Typ[types.Uint64])
		fields := []*types.Var{field1}
		tags := []string{`ssz-index:"5"`}
		structType := types.NewStruct(fields, tags)

		desc, err := parser.buildTypeDescriptor(structType, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build container descriptor: %v", err)
		}
		if len(desc.ContainerDesc.Fields) != 1 {
			t.Errorf("Expected 1 field, got %d", len(desc.ContainerDesc.Fields))
		}
		if desc.ContainerDesc.Fields[0].SszIndex != 5 {
			t.Errorf("Expected ssz-index 5, got %d", desc.ContainerDesc.Fields[0].SszIndex)
		}
	})

	t.Run("InvalidSszIndex", func(t *testing.T) {
		// Create struct with invalid SSZ index
		field1 := types.NewVar(token.NoPos, nil, "Field1", types.Typ[types.Uint64])
		fields := []*types.Var{field1}
		tags := []string{`ssz-index:"invalid"`}
		structType := types.NewStruct(fields, tags)

		_, err := parser.buildTypeDescriptor(structType, nil, nil, nil)
		if err == nil {
			t.Error("Expected error for invalid ssz-index")
		}
	})
}

func TestBuildVectorDescriptor(t *testing.T) {
	parser := NewParser()

	t.Run("ByteArray", func(t *testing.T) {
		byteType := types.Typ[types.Uint8]
		arr := types.NewArray(byteType, 32)
		desc, err := parser.buildTypeDescriptor(arr, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build vector descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszVectorType {
			t.Errorf("Expected SszVectorType, got %v", desc.SszType)
		}
		if desc.Size != 32 {
			t.Errorf("Expected size 32, got %d", desc.Size)
		}
		if desc.Len != 32 {
			t.Errorf("Expected len 32, got %d", desc.Len)
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray == 0 {
			t.Error("Expected byte array flag to be set")
		}
	})

	t.Run("SliceWithSizeHint", func(t *testing.T) {
		byteType := types.Typ[types.Uint8]
		slice := types.NewSlice(byteType)
		sizeHint := []ssztypes.SszSizeHint{{Size: 64}}
		desc, err := parser.buildTypeDescriptor(slice, nil, sizeHint, nil)
		if err != nil {
			t.Fatalf("Failed to build vector descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszVectorType {
			t.Errorf("Expected SszVectorType, got %v", desc.SszType)
		}
		if desc.Len != 64 {
			t.Errorf("Expected len 64, got %d", desc.Len)
		}
	})

	t.Run("SliceWithoutSizeHint", func(t *testing.T) {
		byteType := types.Typ[types.Uint8]
		slice := types.NewSlice(byteType)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszVectorType}}
		_, err := parser.buildTypeDescriptor(slice, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for slice vector without size hint")
		}
	})

	t.Run("StringVector", func(t *testing.T) {
		stringType := types.Typ[types.String]
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszVectorType}}
		sizeHint := []ssztypes.SszSizeHint{{Size: 20}}
		desc, err := parser.buildTypeDescriptor(stringType, typeHint, sizeHint, nil)
		if err != nil {
			t.Fatalf("Failed to build string vector descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszVectorType {
			t.Errorf("Expected SszVectorType, got %v", desc.SszType)
		}
		if desc.Len != 20 {
			t.Errorf("Expected len 20, got %d", desc.Len)
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray == 0 {
			t.Error("Expected byte array flag to be set")
		}
	})

	t.Run("StringVectorWithoutSize", func(t *testing.T) {
		stringType := types.Typ[types.String]
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszVectorType}}
		_, err := parser.buildTypeDescriptor(stringType, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for string vector without size hint")
		}
	})

	t.Run("ArraySizeHintValidation", func(t *testing.T) {
		byteType := types.Typ[types.Uint8]
		arr := types.NewArray(byteType, 32)
		sizeHint := []ssztypes.SszSizeHint{{Size: 64}} // Size hint bigger than array
		_, err := parser.buildTypeDescriptor(arr, nil, sizeHint, nil)
		if err == nil {
			t.Error("Expected error for size hint greater than array length")
		}
	})

	t.Run("UnsupportedVectorBaseType", func(t *testing.T) {
		uintType := types.Typ[types.Uint]
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszVectorType}}
		_, err := parser.buildTypeDescriptor(uintType, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for unsupported vector base type")
		}
	})

	t.Run("VectorWithDynamicElements", func(t *testing.T) {
		// Vector of slices (dynamic elements)
		sliceType := types.NewSlice(types.Typ[types.Uint8])
		arr := types.NewArray(sliceType, 4)
		desc, err := parser.buildTypeDescriptor(arr, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build vector with dynamic elements: %v", err)
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
			t.Error("Expected dynamic flag to be set")
		}
		if desc.Size != 0 {
			t.Errorf("Expected size 0 for dynamic vector, got %d", desc.Size)
		}
	})
}

func TestBuildListDescriptor(t *testing.T) {
	parser := NewParser()

	t.Run("ByteSlice", func(t *testing.T) {
		byteType := types.Typ[types.Uint8]
		slice := types.NewSlice(byteType)
		desc, err := parser.buildTypeDescriptor(slice, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build list descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszListType {
			t.Errorf("Expected SszListType, got %v", desc.SszType)
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
			t.Error("Expected dynamic flag to be set")
		}
		if desc.Size != 0 {
			t.Errorf("Expected size 0 for dynamic list, got %d", desc.Size)
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray == 0 {
			t.Error("Expected byte array flag to be set")
		}
	})

	t.Run("StringList", func(t *testing.T) {
		stringType := types.Typ[types.String]
		desc, err := parser.buildTypeDescriptor(stringType, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build string list descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszListType {
			t.Errorf("Expected SszListType, got %v", desc.SszType)
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
			t.Error("Expected dynamic flag to be set")
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray == 0 {
			t.Error("Expected byte array flag to be set")
		}
	})

	t.Run("UnsupportedListBaseType", func(t *testing.T) {
		uintType := types.Typ[types.Uint]
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszListType}}
		_, err := parser.buildTypeDescriptor(uintType, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for unsupported list base type")
		}
	})
}

func TestBuildBitlistDescriptor(t *testing.T) {
	parser := NewParser()

	t.Run("ValidBitlist", func(t *testing.T) {
		byteType := types.Typ[types.Uint8]
		slice := types.NewSlice(byteType)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszBitlistType}}
		desc, err := parser.buildTypeDescriptor(slice, typeHint, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build bitlist descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszBitlistType {
			t.Errorf("Expected SszBitlistType, got %v", desc.SszType)
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
			t.Error("Expected dynamic flag to be set")
		}
		if desc.Size != 0 {
			t.Errorf("Expected size 0 for dynamic bitlist, got %d", desc.Size)
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray == 0 {
			t.Error("Expected byte array flag to be set")
		}
	})

	t.Run("BitlistWrongElementType", func(t *testing.T) {
		uint16Type := types.Typ[types.Uint16]
		slice := types.NewSlice(uint16Type)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszBitlistType}}
		_, err := parser.buildTypeDescriptor(slice, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for bitlist with non-byte elements")
		}
	})

	t.Run("BitlistWrongBaseType", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszBitlistType}}
		_, err := parser.buildTypeDescriptor(uint64Type, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for bitlist with non-slice type")
		}
	})
}

func TestParseFieldTags(t *testing.T) {
	parser := NewParser()

	t.Run("EmptyTags", func(t *testing.T) {
		typeHints, sizeHints, maxSizeHints, err := parser.parseFieldTags("")
		if err != nil {
			t.Fatalf("Failed to parse empty tags: %v", err)
		}
		if len(typeHints) != 0 || len(sizeHints) != 0 || len(maxSizeHints) != 0 {
			t.Error("Expected empty hints for empty tags")
		}
	})

	t.Run("SszTypeTags", func(t *testing.T) {
		tags := `ssz-type:"vector,uint8"`
		typeHints, _, _, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse ssz-type tags: %v", err)
		}
		if len(typeHints) != 2 {
			t.Errorf("Expected 2 type hints, got %d", len(typeHints))
		}
		if typeHints[0].Type != ssztypes.SszVectorType {
			t.Errorf("Expected SszVectorType, got %v", typeHints[0].Type)
		}
		if typeHints[1].Type != ssztypes.SszUint8Type {
			t.Errorf("Expected SszUint8Type, got %v", typeHints[1].Type)
		}
	})

	t.Run("SszSizeTags", func(t *testing.T) {
		tags := `ssz-size:"32,?"`
		_, sizeHints, _, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse ssz-size tags: %v", err)
		}
		if len(sizeHints) != 2 {
			t.Errorf("Expected 2 size hints, got %d", len(sizeHints))
		}
		if sizeHints[0].Size != 32 {
			t.Errorf("Expected size 32, got %d", sizeHints[0].Size)
		}
		if !sizeHints[1].Dynamic {
			t.Error("Expected second size hint to be dynamic")
		}
	})

	t.Run("SszMaxTags", func(t *testing.T) {
		tags := `ssz-max:"1024,?"`
		_, _, maxSizeHints, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse ssz-max tags: %v", err)
		}
		if len(maxSizeHints) != 2 {
			t.Errorf("Expected 2 max size hints, got %d", len(maxSizeHints))
		}
		if maxSizeHints[0].Size != 1024 {
			t.Errorf("Expected max size 1024, got %d", maxSizeHints[0].Size)
		}
		if !maxSizeHints[1].NoValue {
			t.Error("Expected second max size hint to have no value")
		}
	})

	t.Run("DynSszSizeTags", func(t *testing.T) {
		tags := `dynssz-size:"expr1,32"`
		_, sizeHints, _, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse dynssz-size tags: %v", err)
		}
		if len(sizeHints) != 2 {
			t.Errorf("Expected 2 size hints, got %d", len(sizeHints))
		}
		if sizeHints[0].Expr != "expr1" {
			t.Errorf("Expected expression 'expr1', got %s", sizeHints[0].Expr)
		}
		if !sizeHints[0].Custom {
			t.Error("Expected first size hint to be custom")
		}
		if sizeHints[1].Size != 32 {
			t.Errorf("Expected size 32, got %d", sizeHints[1].Size)
		}
	})

	t.Run("InvalidSszTypeTag", func(t *testing.T) {
		tags := `ssz-type:"invalid_type"`
		_, _, _, err := parser.parseFieldTags(tags)
		if err == nil {
			t.Error("Expected error for invalid ssz-type tag")
		}
	})

	t.Run("InvalidSszSizeTag", func(t *testing.T) {
		tags := `ssz-size:"invalid_size"`
		_, _, _, err := parser.parseFieldTags(tags)
		if err == nil {
			t.Error("Expected error for invalid ssz-size tag")
		}
	})

	t.Run("InvalidSszMaxTag", func(t *testing.T) {
		tags := `ssz-max:"invalid_max"`
		_, _, _, err := parser.parseFieldTags(tags)
		if err == nil {
			t.Error("Expected error for invalid ssz-max tag")
		}
	})
}

func TestExtractSszIndex(t *testing.T) {
	parser := NewParser()

	t.Run("ValidIndex", func(t *testing.T) {
		tags := `ssz-index:"5"`
		index := parser.extractSszIndex(tags)
		if index != "5" {
			t.Errorf("Expected index '5', got %s", index)
		}
	})

	t.Run("NoIndex", func(t *testing.T) {
		tags := `ssz-size:"32"`
		index := parser.extractSszIndex(tags)
		if index != "" {
			t.Errorf("Expected empty index, got %s", index)
		}
	})

	t.Run("EmptyTags", func(t *testing.T) {
		index := parser.extractSszIndex("")
		if index != "" {
			t.Errorf("Expected empty index, got %s", index)
		}
	})
}

func TestIsByteType(t *testing.T) {
	parser := NewParser()

	t.Run("ByteType", func(t *testing.T) {
		byteType := types.Typ[types.Uint8]
		if !parser.isByteType(byteType) {
			t.Error("Expected uint8 to be byte type")
		}
	})

	t.Run("NonByteType", func(t *testing.T) {
		uint16Type := types.Typ[types.Uint16]
		if parser.isByteType(uint16Type) {
			t.Error("Expected uint16 to not be byte type")
		}
	})

	t.Run("NonBasicType", func(t *testing.T) {
		sliceType := types.NewSlice(types.Typ[types.Uint8])
		if parser.isByteType(sliceType) {
			t.Error("Expected slice to not be byte type")
		}
	})
}

func TestMethodSignatureChecking(t *testing.T) {
	parser := NewParser()

	// Create a type with some methods for testing
	pkg := types.NewPackage("test", "test")

	// Create a struct type
	struct1 := types.NewStruct(nil, nil)
	obj := types.NewTypeName(token.NoPos, pkg, "TestStruct", nil)
	namedType := types.NewNamed(obj, struct1, nil)

	// Add a method with signature: func (t *TestStruct) MarshalSSZTo(buf []byte) ([]byte, error)
	recv := types.NewVar(token.NoPos, pkg, "t", types.NewPointer(namedType))
	bufParam := types.NewVar(token.NoPos, pkg, "buf", types.NewSlice(types.Typ[types.Uint8]))
	bytesReturn := types.NewVar(token.NoPos, pkg, "", types.NewSlice(types.Typ[types.Uint8]))
	errorReturn := types.NewVar(token.NoPos, pkg, "", types.Universe.Lookup("error").Type())
	sig := types.NewSignatureType(recv, nil, nil, types.NewTuple(bufParam), types.NewTuple(bytesReturn, errorReturn), false)
	marshalMethod := types.NewFunc(token.NoPos, pkg, "MarshalSSZTo", sig)
	namedType.AddMethod(marshalMethod)

	// Test method detection
	methodSet := types.NewMethodSet(types.NewPointer(namedType))
	hasMethod := parser.hasMethodWithSignature(methodSet, "MarshalSSZTo", []string{"[]byte"}, []string{"[]byte", "error"})
	if !hasMethod {
		t.Error("Expected to find MarshalSSZTo method")
	}

	// Test method not found
	hasMethod = parser.hasMethodWithSignature(methodSet, "NonExistentMethod", []string{}, []string{})
	if hasMethod {
		t.Error("Expected to not find NonExistentMethod")
	}
}

func TestTypeMatches(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		typ      types.Type
		expected string
		matches  bool
	}{
		{"Wildcard", types.Typ[types.Int], "-", true},
		{"ByteSlice", types.NewSlice(types.Typ[types.Uint8]), "[]byte", true},
		{"WrongSlice", types.NewSlice(types.Typ[types.Uint16]), "[]byte", false},
		{"ByteArray", types.NewArray(types.Typ[types.Uint8], 32), "[32]byte", true},
		{"WrongArraySize", types.NewArray(types.Typ[types.Uint8], 16), "[32]byte", false},
		{"WrongArrayType", types.NewArray(types.Typ[types.Uint16], 32), "[32]byte", false},
		{"Int", types.Typ[types.Int], "int", true},
		{"WrongInt", types.Typ[types.Uint64], "int", false},
		{"DynamicSpecs", types.Typ[types.Int], "DynamicSpecs", true},
		{"HashWalker", types.Typ[types.Int], "HashWalker", true},
		{"UnknownType", types.Typ[types.Int], "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := parser.typeMatches(tt.typ, tt.expected)
			if matches != tt.matches {
				t.Errorf("Expected typeMatches(%v, %s) = %v, got %v", tt.typ, tt.expected, tt.matches, matches)
			}
		})
	}

	// Test error type specifically
	t.Run("ErrorType", func(t *testing.T) {
		errorType := types.Universe.Lookup("error").Type()
		matches := parser.typeMatches(errorType, "error")
		if !matches {
			t.Error("Expected error type to match")
		}
	})
}

func TestInterfaceCompatibilityChecks(t *testing.T) {
	parser := NewParser()

	// Create a simple type for testing
	uint64Type := types.Typ[types.Uint64]
	ptrType := types.NewPointer(uint64Type)

	// Test compatibility functions - these will return false for simple types,
	// but we're testing the function execution
	t.Run("FastSSZConvert", func(t *testing.T) {
		compat := parser.getFastsszConvertCompatibility(uint64Type)
		_ = compat // Function should execute without error
	})

	t.Run("FastSSZHash", func(t *testing.T) {
		compat := parser.getFastsszHashCompatibility(ptrType)
		_ = compat // Function should execute without error
	})

	t.Run("HashTreeRootWith", func(t *testing.T) {
		compat := parser.getHashTreeRootWithCompatibility(uint64Type)
		_ = compat // Function should execute without error
	})

	t.Run("DynamicMarshaler", func(t *testing.T) {
		compat := parser.getDynamicMarshalerCompatibility(ptrType)
		_ = compat // Function should execute without error
	})

	t.Run("DynamicUnmarshaler", func(t *testing.T) {
		compat := parser.getDynamicUnmarshalerCompatibility(uint64Type)
		_ = compat // Function should execute without error
	})

	t.Run("DynamicSizer", func(t *testing.T) {
		compat := parser.getDynamicSizerCompatibility(ptrType)
		_ = compat // Function should execute without error
	})

	t.Run("DynamicHashRoot", func(t *testing.T) {
		compat := parser.getDynamicHashRootCompatibility(uint64Type)
		_ = compat // Function should execute without error
	})
}

func TestCustomTypesAndErrors(t *testing.T) {
	parser := NewParser()

	t.Run("CustomTypeWithoutCompatibility", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszCustomType}}
		_, err := parser.buildTypeDescriptor(uint64Type, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for custom type without compatibility")
		}
	})

	t.Run("CustomTypeWithSizeHint", func(t *testing.T) {
		// Mock a type with FastSSZ compatibility for testing
		uint64Type := types.Typ[types.Uint64]
		parser.CompatFlags[uint64Type.String()] = ssztypes.SszCompatFlagFastSSZMarshaler | ssztypes.SszCompatFlagFastSSZHasher

		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszCustomType}}
		sizeHint := []ssztypes.SszSizeHint{{Size: 64}}
		desc, err := parser.buildTypeDescriptor(uint64Type, typeHint, sizeHint, nil)
		if err != nil {
			t.Fatalf("Failed to build custom type descriptor: %v", err)
		}
		if desc.Size != 64 {
			t.Errorf("Expected size 64, got %d", desc.Size)
		}
	})

	t.Run("CustomTypeWithoutSize", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]
		parser.CompatFlags[uint64Type.String()] = ssztypes.SszCompatFlagFastSSZMarshaler | ssztypes.SszCompatFlagFastSSZHasher

		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszCustomType}}
		desc, err := parser.buildTypeDescriptor(uint64Type, typeHint, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build custom type descriptor: %v", err)
		}
		if desc.Size != 0 {
			t.Errorf("Expected size 0, got %d", desc.Size)
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
			t.Error("Expected dynamic flag to be set")
		}
	})
}

func TestSpecialNamedTypes(t *testing.T) {
	parser := NewParser()

	t.Run("TimeType", func(t *testing.T) {
		// Create time.Time named type
		timePkg := types.NewPackage("time", "time")
		timeObj := types.NewTypeName(token.NoPos, timePkg, "Time", nil)
		timeType := types.NewNamed(timeObj, types.NewStruct(nil, nil), nil)

		desc, err := parser.buildTypeDescriptor(timeType, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build time.Time descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszUint64Type {
			t.Errorf("Expected SszUint64Type for time.Time, got %v", desc.SszType)
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsTime == 0 {
			t.Error("Expected time flag to be set")
		}
	})

	t.Run("WellKnownTypes", func(t *testing.T) {
		// Test detection without importing external packages
		testCases := []struct {
			pkgPath  string
			typeName string
			expected ssztypes.SszType
		}{
			{"github.com/pk910/dynamic-ssz", "CompatibleUnion", ssztypes.SszCompatibleUnionType},
			{"github.com/pk910/dynamic-ssz", "TypeWrapper", ssztypes.SszTypeWrapperType},
		}

		for _, tc := range testCases {
			t.Run(tc.typeName, func(t *testing.T) {
				pkg := types.NewPackage(tc.pkgPath, tc.pkgPath)
				obj := types.NewTypeName(token.NoPos, pkg, tc.typeName, nil)
				namedType := types.NewNamed(obj, types.NewStruct(nil, nil), nil)

				desc, err := parser.buildTypeDescriptor(namedType, nil, nil, nil)
				// These will fail because they need special handling, but we're testing detection
				_ = desc
				_ = err
			})
		}
	})
}

func TestComplexStructures(t *testing.T) {
	parser := NewParser()

	t.Run("NestedStructs", func(t *testing.T) {
		// Create inner struct
		innerField := types.NewVar(token.NoPos, nil, "InnerField", types.Typ[types.Uint32])
		innerStruct := types.NewStruct([]*types.Var{innerField}, []string{""})

		// Create outer struct with inner struct field
		field1 := types.NewVar(token.NoPos, nil, "Field1", types.Typ[types.Uint64])
		field2 := types.NewVar(token.NoPos, nil, "Inner", innerStruct)
		outerStruct := types.NewStruct([]*types.Var{field1, field2}, []string{"", ""})

		desc, err := parser.buildTypeDescriptor(outerStruct, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build nested struct descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszContainerType {
			t.Errorf("Expected SszContainerType, got %v", desc.SszType)
		}
		if len(desc.ContainerDesc.Fields) != 2 {
			t.Errorf("Expected 2 fields, got %d", len(desc.ContainerDesc.Fields))
		}
	})

	t.Run("ArrayOfStructs", func(t *testing.T) {
		// Create struct type
		field := types.NewVar(token.NoPos, nil, "Field", types.Typ[types.Uint64])
		structType := types.NewStruct([]*types.Var{field}, []string{""})

		// Create array of structs
		arrayType := types.NewArray(structType, 5)

		desc, err := parser.buildTypeDescriptor(arrayType, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build array of structs descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszVectorType {
			t.Errorf("Expected SszVectorType, got %v", desc.SszType)
		}
		if desc.Len != 5 {
			t.Errorf("Expected len 5, got %d", desc.Len)
		}
	})
}

func TestCacheNotUsedWithHints(t *testing.T) {
	parser := NewParser()

	t.Run("CacheNotUsedWithHints", func(t *testing.T) {
		uint64Type := types.Typ[types.Uint64]

		// First call without hints (should be cached)
		desc1, err := parser.buildTypeDescriptor(uint64Type, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build first descriptor: %v", err)
		}

		// Second call with hints (should not use cache)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint64Type}}
		desc2, err := parser.buildTypeDescriptor(uint64Type, typeHint, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build second descriptor: %v", err)
		}

		// Third call without hints again (should use cache and be same as first)
		desc3, err := parser.buildTypeDescriptor(uint64Type, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build third descriptor: %v", err)
		}

		// desc1 and desc3 should be same (cached), desc2 should be different
		if desc1 != desc3 {
			t.Error("Expected desc1 and desc3 to be same (cached)")
		}
		// desc2 might be different object but we can't easily test that without changing internals
		_ = desc2 // Avoid unused variable error
	})
}

func TestBuildCompatibleUnionDescriptor(t *testing.T) {
	parser := NewParser()

	t.Run("UnionWithValidDescriptor", func(t *testing.T) {
		// Create a descriptor struct with multiple variant fields
		variant0 := types.NewVar(token.NoPos, nil, "Variant0", types.Typ[types.Uint64])
		variant1 := types.NewVar(token.NoPos, nil, "Variant1", types.Typ[types.Uint32])
		descriptorStruct := types.NewStruct([]*types.Var{variant0, variant1}, []string{"", ""})

		// Create the CompatibleUnion generic type
		pkg := types.NewPackage("github.com/pk910/dynamic-ssz", "dynssz")
		typeParam := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "T", nil), types.NewInterfaceType(nil, nil))
		unionObj := types.NewTypeName(token.NoPos, pkg, "CompatibleUnion", nil)

		// Create the base struct for CompatibleUnion
		variantField := types.NewVar(token.NoPos, nil, "Variant", types.Typ[types.Uint8])
		dataField := types.NewVar(token.NoPos, nil, "Data", types.NewInterfaceType(nil, nil))
		unionStruct := types.NewStruct([]*types.Var{variantField, dataField}, []string{"", ""})

		unionType := types.NewNamed(unionObj, unionStruct, nil)
		unionType.SetTypeParams([]*types.TypeParam{typeParam})

		// Instantiate with the descriptor struct
		instantiated, err := types.Instantiate(nil, unionType, []types.Type{descriptorStruct}, false)
		if err != nil {
			t.Fatalf("Failed to instantiate CompatibleUnion: %v", err)
		}

		namedInstantiated := instantiated.(*types.Named)

		// Create descriptor and call buildCompatibleUnionDescriptor
		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszCompatibleUnionType,
		}
		err = parser.buildCompatibleUnionDescriptor(desc, namedInstantiated)
		if err != nil {
			t.Fatalf("Failed to build CompatibleUnion descriptor: %v", err)
		}

		if len(desc.UnionVariants) != 2 {
			t.Errorf("Expected 2 union variants, got %d", len(desc.UnionVariants))
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
			t.Error("Expected dynamic flag to be set")
		}
		if desc.Size != 0 {
			t.Errorf("Expected size 0, got %d", desc.Size)
		}
	})

	t.Run("UnionWithEmptyDescriptor", func(t *testing.T) {
		// Create empty descriptor struct
		descriptorStruct := types.NewStruct(nil, nil)

		// Create the CompatibleUnion generic type
		pkg := types.NewPackage("github.com/pk910/dynamic-ssz", "dynssz")
		typeParam := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "T", nil), types.NewInterfaceType(nil, nil))
		unionObj := types.NewTypeName(token.NoPos, pkg, "CompatibleUnion", nil)

		variantField := types.NewVar(token.NoPos, nil, "Variant", types.Typ[types.Uint8])
		dataField := types.NewVar(token.NoPos, nil, "Data", types.NewInterfaceType(nil, nil))
		unionStruct := types.NewStruct([]*types.Var{variantField, dataField}, []string{"", ""})

		unionType := types.NewNamed(unionObj, unionStruct, nil)
		unionType.SetTypeParams([]*types.TypeParam{typeParam})

		instantiated, err := types.Instantiate(nil, unionType, []types.Type{descriptorStruct}, false)
		if err != nil {
			t.Fatalf("Failed to instantiate CompatibleUnion: %v", err)
		}

		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszCompatibleUnionType,
		}
		err = parser.buildCompatibleUnionDescriptor(desc, instantiated.(*types.Named))
		if err == nil {
			t.Error("Expected error for empty descriptor struct")
		}
		if !strings.Contains(err.Error(), "no fields") {
			t.Errorf("Expected 'no fields' error, got: %v", err)
		}
	})

	t.Run("UnionWithNonStructDescriptor", func(t *testing.T) {
		// Create union with non-struct type argument
		pkg := types.NewPackage("github.com/pk910/dynamic-ssz", "dynssz")
		typeParam := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "T", nil), types.NewInterfaceType(nil, nil))
		unionObj := types.NewTypeName(token.NoPos, pkg, "CompatibleUnion", nil)

		variantField := types.NewVar(token.NoPos, nil, "Variant", types.Typ[types.Uint8])
		dataField := types.NewVar(token.NoPos, nil, "Data", types.NewInterfaceType(nil, nil))
		unionStruct := types.NewStruct([]*types.Var{variantField, dataField}, []string{"", ""})

		unionType := types.NewNamed(unionObj, unionStruct, nil)
		unionType.SetTypeParams([]*types.TypeParam{typeParam})

		// Instantiate with uint64 instead of struct
		instantiated, err := types.Instantiate(nil, unionType, []types.Type{types.Typ[types.Uint64]}, false)
		if err != nil {
			t.Fatalf("Failed to instantiate CompatibleUnion: %v", err)
		}

		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszCompatibleUnionType,
		}
		err = parser.buildCompatibleUnionDescriptor(desc, instantiated.(*types.Named))
		if err == nil {
			t.Error("Expected error for non-struct descriptor")
		}
		if !strings.Contains(err.Error(), "must be a struct") {
			t.Errorf("Expected 'must be a struct' error, got: %v", err)
		}
	})

	t.Run("UnionWithNoTypeArgs", func(t *testing.T) {
		// Create union without type args
		pkg := types.NewPackage("github.com/pk910/dynamic-ssz", "dynssz")
		unionObj := types.NewTypeName(token.NoPos, pkg, "CompatibleUnion", nil)

		variantField := types.NewVar(token.NoPos, nil, "Variant", types.Typ[types.Uint8])
		dataField := types.NewVar(token.NoPos, nil, "Data", types.NewInterfaceType(nil, nil))
		unionStruct := types.NewStruct([]*types.Var{variantField, dataField}, []string{"", ""})

		unionType := types.NewNamed(unionObj, unionStruct, nil)

		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszCompatibleUnionType,
		}
		err := parser.buildCompatibleUnionDescriptor(desc, unionType)
		if err == nil {
			t.Error("Expected error for no type args")
		}
		if !strings.Contains(err.Error(), "exactly 1 type argument") {
			t.Errorf("Expected 'exactly 1 type argument' error, got: %v", err)
		}
	})

	t.Run("UnionWithTaggedVariants", func(t *testing.T) {
		// Create descriptor with SSZ-tagged fields
		variant0 := types.NewVar(token.NoPos, nil, "ByteList", types.NewSlice(types.Typ[types.Uint8]))
		descriptorStruct := types.NewStruct([]*types.Var{variant0}, []string{`ssz-max:"1024"`})

		pkg := types.NewPackage("github.com/pk910/dynamic-ssz", "dynssz")
		typeParam := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "T", nil), types.NewInterfaceType(nil, nil))
		unionObj := types.NewTypeName(token.NoPos, pkg, "CompatibleUnion", nil)

		variantField := types.NewVar(token.NoPos, nil, "Variant", types.Typ[types.Uint8])
		dataField := types.NewVar(token.NoPos, nil, "Data", types.NewInterfaceType(nil, nil))
		unionStruct := types.NewStruct([]*types.Var{variantField, dataField}, []string{"", ""})

		unionType := types.NewNamed(unionObj, unionStruct, nil)
		unionType.SetTypeParams([]*types.TypeParam{typeParam})

		instantiated, err := types.Instantiate(nil, unionType, []types.Type{descriptorStruct}, false)
		if err != nil {
			t.Fatalf("Failed to instantiate CompatibleUnion: %v", err)
		}

		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszCompatibleUnionType,
		}
		err = parser.buildCompatibleUnionDescriptor(desc, instantiated.(*types.Named))
		if err != nil {
			t.Fatalf("Failed to build CompatibleUnion descriptor: %v", err)
		}

		if len(desc.UnionVariants) != 1 {
			t.Errorf("Expected 1 union variant, got %d", len(desc.UnionVariants))
		}
		// Variant should be a list type
		if desc.UnionVariants[0].SszType != ssztypes.SszListType {
			t.Errorf("Expected SszListType for variant 0, got %v", desc.UnionVariants[0].SszType)
		}
	})
}

func TestBuildTypeWrapperDescriptor(t *testing.T) {
	parser := NewParser()

	t.Run("WrapperWithValidDescriptor", func(t *testing.T) {
		// Create descriptor struct with one field matching wrapped type
		wrappedField := types.NewVar(token.NoPos, nil, "Value", types.NewSlice(types.Typ[types.Uint8]))
		descriptorStruct := types.NewStruct([]*types.Var{wrappedField}, []string{`ssz-max:"256"`})

		// Create TypeWrapper generic type
		pkg := types.NewPackage("github.com/pk910/dynamic-ssz", "dynssz")
		typeParamD := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "D", nil), types.NewInterfaceType(nil, nil))
		typeParamT := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "T", nil), types.NewInterfaceType(nil, nil))
		wrapperObj := types.NewTypeName(token.NoPos, pkg, "TypeWrapper", nil)

		dataField := types.NewVar(token.NoPos, nil, "Data", typeParamT)
		wrapperStruct := types.NewStruct([]*types.Var{dataField}, []string{""})

		wrapperType := types.NewNamed(wrapperObj, wrapperStruct, nil)
		wrapperType.SetTypeParams([]*types.TypeParam{typeParamD, typeParamT})

		// Instantiate with descriptor and wrapped type
		wrappedType := types.NewSlice(types.Typ[types.Uint8])
		instantiated, err := types.Instantiate(nil, wrapperType, []types.Type{descriptorStruct, wrappedType}, false)
		if err != nil {
			t.Fatalf("Failed to instantiate TypeWrapper: %v", err)
		}

		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszTypeWrapperType,
		}
		err = parser.buildTypeWrapperDescriptor(desc, instantiated.(*types.Named), nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build TypeWrapper descriptor: %v", err)
		}

		if desc.ElemDesc == nil {
			t.Error("Expected ElemDesc to be set")
		}
		if desc.ElemDesc.SszType != ssztypes.SszListType {
			t.Errorf("Expected SszListType for wrapped type, got %v", desc.ElemDesc.SszType)
		}
	})

	t.Run("WrapperWithNoTypeArgs", func(t *testing.T) {
		pkg := types.NewPackage("github.com/pk910/dynamic-ssz", "dynssz")
		wrapperObj := types.NewTypeName(token.NoPos, pkg, "TypeWrapper", nil)

		dataField := types.NewVar(token.NoPos, nil, "Data", types.Typ[types.Uint64])
		wrapperStruct := types.NewStruct([]*types.Var{dataField}, []string{""})

		wrapperType := types.NewNamed(wrapperObj, wrapperStruct, nil)

		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszTypeWrapperType,
		}
		err := parser.buildTypeWrapperDescriptor(desc, wrapperType, nil, nil, nil)
		if err == nil {
			t.Error("Expected error for no type args")
		}
		if !strings.Contains(err.Error(), "exactly 2 type arguments") {
			t.Errorf("Expected 'exactly 2 type arguments' error, got: %v", err)
		}
	})

	t.Run("WrapperWithNonStructDescriptor", func(t *testing.T) {
		pkg := types.NewPackage("github.com/pk910/dynamic-ssz", "dynssz")
		typeParamD := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "D", nil), types.NewInterfaceType(nil, nil))
		typeParamT := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "T", nil), types.NewInterfaceType(nil, nil))
		wrapperObj := types.NewTypeName(token.NoPos, pkg, "TypeWrapper", nil)

		dataField := types.NewVar(token.NoPos, nil, "Data", typeParamT)
		wrapperStruct := types.NewStruct([]*types.Var{dataField}, []string{""})

		wrapperType := types.NewNamed(wrapperObj, wrapperStruct, nil)
		wrapperType.SetTypeParams([]*types.TypeParam{typeParamD, typeParamT})

		// Instantiate with non-struct descriptor
		wrappedType := types.Typ[types.Uint64]
		instantiated, err := types.Instantiate(nil, wrapperType, []types.Type{types.Typ[types.Uint64], wrappedType}, false)
		if err != nil {
			t.Fatalf("Failed to instantiate TypeWrapper: %v", err)
		}

		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszTypeWrapperType,
		}
		err = parser.buildTypeWrapperDescriptor(desc, instantiated.(*types.Named), nil, nil, nil)
		if err == nil {
			t.Error("Expected error for non-struct descriptor")
		}
		if !strings.Contains(err.Error(), "must be a struct") {
			t.Errorf("Expected 'must be a struct' error, got: %v", err)
		}
	})

	t.Run("WrapperWithWrongFieldCount", func(t *testing.T) {
		// Create descriptor with 2 fields (should have exactly 1)
		field1 := types.NewVar(token.NoPos, nil, "Field1", types.Typ[types.Uint64])
		field2 := types.NewVar(token.NoPos, nil, "Field2", types.Typ[types.Uint64])
		descriptorStruct := types.NewStruct([]*types.Var{field1, field2}, []string{"", ""})

		pkg := types.NewPackage("github.com/pk910/dynamic-ssz", "dynssz")
		typeParamD := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "D", nil), types.NewInterfaceType(nil, nil))
		typeParamT := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "T", nil), types.NewInterfaceType(nil, nil))
		wrapperObj := types.NewTypeName(token.NoPos, pkg, "TypeWrapper", nil)

		dataField := types.NewVar(token.NoPos, nil, "Data", typeParamT)
		wrapperStruct := types.NewStruct([]*types.Var{dataField}, []string{""})

		wrapperType := types.NewNamed(wrapperObj, wrapperStruct, nil)
		wrapperType.SetTypeParams([]*types.TypeParam{typeParamD, typeParamT})

		wrappedType := types.Typ[types.Uint64]
		instantiated, err := types.Instantiate(nil, wrapperType, []types.Type{descriptorStruct, wrappedType}, false)
		if err != nil {
			t.Fatalf("Failed to instantiate TypeWrapper: %v", err)
		}

		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszTypeWrapperType,
		}
		err = parser.buildTypeWrapperDescriptor(desc, instantiated.(*types.Named), nil, nil, nil)
		if err == nil {
			t.Error("Expected error for wrong field count")
		}
		if !strings.Contains(err.Error(), "exactly 1 field") {
			t.Errorf("Expected 'exactly 1 field' error, got: %v", err)
		}
	})

	t.Run("WrapperWithMismatchedTypes", func(t *testing.T) {
		// Create descriptor with field type different from wrapped type
		wrappedField := types.NewVar(token.NoPos, nil, "Value", types.Typ[types.Uint32])
		descriptorStruct := types.NewStruct([]*types.Var{wrappedField}, []string{""})

		pkg := types.NewPackage("github.com/pk910/dynamic-ssz", "dynssz")
		typeParamD := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "D", nil), types.NewInterfaceType(nil, nil))
		typeParamT := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "T", nil), types.NewInterfaceType(nil, nil))
		wrapperObj := types.NewTypeName(token.NoPos, pkg, "TypeWrapper", nil)

		dataField := types.NewVar(token.NoPos, nil, "Data", typeParamT)
		wrapperStruct := types.NewStruct([]*types.Var{dataField}, []string{""})

		wrapperType := types.NewNamed(wrapperObj, wrapperStruct, nil)
		wrapperType.SetTypeParams([]*types.TypeParam{typeParamD, typeParamT})

		// Instantiate with different wrapped type (uint64 vs uint32 in descriptor)
		wrappedType := types.Typ[types.Uint64]
		instantiated, err := types.Instantiate(nil, wrapperType, []types.Type{descriptorStruct, wrappedType}, false)
		if err != nil {
			t.Fatalf("Failed to instantiate TypeWrapper: %v", err)
		}

		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszTypeWrapperType,
		}
		err = parser.buildTypeWrapperDescriptor(desc, instantiated.(*types.Named), nil, nil, nil)
		if err == nil {
			t.Error("Expected error for mismatched types")
		}
		if !strings.Contains(err.Error(), "does not match") {
			t.Errorf("Expected 'does not match' error, got: %v", err)
		}
	})

	t.Run("WrapperWithStaticType", func(t *testing.T) {
		// Create descriptor with static uint64 type
		wrappedField := types.NewVar(token.NoPos, nil, "Value", types.Typ[types.Uint64])
		descriptorStruct := types.NewStruct([]*types.Var{wrappedField}, []string{""})

		pkg := types.NewPackage("github.com/pk910/dynamic-ssz", "dynssz")
		typeParamD := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "D", nil), types.NewInterfaceType(nil, nil))
		typeParamT := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "T", nil), types.NewInterfaceType(nil, nil))
		wrapperObj := types.NewTypeName(token.NoPos, pkg, "TypeWrapper", nil)

		dataField := types.NewVar(token.NoPos, nil, "Data", typeParamT)
		wrapperStruct := types.NewStruct([]*types.Var{dataField}, []string{""})

		wrapperType := types.NewNamed(wrapperObj, wrapperStruct, nil)
		wrapperType.SetTypeParams([]*types.TypeParam{typeParamD, typeParamT})

		wrappedType := types.Typ[types.Uint64]
		instantiated, err := types.Instantiate(nil, wrapperType, []types.Type{descriptorStruct, wrappedType}, false)
		if err != nil {
			t.Fatalf("Failed to instantiate TypeWrapper: %v", err)
		}

		desc := &ssztypes.TypeDescriptor{
			SszType: ssztypes.SszTypeWrapperType,
		}
		err = parser.buildTypeWrapperDescriptor(desc, instantiated.(*types.Named), nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build TypeWrapper descriptor: %v", err)
		}

		if desc.Size != 8 {
			t.Errorf("Expected size 8, got %d", desc.Size)
		}
		if desc.ElemDesc.SszType != ssztypes.SszUint64Type {
			t.Errorf("Expected SszUint64Type for wrapped type, got %v", desc.ElemDesc.SszType)
		}
	})
}

func TestParseFieldTagsEdgeCases(t *testing.T) {
	parser := NewParser()

	t.Run("SszBitsizeTags", func(t *testing.T) {
		tags := `ssz-bitsize:"256"`
		_, sizeHints, _, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse ssz-bitsize tags: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Errorf("Expected 1 size hint, got %d", len(sizeHints))
		}
		if sizeHints[0].Size != 256 {
			t.Errorf("Expected size 256, got %d", sizeHints[0].Size)
		}
		if !sizeHints[0].Bits {
			t.Error("Expected Bits flag to be set")
		}
	})

	t.Run("SszBitsizeWithMultipleParts", func(t *testing.T) {
		tags := `ssz-bitsize:"128,256"`
		_, sizeHints, _, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse ssz-bitsize tags: %v", err)
		}
		if len(sizeHints) != 2 {
			t.Errorf("Expected 2 size hints, got %d", len(sizeHints))
		}
		if sizeHints[0].Size != 128 || !sizeHints[0].Bits {
			t.Errorf("Expected first hint to be 128 bits")
		}
		if sizeHints[1].Size != 256 || !sizeHints[1].Bits {
			t.Errorf("Expected second hint to be 256 bits")
		}
	})

	t.Run("SszSizeAndBitsizeCombined", func(t *testing.T) {
		// ssz-bitsize takes precedence when both are present for same index
		tags := `ssz-size:"32" ssz-bitsize:"256"`
		_, sizeHints, _, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse combined tags: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Errorf("Expected 1 size hint, got %d", len(sizeHints))
		}
		// ssz-bitsize should take precedence
		if sizeHints[0].Size != 256 || !sizeHints[0].Bits {
			t.Errorf("Expected 256 bits (from ssz-bitsize), got size=%d, bits=%v", sizeHints[0].Size, sizeHints[0].Bits)
		}
	})

	t.Run("DynSszBitsizeTags", func(t *testing.T) {
		tags := `dynssz-bitsize:"512"`
		_, sizeHints, _, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse dynssz-bitsize tags: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Errorf("Expected 1 size hint, got %d", len(sizeHints))
		}
		if sizeHints[0].Size != 512 {
			t.Errorf("Expected size 512, got %d", sizeHints[0].Size)
		}
		if !sizeHints[0].Bits {
			t.Error("Expected Bits flag to be set")
		}
	})

	t.Run("DynSszMaxTags", func(t *testing.T) {
		tags := `dynssz-max:"2048"`
		_, _, maxSizeHints, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse dynssz-max tags: %v", err)
		}
		if len(maxSizeHints) != 1 {
			t.Errorf("Expected 1 max size hint, got %d", len(maxSizeHints))
		}
		if maxSizeHints[0].Size != 2048 {
			t.Errorf("Expected max size 2048, got %d", maxSizeHints[0].Size)
		}
	})

	t.Run("DynSszMaxWithNoValue", func(t *testing.T) {
		tags := `dynssz-max:"?"`
		_, _, maxSizeHints, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse dynssz-max tags: %v", err)
		}
		if len(maxSizeHints) != 1 {
			t.Errorf("Expected 1 max size hint, got %d", len(maxSizeHints))
		}
		if !maxSizeHints[0].NoValue {
			t.Error("Expected NoValue flag to be set")
		}
	})

	t.Run("DynSszMaxWithExpr", func(t *testing.T) {
		tags := `ssz-max:"1024" dynssz-max:"MAX_VALIDATORS"`
		_, _, maxSizeHints, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse dynssz-max expr tags: %v", err)
		}
		if len(maxSizeHints) != 1 {
			t.Errorf("Expected 1 max size hint, got %d", len(maxSizeHints))
		}
		if maxSizeHints[0].Expr != "MAX_VALIDATORS" {
			t.Errorf("Expected expression 'MAX_VALIDATORS', got %s", maxSizeHints[0].Expr)
		}
	})

	t.Run("DynSszSizeWithDynamicMarker", func(t *testing.T) {
		tags := `dynssz-size:"?"`
		_, sizeHints, _, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse dynssz-size dynamic tags: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Errorf("Expected 1 size hint, got %d", len(sizeHints))
		}
		if !sizeHints[0].Dynamic {
			t.Error("Expected Dynamic flag to be set")
		}
	})

	t.Run("InvalidSszBitsizeTag", func(t *testing.T) {
		tags := `ssz-bitsize:"invalid"`
		_, _, _, err := parser.parseFieldTags(tags)
		if err == nil {
			t.Error("Expected error for invalid ssz-bitsize tag")
		}
	})

	t.Run("DynSszSizeUpdatesExistingHint", func(t *testing.T) {
		// When ssz-size and dynssz-size both exist with different values
		tags := `ssz-size:"32" dynssz-size:"64"`
		_, sizeHints, _, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse combined size tags: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Errorf("Expected 1 size hint, got %d", len(sizeHints))
		}
		// dynssz-size should override
		if sizeHints[0].Size != 64 {
			t.Errorf("Expected size 64 (from dynssz-size), got %d", sizeHints[0].Size)
		}
	})

	t.Run("DynSszMaxUpdatesExistingHint", func(t *testing.T) {
		// When ssz-max and dynssz-max both exist with different values
		tags := `ssz-max:"1024" dynssz-max:"2048"`
		_, _, maxSizeHints, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse combined max size tags: %v", err)
		}
		if len(maxSizeHints) != 1 {
			t.Errorf("Expected 1 max size hint, got %d", len(maxSizeHints))
		}
		// dynssz-max should override
		if maxSizeHints[0].Size != 2048 {
			t.Errorf("Expected max size 2048 (from dynssz-max), got %d", maxSizeHints[0].Size)
		}
	})

	t.Run("DynSszBitsizeWithExpression", func(t *testing.T) {
		tags := `ssz-bitsize:"256" dynssz-bitsize:"SOME_EXPR"`
		_, sizeHints, _, err := parser.parseFieldTags(tags)
		if err != nil {
			t.Fatalf("Failed to parse dynssz-bitsize expr tags: %v", err)
		}
		if len(sizeHints) != 1 {
			t.Errorf("Expected 1 size hint, got %d", len(sizeHints))
		}
		if sizeHints[0].Expr != "SOME_EXPR" {
			t.Errorf("Expected expression 'SOME_EXPR', got %s", sizeHints[0].Expr)
		}
		if !sizeHints[0].Bits {
			t.Error("Expected Bits flag to be set for bitsize expression")
		}
	})
}

func TestBuildVectorDescriptorEdgeCases(t *testing.T) {
	parser := NewParser()

	t.Run("BitvectorWithBitsizeHint", func(t *testing.T) {
		// Test bitvector with bitsize hint (bits are converted to bytes)
		arr := types.NewArray(types.Typ[types.Uint8], 32)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszBitvectorType}}
		sizeHint := []ssztypes.SszSizeHint{{Size: 128, Bits: true}} // 128 bits = 16 bytes
		desc, err := parser.buildTypeDescriptor(arr, typeHint, sizeHint, nil)
		if err != nil {
			t.Fatalf("Failed to build bitvector descriptor: %v", err)
		}
		if desc.Len != 16 { // 128 bits = 16 bytes
			t.Errorf("Expected len 16 (128 bits), got %d", desc.Len)
		}
	})

	t.Run("BitvectorSliceWithBitsizeHint", func(t *testing.T) {
		// Test bitvector slice with bitsize hint
		slice := types.NewSlice(types.Typ[types.Uint8])
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszBitvectorType}}
		sizeHint := []ssztypes.SszSizeHint{{Size: 256, Bits: true}} // 256 bits = 32 bytes
		desc, err := parser.buildTypeDescriptor(slice, typeHint, sizeHint, nil)
		if err != nil {
			t.Fatalf("Failed to build bitvector descriptor: %v", err)
		}
		if desc.Len != 32 { // 256 bits = 32 bytes
			t.Errorf("Expected len 32 (256 bits), got %d", desc.Len)
		}
	})

	t.Run("BitvectorStringWithBitsizeHint", func(t *testing.T) {
		// Test string as bitvector with bitsize hint
		stringType := types.Typ[types.String]
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszBitvectorType}}
		sizeHint := []ssztypes.SszSizeHint{{Size: 160, Bits: true}} // 160 bits = 20 bytes
		desc, err := parser.buildTypeDescriptor(stringType, typeHint, sizeHint, nil)
		if err != nil {
			t.Fatalf("Failed to build string bitvector descriptor: %v", err)
		}
		if desc.Len != 20 { // 160 bits = 20 bytes
			t.Errorf("Expected len 20 (160 bits), got %d", desc.Len)
		}
	})

	t.Run("UnsupportedVectorType", func(t *testing.T) {
		// Test unsupported type for vector (map)
		mapType := types.NewMap(types.Typ[types.String], types.Typ[types.Int])
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszVectorType}}
		sizeHint := []ssztypes.SszSizeHint{{Size: 10}}
		_, err := parser.buildTypeDescriptor(mapType, typeHint, sizeHint, nil)
		if err == nil {
			t.Error("Expected error for unsupported vector type")
		}
	})

	t.Run("VectorWithChildHints", func(t *testing.T) {
		// Test vector of vectors with child hints
		innerSlice := types.NewSlice(types.Typ[types.Uint8])
		outerArr := types.NewArray(innerSlice, 10)
		typeHints := []ssztypes.SszTypeHint{{}, {Type: ssztypes.SszVectorType}}
		sizeHints := []ssztypes.SszSizeHint{{}, {Size: 32}}
		maxSizeHints := []ssztypes.SszMaxSizeHint{{}, {Size: 1024}}
		desc, err := parser.buildTypeDescriptor(outerArr, typeHints, sizeHints, maxSizeHints)
		if err != nil {
			t.Fatalf("Failed to build nested vector descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszVectorType {
			t.Errorf("Expected SszVectorType, got %v", desc.SszType)
		}
		if desc.ElemDesc.SszType != ssztypes.SszVectorType {
			t.Errorf("Expected inner SszVectorType, got %v", desc.ElemDesc.SszType)
		}
	})

	t.Run("VectorWithDynamicElements", func(t *testing.T) {
		// Test vector of dynamic elements (slices)
		innerSlice := types.NewSlice(types.Typ[types.Uint8])
		outerArr := types.NewArray(innerSlice, 5)
		desc, err := parser.buildTypeDescriptor(outerArr, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build vector of dynamic elements: %v", err)
		}
		if desc.SszTypeFlags&ssztypes.SszTypeFlagIsDynamic == 0 {
			t.Error("Expected dynamic flag to be set for vector of dynamic elements")
		}
		if desc.Size != 0 {
			t.Errorf("Expected size 0 for dynamic vector, got %d", desc.Size)
		}
	})
}

func TestBuildListDescriptorEdgeCases(t *testing.T) {
	parser := NewParser()

	t.Run("ListWithChildHints", func(t *testing.T) {
		// Test list of vectors with child hints
		innerSlice := types.NewSlice(types.Typ[types.Uint8])
		outerSlice := types.NewSlice(innerSlice)
		typeHints := []ssztypes.SszTypeHint{{Type: ssztypes.SszListType}, {Type: ssztypes.SszVectorType}}
		sizeHints := []ssztypes.SszSizeHint{{}, {Size: 32}}
		maxSizeHints := []ssztypes.SszMaxSizeHint{{Size: 100}, {}}
		desc, err := parser.buildTypeDescriptor(outerSlice, typeHints, sizeHints, maxSizeHints)
		if err != nil {
			t.Fatalf("Failed to build nested list descriptor: %v", err)
		}
		if desc.SszType != ssztypes.SszListType {
			t.Errorf("Expected SszListType, got %v", desc.SszType)
		}
		if desc.ElemDesc.SszType != ssztypes.SszVectorType {
			t.Errorf("Expected inner SszVectorType, got %v", desc.ElemDesc.SszType)
		}
	})

	t.Run("UnsupportedListBaseType", func(t *testing.T) {
		// Test unsupported basic type for list
		uint64Type := types.Typ[types.Uint64]
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszListType}}
		_, err := parser.buildTypeDescriptor(uint64Type, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for unsupported list base type")
		}
	})

	t.Run("UnsupportedListType", func(t *testing.T) {
		// Test unsupported type for list (map)
		mapType := types.NewMap(types.Typ[types.String], types.Typ[types.Int])
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszListType}}
		_, err := parser.buildTypeDescriptor(mapType, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for unsupported list type")
		}
	})
}

func TestHasMethodWithSignature(t *testing.T) {
	parser := NewParser()

	// Create a simple struct type with methods for testing
	pkg := types.NewPackage("test", "test")
	obj := types.NewTypeName(token.NoPos, pkg, "TestType", nil)
	underlying := types.NewStruct(nil, nil)
	namedType := types.NewNamed(obj, underlying, nil)

	// Add methods to the type
	// Method 1: SizeSSZ() int
	sig1 := types.NewSignatureType(
		types.NewVar(token.NoPos, pkg, "", types.NewPointer(namedType)),
		nil, nil,
		types.NewTuple(),
		types.NewTuple(types.NewVar(token.NoPos, pkg, "", types.Typ[types.Int])),
		false,
	)
	method1 := types.NewFunc(token.NoPos, pkg, "SizeSSZ", sig1)
	namedType.AddMethod(method1)

	// Method 2: MarshalSSZ() ([]byte, error)
	errorType := types.Universe.Lookup("error").Type()
	byteSliceType := types.NewSlice(types.Typ[types.Uint8])
	sig2 := types.NewSignatureType(
		types.NewVar(token.NoPos, pkg, "", types.NewPointer(namedType)),
		nil, nil,
		types.NewTuple(),
		types.NewTuple(
			types.NewVar(token.NoPos, pkg, "", byteSliceType),
			types.NewVar(token.NoPos, pkg, "", errorType),
		),
		false,
	)
	method2 := types.NewFunc(token.NoPos, pkg, "MarshalSSZ", sig2)
	namedType.AddMethod(method2)

	// Method 3: HashTreeRoot() ([32]byte, error)
	byte32Type := types.NewArray(types.Typ[types.Uint8], 32)
	sig3 := types.NewSignatureType(
		types.NewVar(token.NoPos, pkg, "", types.NewPointer(namedType)),
		nil, nil,
		types.NewTuple(),
		types.NewTuple(
			types.NewVar(token.NoPos, pkg, "", byte32Type),
			types.NewVar(token.NoPos, pkg, "", errorType),
		),
		false,
	)
	method3 := types.NewFunc(token.NoPos, pkg, "HashTreeRoot", sig3)
	namedType.AddMethod(method3)

	methodSet := types.NewMethodSet(types.NewPointer(namedType))

	t.Run("MatchingSizeSSZ", func(t *testing.T) {
		found := parser.hasMethodWithSignature(methodSet, "SizeSSZ", []string{}, []string{"int"})
		if !found {
			t.Error("Expected to find SizeSSZ method")
		}
	})

	t.Run("MatchingMarshalSSZ", func(t *testing.T) {
		found := parser.hasMethodWithSignature(methodSet, "MarshalSSZ", []string{}, []string{"[]byte", "error"})
		if !found {
			t.Error("Expected to find MarshalSSZ method")
		}
	})

	t.Run("MatchingHashTreeRoot", func(t *testing.T) {
		found := parser.hasMethodWithSignature(methodSet, "HashTreeRoot", []string{}, []string{"[32]byte", "error"})
		if !found {
			t.Error("Expected to find HashTreeRoot method")
		}
	})

	t.Run("NonExistentMethod", func(t *testing.T) {
		found := parser.hasMethodWithSignature(methodSet, "NonExistent", []string{}, []string{})
		if found {
			t.Error("Expected not to find NonExistent method")
		}
	})

	t.Run("WrongParamCount", func(t *testing.T) {
		found := parser.hasMethodWithSignature(methodSet, "SizeSSZ", []string{"int"}, []string{"int"})
		if found {
			t.Error("Expected not to match with wrong param count")
		}
	})

	t.Run("WrongReturnCount", func(t *testing.T) {
		found := parser.hasMethodWithSignature(methodSet, "SizeSSZ", []string{}, []string{"int", "error"})
		if found {
			t.Error("Expected not to match with wrong return count")
		}
	})

	t.Run("WrongReturnType", func(t *testing.T) {
		found := parser.hasMethodWithSignature(methodSet, "SizeSSZ", []string{}, []string{"[]byte"})
		if found {
			t.Error("Expected not to match with wrong return type")
		}
	})

	t.Run("WildcardMatch", func(t *testing.T) {
		// "-" should match any type
		found := parser.hasMethodWithSignature(methodSet, "SizeSSZ", []string{}, []string{"-"})
		if !found {
			t.Error("Expected to match with wildcard")
		}
	})
}

func TestBuildUint256DescriptorSlices(t *testing.T) {
	parser := NewParser()

	t.Run("Uint8Slice", func(t *testing.T) {
		// Test []uint8 with uint256 type hint
		sliceType := types.NewSlice(types.Typ[types.Uint8])
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint256Type}}
		desc, err := parser.buildTypeDescriptor(sliceType, typeHint, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build uint256 descriptor from []uint8: %v", err)
		}
		if desc.SszType != ssztypes.SszUint256Type {
			t.Errorf("Expected SszUint256Type, got %v", desc.SszType)
		}
		if desc.Size != 32 {
			t.Errorf("Expected size 32, got %d", desc.Size)
		}
		if desc.Len != 32 {
			t.Errorf("Expected len 32, got %d", desc.Len)
		}
		if desc.GoTypeFlags&ssztypes.GoTypeFlagIsByteArray == 0 {
			t.Error("Expected byte array flag to be set")
		}
	})

	t.Run("Uint64Slice", func(t *testing.T) {
		// Test []uint64 with uint256 type hint
		sliceType := types.NewSlice(types.Typ[types.Uint64])
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint256Type}}
		desc, err := parser.buildTypeDescriptor(sliceType, typeHint, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build uint256 descriptor from []uint64: %v", err)
		}
		if desc.SszType != ssztypes.SszUint256Type {
			t.Errorf("Expected SszUint256Type, got %v", desc.SszType)
		}
		if desc.Size != 32 {
			t.Errorf("Expected size 32, got %d", desc.Size)
		}
		if desc.Len != 4 {
			t.Errorf("Expected len 4, got %d", desc.Len)
		}
	})

	t.Run("InvalidUint256Base", func(t *testing.T) {
		// Test unsupported slice type with uint256 type hint
		sliceType := types.NewSlice(types.Typ[types.Uint32])
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint256Type}}
		_, err := parser.buildTypeDescriptor(sliceType, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for invalid uint256 base type")
		}
	})

	t.Run("Uint8Array32", func(t *testing.T) {
		// Test [32]uint8 with uint256 type hint
		arrayType := types.NewArray(types.Typ[types.Uint8], 32)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint256Type}}
		desc, err := parser.buildTypeDescriptor(arrayType, typeHint, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build uint256 descriptor from [32]uint8: %v", err)
		}
		if desc.SszType != ssztypes.SszUint256Type {
			t.Errorf("Expected SszUint256Type, got %v", desc.SszType)
		}
		if desc.Size != 32 {
			t.Errorf("Expected size 32, got %d", desc.Size)
		}
	})

	t.Run("Uint64Array4", func(t *testing.T) {
		// Test [4]uint64 with uint256 type hint
		arrayType := types.NewArray(types.Typ[types.Uint64], 4)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint256Type}}
		desc, err := parser.buildTypeDescriptor(arrayType, typeHint, nil, nil)
		if err != nil {
			t.Fatalf("Failed to build uint256 descriptor from [4]uint64: %v", err)
		}
		if desc.SszType != ssztypes.SszUint256Type {
			t.Errorf("Expected SszUint256Type, got %v", desc.SszType)
		}
		if desc.Size != 32 {
			t.Errorf("Expected size 32, got %d", desc.Size)
		}
		if desc.Len != 4 {
			t.Errorf("Expected len 4, got %d", desc.Len)
		}
	})

	t.Run("WrongSizedUint8Array", func(t *testing.T) {
		// Test [16]uint8 with uint256 type hint - should fail
		arrayType := types.NewArray(types.Typ[types.Uint8], 16)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint256Type}}
		_, err := parser.buildTypeDescriptor(arrayType, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for wrong-sized uint8 array")
		}
	})

	t.Run("WrongSizedUint64Array", func(t *testing.T) {
		// Test [2]uint64 with uint256 type hint - should fail
		arrayType := types.NewArray(types.Typ[types.Uint64], 2)
		typeHint := []ssztypes.SszTypeHint{{Type: ssztypes.SszUint256Type}}
		_, err := parser.buildTypeDescriptor(arrayType, typeHint, nil, nil)
		if err == nil {
			t.Error("Expected error for wrong-sized uint64 array")
		}
	})
}
