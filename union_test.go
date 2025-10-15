// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"reflect"
	"testing"
)

func TestNewCompatibleUnion(t *testing.T) {
	// Define test types
	type ExecutionPayload struct {
		BlockHash []byte
		StateRoot []byte
	}
	
	type ExecutionPayloadWithBlobs struct {
		BlockHash []byte
		StateRoot []byte
		Blobs     [][]byte
	}
	
	// Define union descriptor - each field represents a variant type
	type UnionDescriptor struct {
		ExecutionPayload         ExecutionPayload
		ExecutionPayloadWithBlobs ExecutionPayloadWithBlobs
	}
	
	tests := []struct {
		name          string
		variantIndex  uint8
		data          interface{}
		expectError   bool
	}{
		{
			name:         "create union with first variant",
			variantIndex: 0,
			data: ExecutionPayload{
				BlockHash: []byte{1, 2, 3},
				StateRoot: []byte{4, 5, 6},
			},
			expectError: false,
		},
		{
			name:         "create union with second variant",
			variantIndex: 1,
			data: ExecutionPayloadWithBlobs{
				BlockHash: []byte{1, 2, 3},
				StateRoot: []byte{4, 5, 6},
				Blobs:     [][]byte{{7, 8, 9}},
			},
			expectError: false,
		},
		{
			name:         "create union with nil data",
			variantIndex: 0,
			data:         nil,
			expectError:  false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			union, err := NewCompatibleUnion[UnionDescriptor](tt.variantIndex, tt.data)
			
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			
			if union == nil {
				t.Fatal("union should not be nil")
			}
			
			if union.Variant != tt.variantIndex {
				t.Errorf("variant mismatch: got %d, want %d", union.Variant, tt.variantIndex)
			}
			
			if !reflect.DeepEqual(union.Data, tt.data) {
				t.Errorf("data mismatch: got %v, want %v", union.Data, tt.data)
			}
		})
	}
}

func TestCompatibleUnionGetDescriptorType(t *testing.T) {
	type TestVariantA struct {
		FieldA uint64
	}
	
	type TestVariantB struct {
		FieldB string
	}
	
	type TestUnionDescriptor struct {
		TestVariantA TestVariantA
		TestVariantB TestVariantB
	}
	
	union := &CompatibleUnion[TestUnionDescriptor]{
		Variant: 0,
		Data:    TestVariantA{FieldA: 42},
	}
	
	descriptorType := union.GetDescriptorType()
	
	if descriptorType == nil {
		t.Fatal("GetDescriptorType() returned nil")
	}
	
	if descriptorType.Kind() != reflect.Struct {
		t.Errorf("descriptor type should be struct, got %v", descriptorType.Kind())
	}
	
	if descriptorType.Name() != "TestUnionDescriptor" {
		t.Errorf("descriptor type name mismatch: got %v, want TestUnionDescriptor", descriptorType.Name())
	}
	
	// Verify it has the expected fields
	if descriptorType.NumField() != 2 {
		t.Errorf("descriptor should have 2 fields, got %d", descriptorType.NumField())
	}
}

func TestExtractUnionDescriptorInfo(t *testing.T) {
	dynssz := NewDynSsz(map[string]any{})
	
	tests := []struct {
		name           string
		descriptorType reflect.Type
		expectError    bool
		errorContains  string
		validateInfo   func(*testing.T, map[uint8]UnionVariantInfo)
	}{
		{
			name: "valid union descriptor",
			descriptorType: reflect.TypeOf(struct {
				VariantA struct {
					Field []byte `ssz-size:"32"`
				}
				VariantB struct {
					Field []uint64 `ssz-max:"1024"`
				}
			}{}),
			expectError: false,
			validateInfo: func(t *testing.T, info map[uint8]UnionVariantInfo) {
				if len(info) != 2 {
					t.Errorf("expected 2 variants, got %d", len(info))
				}
				
				// Check that both variants exist
				if _, ok := info[0]; !ok {
					t.Error("variant 0 not found")
				}
				if _, ok := info[1]; !ok {
					t.Error("variant 1 not found")
				}
			},
		},
		{
			name: "union with type hints",
			descriptorType: reflect.TypeOf(struct {
				VariantA struct {
					Field uint64 `ssz-type:"uint64"`
				}
			}{}),
			expectError: false,
			validateInfo: func(t *testing.T, info map[uint8]UnionVariantInfo) {
				if _, ok := info[0]; !ok {
					t.Error("variant 0 not found")
				}
			},
		},
		{
			name:           "non-struct descriptor",
			descriptorType: reflect.TypeOf("not a struct"),
			expectError:    true,
			errorContains:  "union descriptor must be a struct",
		},
		{
			name:           "empty union descriptor",
			descriptorType: reflect.TypeOf(struct{}{}),
			expectError:    true,
			errorContains:  "union descriptor struct has no fields",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ExtractUnionDescriptorInfo(tt.descriptorType, dynssz)
			
			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("error should contain %q, got %v", tt.errorContains, err)
				}
				return
			}
			
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			
			if info == nil {
				t.Fatal("info should not be nil")
			}
			
			if tt.validateInfo != nil {
				tt.validateInfo(t, info)
			}
		})
	}
}

