// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"reflect"
	"strings"
	"testing"
)

func TestNewTypeWrapper(t *testing.T) {
	type TestDescriptor struct {
		Data []byte `ssz-size:"32"`
	}

	testData := []byte{1, 2, 3, 4}
	wrapper, err := NewTypeWrapper[TestDescriptor](testData)

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

type TestInvalidTypeWrapper1 struct{}

func (t *TestInvalidTypeWrapper1) GetDescriptorType() {}

type TestInvalidTypeWrapper2 struct{}

func (t *TestInvalidTypeWrapper2) GetDescriptorType() uint64 {
	return 0
}

// Test types with invalid TypeWrapper method
func InvalidTypeWrapperTypes(t *testing.T) {
	ds := NewDynSsz(nil)

	tests := []struct {
		name          string
		typ           reflect.Type
		expectedError string
	}{
		{
			name: "invalid type wrapper type 1",
			typ: reflect.TypeOf(TypeWrapper[struct {
				F struct{} `ssz-type:"wrapper"`
			}, struct{}]{}),
			expectedError: "GetDescriptorType method not found on type",
		},
		{
			name: "invalid type wrapper type 2",
			typ: reflect.TypeOf(TypeWrapper[struct {
				F TestInvalidTypeWrapper1 `ssz-type:"wrapper"`
			}, TestInvalidTypeWrapper1]{}),
			expectedError: "GetDescriptorType returned no results",
		},
		{
			name: "invalid type wrapper type 3",
			typ: reflect.TypeOf(TypeWrapper[struct {
				F TestInvalidTypeWrapper2 `ssz-type:"wrapper"`
			}, TestInvalidTypeWrapper2]{}),
			expectedError: "GetDescriptorType did not return a reflect.Type",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ds.GetTypeCache().GetTypeDescriptor(test.typ, nil, nil, nil)
			if err == nil {
				t.Errorf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), test.expectedError) {
				t.Errorf("expected error %q, got %q", test.expectedError, err.Error())
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

		wrapper, err := NewTypeWrapper[ByteArrayDescriptor](data)
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
		wrapper, err := NewTypeWrapper[SliceDescriptor](data)
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
		wrapper, err := NewTypeWrapper[StructDescriptor](data)
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
		wrapper, err := NewTypeWrapper[NilSliceDescriptor](nilSlice)
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
