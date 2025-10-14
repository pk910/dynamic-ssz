// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"reflect"
	"testing"
)

func TestNewTypeWrapper(t *testing.T) {
	type TestDescriptor struct {
		Data []byte `ssz-size:"32"`
	}
	
	testData := []byte{1, 2, 3, 4}
	wrapper, err := NewTypeWrapper[TestDescriptor, []byte](testData)
	
	if err != nil {
		t.Fatalf("NewTypeWrapper returned error: %v", err)
	}
	
	if wrapper == nil {
		t.Fatal("wrapper should not be nil")
	}
	
	if !reflect.DeepEqual(wrapper.Data, testData) {
		t.Errorf("wrapper data mismatch: got %v, want %v", wrapper.Data, testData)
	}
}

func TestTypeWrapperGet(t *testing.T) {
	type TestDescriptor struct {
		Data string `ssz-size:"64"`
	}
	
	testData := "hello world"
	wrapper := &TypeWrapper[TestDescriptor, string]{
		Data: testData,
	}
	
	result := wrapper.Get()
	
	if result != testData {
		t.Errorf("Get() returned %v, want %v", result, testData)
	}
}

func TestTypeWrapperSet(t *testing.T) {
	type TestDescriptor struct {
		Data int `ssz-type:"uint64"`
	}
	
	wrapper := &TypeWrapper[TestDescriptor, int]{
		Data: 42,
	}
	
	newValue := 100
	wrapper.Set(newValue)
	
	if wrapper.Data != newValue {
		t.Errorf("Set() did not update value: got %v, want %v", wrapper.Data, newValue)
	}
}

func TestTypeWrapperGetDescriptorType(t *testing.T) {
	type TestDescriptor struct {
		Data []uint64 `ssz-max:"1024"`
	}
	
	wrapper := &TypeWrapper[TestDescriptor, []uint64]{}
	
	descriptorType := wrapper.GetDescriptorType()
	
	if descriptorType == nil {
		t.Fatal("GetDescriptorType() returned nil")
	}
	
	if descriptorType.Kind() != reflect.Struct {
		t.Errorf("descriptor type should be struct, got %v", descriptorType.Kind())
	}
	
	if descriptorType.Name() != "TestDescriptor" {
		t.Errorf("descriptor type name mismatch: got %v, want TestDescriptor", descriptorType.Name())
	}
}