func TestCompatibleUnionWithComplexTypes(t *testing.T) {
	t.Run("union with embedded structs", func(t *testing.T) {
		type BasePayload struct {
			Timestamp uint64
			BaseFee   []byte
		}
		
		type ExtendedPayload struct {
			BasePayload
			ExtraData []byte
		}
		
		type PayloadUnion struct {
			BasePayload     BasePayload
			ExtendedPayload ExtendedPayload
		}
		
		// Create union with base variant
		baseData := BasePayload{
			Timestamp: 12345,
			BaseFee:   []byte{1, 2, 3},
		}
		
		union, err := NewCompatibleUnion[PayloadUnion](0, baseData)
		if err != nil {
			t.Fatalf("failed to create union: %v", err)
		}
		
		if union.Variant != 0 {
			t.Error("variant mismatch")
		}
		
		if data, ok := union.Data.(BasePayload); ok {
			if data.Timestamp != 12345 {
				t.Error("timestamp mismatch")
			}
		} else {
			t.Error("data type assertion failed")
		}
	})
	
	t.Run("union with slice variants", func(t *testing.T) {
		type SliceUnion struct {
			ByteSlice   []byte   `ssz-max:"100"`
			Uint64Slice []uint64 `ssz-max:"50"`
			StringSlice []string `ssz-max:"25"`
		}
		
		// Test each variant
		variants := []struct {
			index uint8
			data  interface{}
		}{
			{0, []byte{1, 2, 3, 4, 5}},
			{1, []uint64{10, 20, 30}},
			{2, []string{"hello", "world"}},
		}
		
		for _, v := range variants {
			union, err := NewCompatibleUnion[SliceUnion](v.index, v.data)
			if err != nil {
				t.Errorf("failed to create union for variant %d: %v", v.index, err)
				continue
			}
			
			if union.Variant != v.index {
				t.Errorf("variant mismatch for index %d", v.index)
			}
			
			if !reflect.DeepEqual(union.Data, v.data) {
				t.Errorf("data mismatch for variant %d", v.index)
			}
		}
	})
}

func TestCompatibleUnionVariantIndexing(t *testing.T) {
	// Test that variant indices are assigned based on field order
	type OrderedUnion struct {
		First  struct{ Value uint8 }
		Second struct{ Value uint16 }
		Third  struct{ Value uint32 }
		Fourth struct{ Value uint64 }
	}
	
	dynssz := NewDynSsz(map[string]any{})
	info, err := ExtractUnionDescriptorInfo(reflect.TypeOf(OrderedUnion{}), dynssz)
	if err != nil {
		t.Fatalf("failed to extract union info: %v", err)
	}
	
	// Verify that indices 0-3 are present
	for i := uint8(0); i < 4; i++ {
		if _, ok := info[i]; !ok {
			t.Errorf("expected variant at index %d", i)
		}
	}
	
	// Verify field types match expected order
	expectedKinds := []reflect.Kind{
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
	}
	
	for i, expectedKind := range expectedKinds {
		variant := info[uint8(i)]
		if variant.Type.Kind() != reflect.Struct {
			t.Errorf("variant %d should be struct", i)
			continue
		}
		
		if variant.Type.NumField() != 1 {
			t.Errorf("variant %d should have 1 field", i)
			continue
		}
		
		field := variant.Type.Field(0)
		if field.Type.Kind() != expectedKind {
			t.Errorf("variant %d field type mismatch: got %v, want %v", i, field.Type.Kind(), expectedKind)
		}
	}
}

func TestCompatibleUnionEdgeCases(t *testing.T) {
	t.Run("union with single variant", func(t *testing.T) {
		type SingleVariantUnion struct {
			OnlyVariant struct {
				Data string
			}
		}
		
		union, err := NewCompatibleUnion[SingleVariantUnion](0, struct{ Data string }{Data: "test"})
		if err != nil {
			t.Fatalf("failed to create union: %v", err)
		}
		
		if union.Variant != 0 {
			t.Error("variant should be 0 for single variant union")
		}
	})
	
	t.Run("union variant switching", func(t *testing.T) {
		type SwitchableUnion struct {
			TypeA struct{ A int }
			TypeB struct{ B string }
		}
		
		// Start with TypeA
		union, err := NewCompatibleUnion[SwitchableUnion](0, struct{ A int }{A: 42})
		if err != nil {
			t.Fatalf("failed to create union: %v", err)
		}
		
		// Switch to TypeB
		union.Variant = 1
		union.Data = struct{ B string }{B: "switched"}
		
		if union.Variant != 1 {
			t.Error("variant should be updated to 1")
		}
		
		if data, ok := union.Data.(struct{ B string }); ok {
			if data.B != "switched" {
				t.Error("data not properly switched")
			}
		} else {
			t.Error("data type assertion failed after switch")
		}
	})
	
	t.Run("union descriptor type caching", func(t *testing.T) {
		type CachedUnion struct {
			A struct{ Field uint64 }
			B struct{ Field string }
		}
		
		union1 := &CompatibleUnion[CachedUnion]{Variant: 0}
		union2 := &CompatibleUnion[CachedUnion]{Variant: 1}
		
		type1 := union1.GetDescriptorType()
		type2 := union2.GetDescriptorType()
		
		// Both should return the same descriptor type
		if type1 != type2 {
			t.Error("GetDescriptorType should return same type for same union descriptor")
		}
	})
	
	t.Run("union with anonymous fields", func(t *testing.T) {
		type AnonymousUnion struct {
			VariantA struct {
				X int
				Y int
			}
			VariantB struct {
				A string
				B string
			}
		}
		
		dynssz := NewDynSsz(map[string]any{})
		info, err := ExtractUnionDescriptorInfo(reflect.TypeOf(AnonymousUnion{}), dynssz)
		if err != nil {
			t.Fatalf("failed to extract union info: %v", err)
		}
		
		if len(info) != 2 {
			t.Errorf("expected 2 variants, got %d", len(info))
		}
		
		// Both variants should be embedded structs
		for i := uint8(0); i < 2; i++ {
			if variant, ok := info[i]; ok {
				if variant.Type.Kind() != reflect.Struct {
					t.Errorf("variant %d should be struct", i)
				}
			}
		}
	})
}