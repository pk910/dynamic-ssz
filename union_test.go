// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
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
		ExecutionPayload          ExecutionPayload
		ExecutionPayloadWithBlobs ExecutionPayloadWithBlobs
	}

	tests := []struct {
		name         string
		variantIndex uint8
		data         interface{}
		expectError  bool
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

type TestInvalidUnion1 struct{}

func (t *TestInvalidUnion1) GetDescriptorType() {}

type TestInvalidUnion2 struct{}

func (t *TestInvalidUnion2) GetDescriptorType() uint64 {
	return 0
}

type TestInvalidUnion3 struct{}

func (t *TestInvalidUnion3) GetDescriptorType() reflect.Type {
	return reflect.TypeOf(uint64(0))
}

// Test types with invalid HashTreeRootWith method
func TestTypeCache_InvalidUnionTypes(t *testing.T) {
	ds := NewDynSsz(nil)

	tests := []struct {
		name          string
		typ           reflect.Type
		expectedError string
	}{
		{
			name: "invalid union type 1",
			typ: reflect.TypeOf(TypeWrapper[struct {
				F struct{} `ssz-type:"union"`
			}, struct{}]{}),
			expectedError: "GetDescriptorType method not found on type",
		},
		{
			name: "invalid union type 2",
			typ: reflect.TypeOf(TypeWrapper[struct {
				F TestInvalidUnion1 `ssz-type:"union"`
			}, TestInvalidUnion1]{}),
			expectedError: "GetDescriptorType returned no results",
		},
		{
			name: "invalid union type 3",
			typ: reflect.TypeOf(TypeWrapper[struct {
				F TestInvalidUnion2 `ssz-type:"union"`
			}, TestInvalidUnion2]{}),
			expectedError: "GetDescriptorType did not return a reflect.Type",
		},
		{
			name: "invalid union type 4",
			typ: reflect.TypeOf(TypeWrapper[struct {
				F TestInvalidUnion3 `ssz-type:"union"`
			}, TestInvalidUnion3]{}),
			expectedError: "union descriptor must be a struct",
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
}

// TestZeroUnionMarshalDoesNotPanic verifies that a zero-value CompatibleUnion
// (nil Data interface) returns a clean error across marshal, stream marshal and
// HTR instead of panicking on the zero reflect.Value.
func TestZeroUnionMarshalDoesNotPanic(t *testing.T) {
	type T struct {
		U CompatibleUnion[struct {
			V0 uint32
			V1 [16]byte
		}]
	}

	ds := NewDynSsz(nil)

	t.Run("buffer", func(t *testing.T) {
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("zero union marshal panicked: %v", r)
				}
			}()
			_, err = ds.MarshalSSZ(&T{})
		}()
		if err == nil {
			t.Fatalf("expected error for zero-value union marshal")
		}
		if !errors.Is(err, sszutils.ErrInvalidValueRange) {
			t.Fatalf("expected ErrInvalidValueRange, got %v", err)
		}
	})

	t.Run("stream", func(t *testing.T) {
		var sb bytes.Buffer
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("zero union stream marshal panicked: %v", r)
				}
			}()
			err = ds.MarshalSSZWriter(&T{}, &sb)
		}()
		if err == nil {
			t.Fatalf("expected error for zero-value union stream marshal")
		}
		if !errors.Is(err, sszutils.ErrInvalidValueRange) {
			t.Fatalf("expected ErrInvalidValueRange, got %v", err)
		}
	})

	t.Run("htr", func(t *testing.T) {
		_, err := ds.HashTreeRoot(&T{})
		if err == nil {
			t.Fatalf("expected error for zero-value union HTR")
		}
		if !errors.Is(err, sszutils.ErrInvalidValueRange) {
			t.Fatalf("expected ErrInvalidValueRange, got %v", err)
		}
	})

	// A zero-value union whose selected variant is a dynamic type would panic in
	// the size path (getSszValueSize -> Len() on a zero reflect.Value) rather than
	// the marshal path. This exercises the size-path nil guard independently.
	t.Run("sizeDynamicVariant", func(t *testing.T) {
		type D struct {
			U CompatibleUnion[struct {
				V0 []uint64 `ssz-max:"8"`
				V1 uint32
			}]
		}

		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("zero union with dynamic variant panicked during sizing: %v", r)
				}
			}()
			_, err = ds.MarshalSSZ(&D{}) // Variant 0 (dynamic []uint64), Data nil
		}()
		if err == nil {
			t.Fatalf("expected error for zero-value union with dynamic variant")
		}
		if !errors.Is(err, sszutils.ErrInvalidValueRange) {
			t.Fatalf("expected ErrInvalidValueRange, got %v", err)
		}
	})

	// A top-level union streamed via MarshalSSZWriter reaches marshalCompatibleUnion
	// directly (no prior size/offset computation), exercising the marshal-path nil
	// guard rather than the size-path guard hit by the nested cases above.
	t.Run("streamTopLevel", func(t *testing.T) {
		u := &CompatibleUnion[struct {
			V0 uint32
			V1 [16]byte
		}]{} // Variant 0, Data nil

		var sb bytes.Buffer
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("top-level zero union stream marshal panicked: %v", r)
				}
			}()
			err = ds.MarshalSSZWriter(u, &sb)
		}()
		if err == nil {
			t.Fatalf("expected error for top-level zero-value union stream marshal")
		}
		if !errors.Is(err, sszutils.ErrInvalidValueRange) {
			t.Fatalf("expected ErrInvalidValueRange, got %v", err)
		}
	})
}