func TestExtractWrapperDescriptorInfo(t *testing.T) {
	dynssz := NewDynSsz(map[string]any{})
	
	tests := []struct {
		name            string
		descriptorType  reflect.Type
		expectError     bool
		errorContains   string
		validateInfo    func(*testing.T, *wrapperDescriptorInfo)
	}{
		{
			name: "valid descriptor with size hint",
			descriptorType: reflect.TypeOf(struct {
				Data []byte `ssz-size:"32"`
			}{}),
			expectError: false,
			validateInfo: func(t *testing.T, info *wrapperDescriptorInfo) {
				if info.Type.Kind() != reflect.Slice {
					t.Error("expected slice type")
				}
			},
		},
		{
			name: "valid descriptor with max size hint",
			descriptorType: reflect.TypeOf(struct {
				Data []uint64 `ssz-max:"1024"`
			}{}),
			expectError: false,
			validateInfo: func(t *testing.T, info *wrapperDescriptorInfo) {
				if info.Type.Kind() != reflect.Slice {
					t.Error("expected slice type")
				}
			},
		},
		{
			name: "valid descriptor with type hint",
			descriptorType: reflect.TypeOf(struct {
				Data uint64 `ssz-type:"uint64"`
			}{}),
			expectError: false,
			validateInfo: func(t *testing.T, info *wrapperDescriptorInfo) {
				if info.Type.Kind() != reflect.Uint64 {
					t.Error("expected uint64 type")
				}
			},
		},
		{
			name:           "non-struct descriptor",
			descriptorType: reflect.TypeOf(42),
			expectError:    true,
			errorContains:  "wrapper descriptor must be a struct",
		},
		{
			name: "descriptor with no fields",
			descriptorType: reflect.TypeOf(struct{}{}),
			expectError:    true,
			errorContains:  "wrapper descriptor must have exactly 1 field",
		},
		{
			name: "descriptor with multiple fields",
			descriptorType: reflect.TypeOf(struct {
				Field1 []byte
				Field2 string
			}{}),
			expectError:   true,
			errorContains: "wrapper descriptor must have exactly 1 field",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := extractWrapperDescriptorInfo(tt.descriptorType, dynssz)
			
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

func TestTypeWrapperWithComplexTypes(t *testing.T) {
	t.Run("wrapper with byte array", func(t *testing.T) {
		type ByteArrayDescriptor struct {
			Data [32]byte `ssz-size:"32"`
		}
		
		var data [32]byte
		for i := range data {
			data[i] = byte(i)
		}
		
		wrapper, err := NewTypeWrapper[ByteArrayDescriptor, [32]byte](data)
		if err != nil {
			t.Fatalf("NewTypeWrapper error: %v", err)
		}
		
		retrieved := wrapper.Get()
		if retrieved != data {
			t.Error("Get() returned different array")
		}
	})
	
	t.Run("wrapper with slice", func(t *testing.T) {
		type SliceDescriptor struct {
			Data []uint64 `ssz-max:"100"`
		}
		
		data := []uint64{1, 2, 3, 4, 5}
		wrapper, err := NewTypeWrapper[SliceDescriptor, []uint64](data)
		if err != nil {
			t.Fatalf("NewTypeWrapper error: %v", err)
		}
		
		wrapper.Set([]uint64{10, 20, 30})
		if len(wrapper.Get()) != 3 {
			t.Error("Set() failed to update slice")
		}
	})
	
	t.Run("wrapper with struct", func(t *testing.T) {
		type InnerStruct struct {
			A uint64
			B string
		}
		
		type StructDescriptor struct {
			Data InnerStruct
		}
		
		data := InnerStruct{A: 42, B: "test"}
		wrapper, err := NewTypeWrapper[StructDescriptor, InnerStruct](data)
		if err != nil {
			t.Fatalf("NewTypeWrapper error: %v", err)
		}
		
		if wrapper.Get().A != 42 || wrapper.Get().B != "test" {
			t.Error("struct data mismatch")
		}
	})
}

func TestTypeWrapperEdgeCases(t *testing.T) {
	t.Run("wrapper with nil slice", func(t *testing.T) {
		type NilSliceDescriptor struct {
			Data []byte `ssz-max:"100"`
		}
		
		var nilSlice []byte
		wrapper, err := NewTypeWrapper[NilSliceDescriptor, []byte](nilSlice)
		if err != nil {
			t.Fatalf("NewTypeWrapper error: %v", err)
		}
		
		if wrapper.Get() != nil {
			t.Error("expected nil slice")
		}
		
		wrapper.Set([]byte{1, 2, 3})
		if len(wrapper.Get()) != 3 {
			t.Error("Set() should work with previously nil slice")
		}
	})
	
	t.Run("wrapper with zero value", func(t *testing.T) {
		type ZeroDescriptor struct {
			Data uint64
		}
		
		wrapper, err := NewTypeWrapper[ZeroDescriptor, uint64](0)
		if err != nil {
			t.Fatalf("NewTypeWrapper error: %v", err)
		}
		
		if wrapper.Get() != 0 {
			t.Error("expected zero value")
		}
	})
	
	t.Run("wrapper descriptor type caching", func(t *testing.T) {
		type CacheDescriptor struct {
			Data string `ssz-size:"100"`
		}
		
		wrapper1 := &TypeWrapper[CacheDescriptor, string]{Data: "test1"}
		wrapper2 := &TypeWrapper[CacheDescriptor, string]{Data: "test2"}
		
		type1 := wrapper1.GetDescriptorType()
		type2 := wrapper2.GetDescriptorType()
		
		// Both should return the same type
		if type1 != type2 {
			t.Error("GetDescriptorType should return same type for same descriptor")
		}
	})
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || 
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr) >= 0))
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}