// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package codegen

import (
	"go/types"
	"testing"

	ssz "github.com/pk910/dynamic-ssz"
)

func TestParser(t *testing.T) {
	// Create a parser instance
	parser := NewParser()

	// Test basic types
	t.Run("BasicTypes", func(t *testing.T) {
		// Test uint64
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
		if desc.SszType != ssz.SszUint64Type {
			t.Errorf("Expected SszUint64Type, got %v", desc.SszType)
		}

		// Test bool
		boolType := types.Typ[types.Bool]
		desc, err = parser.GetTypeDescriptor(boolType, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to get bool descriptor: %v", err)
		}
		if desc == nil {
			t.Fatal("Expected non-nil descriptor")
		}
		if desc.Size != 1 {
			t.Errorf("Expected size 1, got %d", desc.Size)
		}
		if desc.SszType != ssz.SszBoolType {
			t.Errorf("Expected SszBoolType, got %v", desc.SszType)
		}
	})

	// Test slice types
	t.Run("SliceTypes", func(t *testing.T) {
		// Create []byte type
		byteType := types.Typ[types.Uint8]
		byteSlice := types.NewSlice(byteType)

		desc, err := parser.GetTypeDescriptor(byteSlice, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to get byte slice descriptor: %v", err)
		}
		if desc == nil {
			t.Fatal("Expected non-nil descriptor")
		}
		if desc.SszType != ssz.SszListType {
			t.Errorf("Expected SszListType, got %v", desc.SszType)
		}
		if desc.SszTypeFlags&ssz.SszTypeFlagIsDynamic == 0 {
			t.Error("Expected dynamic flag to be true")
		}
	})

	// Test array types
	t.Run("ArrayTypes", func(t *testing.T) {
		// Create [32]byte type
		byteType := types.Typ[types.Uint8]
		byteArray := types.NewArray(byteType, 32)

		desc, err := parser.GetTypeDescriptor(byteArray, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to get byte array descriptor: %v", err)
		}
		if desc == nil {
			t.Fatal("Expected non-nil descriptor")
		}
		if desc.SszType != ssz.SszVectorType {
			t.Errorf("Expected SszVectorType, got %v", desc.SszType)
		}
		if desc.Size != 32 {
			t.Errorf("Expected size 32, got %d", desc.Size)
		}
		if desc.Len != 32 {
			t.Errorf("Expected len 32, got %d", desc.Len)
		}
	})

	// Test pointer types
	t.Run("PointerTypes", func(t *testing.T) {
		// Create *uint64 type
		uint64Type := types.Typ[types.Uint64]
		ptrType := types.NewPointer(uint64Type)

		desc, err := parser.GetTypeDescriptor(ptrType, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to get pointer type descriptor: %v", err)
		}
		if desc == nil {
			t.Fatal("Expected non-nil descriptor")
		}
		if desc.SszType != ssz.SszUint64Type {
			t.Errorf("Expected SszUint64Type, got %v", desc.SszType)
		}
		if desc.GoTypeFlags&ssz.GoTypeFlagIsPointer == 0 {
			t.Error("Expected pointer flag to be set")
		}
	})
}
