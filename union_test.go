// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package dynssz

import (
	"bytes"
	"crypto/sha256"
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
			variantIndex: 1,
			data: ExecutionPayload{
				BlockHash: []byte{1, 2, 3},
				StateRoot: []byte{4, 5, 6},
			},
			expectError: false,
		},
		{
			name:         "create union with second variant",
			variantIndex: 2,
			data: ExecutionPayloadWithBlobs{
				BlockHash: []byte{1, 2, 3},
				StateRoot: []byte{4, 5, 6},
				Blobs:     [][]byte{{7, 8, 9}},
			},
			expectError: false,
		},
		{
			name:         "create union with nil data",
			variantIndex: 1,
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
		Variant: 1,
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

		union, err := NewCompatibleUnion[PayloadUnion](1, baseData)
		if err != nil {
			t.Fatalf("failed to create union: %v", err)
		}

		if union.Variant != 1 {
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
			{1, []byte{1, 2, 3, 4, 5}},
			{2, []uint64{10, 20, 30}},
			{3, []string{"hello", "world"}},
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

		union, err := NewCompatibleUnion[SingleVariantUnion](1, struct{ Data string }{Data: "test"})
		if err != nil {
			t.Fatalf("failed to create union: %v", err)
		}

		if union.Variant != 1 {
			t.Error("variant should be 1 for single variant union")
		}
	})

	t.Run("union variant switching", func(t *testing.T) {
		type SwitchableUnion struct {
			TypeA struct{ A int }
			TypeB struct{ B string }
		}

		// Start with TypeA
		union, err := NewCompatibleUnion[SwitchableUnion](1, struct{ A int }{A: 42})
		if err != nil {
			t.Fatalf("failed to create union: %v", err)
		}

		// Switch to TypeB
		union.Variant = 2
		union.Data = struct{ B string }{B: "switched"}

		if union.Variant != 2 {
			t.Error("variant should be updated to 2")
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

		union1 := &CompatibleUnion[CachedUnion]{Variant: 1}
		union2 := &CompatibleUnion[CachedUnion]{Variant: 2}

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

// TestUnionDataTypeMismatch verifies that a CompatibleUnion whose Data holds a
// value of the wrong concrete type returns a clean error across marshal/HTR/size
// instead of panicking inside the typed marshaler.
func TestUnionDataTypeMismatch(t *testing.T) {
	type T struct {
		U CompatibleUnion[struct {
			Variant0 uint32
			Variant1 [16]byte
		}]
	}
	ds := NewDynSsz(nil)

	check := func(name string, fn func() error) {
		t.Helper()
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s panicked: %v", name, r)
				}
			}()
			err = fn()
		}()
		if err == nil {
			t.Fatalf("%s: expected error", name)
		}
		if !errors.Is(err, sszutils.ErrInvalidValueRange) {
			t.Fatalf("%s: expected ErrInvalidValueRange, got %v", name, err)
		}
	}

	// the first variant expects uint32, Data is a string
	bad := &T{}
	bad.U.Variant = 1
	bad.U.Data = "not a uint32"
	check("marshal", func() error { _, e := ds.MarshalSSZ(bad); return e })
	check("htr", func() error { _, e := ds.HashTreeRoot(bad); return e })

	// the second variant expects [16]byte, Data is uint32
	bad2 := &T{}
	bad2.U.Variant = 2
	bad2.U.Data = uint32(42)
	check("marshal2", func() error { _, e := ds.MarshalSSZ(bad2); return e })
	check("htr2", func() error { _, e := ds.HashTreeRoot(bad2); return e })

	// A dynamic variant exercises the size-path guard: without it, sizing the
	// wrong-typed value would call Len() on a non-slice and panic.
	type D struct {
		U CompatibleUnion[struct {
			Variant0 []uint64 `ssz-max:"8"`
			Variant1 uint32
		}]
	}
	dynBad := &D{}
	dynBad.U.Variant = 1 // expects []uint64
	dynBad.U.Data = uint32(42)
	check("size-dynvariant", func() error { _, e := ds.MarshalSSZ(dynBad); return e })

	// A valid selector with nil data exercises the size- and HTR-path nil
	// guards directly (the zero value stops earlier at the unassigned
	// selector 0).
	nilData := &T{}
	nilData.U.Variant = 1
	check("size-nildata", func() error { _, e := ds.MarshalSSZ(nilData); return e })
	check("htr-nildata", func() error { _, e := ds.HashTreeRoot(nilData); return e })
}

// Union variant fields may carry ssz-index tags to assign explicit selector
// values (e.g. 1-based selectors for EIP-8016 conformance).
func TestCompatibleUnionExplicitSelectors(t *testing.T) {
	type tagged struct {
		U CompatibleUnion[struct {
			F1 uint32 `ssz-index:"1"`
			F2 uint64 `ssz-index:"2"`
		}]
	}

	ds := NewDynSsz(nil)

	v := tagged{}
	v.U.Variant = 1
	v.U.Data = uint32(7)

	enc, err := ds.MarshalSSZ(&v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// offset (4) + selector byte 1 + uint32 payload
	if len(enc) != 9 || enc[4] != 1 {
		t.Fatalf("unexpected encoding: %x", enc)
	}

	var back tagged
	if err := ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.U.Variant != 1 || back.U.Data != uint32(7) {
		t.Fatalf("unexpected roundtrip result: %+v", back.U)
	}

	// selector 0 is not assigned in this union and must be rejected
	bad := append([]byte{}, enc...)
	bad[4] = 0
	var b2 tagged
	if err := ds.UnmarshalSSZ(&b2, bad); err == nil {
		t.Fatal("expected error for unassigned selector 0")
	}
}

// Duplicate selectors and partially tagged unions must be rejected.
func TestCompatibleUnionSelectorValidation(t *testing.T) {
	type dupe struct {
		U CompatibleUnion[struct {
			F1 uint32 `ssz-index:"1"`
			F2 uint64 `ssz-index:"1"`
		}]
	}
	ds := NewDynSsz(nil)
	if _, err := ds.MarshalSSZ(&dupe{}); err == nil || !strings.Contains(err.Error(), "duplicate union selector") {
		t.Errorf("expected duplicate selector error, got: %v", err)
	}

	type mixed struct {
		U CompatibleUnion[struct {
			F1 uint32 `ssz-index:"1"`
			F2 uint64
		}]
	}
	ds2 := NewDynSsz(nil)
	if _, err := ds2.MarshalSSZ(&mixed{}); err == nil || !strings.Contains(err.Error(), "ssz-index") {
		t.Errorf("expected mixed ssz-index error, got: %v", err)
	}
}

// Default selectors follow field order starting at 1 per EIP-8016: the first
// variant serializes with selector byte 0x01, never the reserved 0x00.
func TestCompatibleUnionDefaultSelectorsStartAtOne(t *testing.T) {
	type T struct {
		U CompatibleUnion[struct {
			F1 uint32
			F2 uint64
		}]
	}
	ds := NewDynSsz(nil)

	v := T{}
	v.U.Variant = 1
	v.U.Data = uint32(7)

	enc, err := ds.MarshalSSZ(&v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// offset (4) + selector byte + uint32 payload
	if len(enc) != 9 || enc[4] != 1 {
		t.Fatalf("first variant should serialize with selector 1, got: %x", enc)
	}

	// The reserved selector 0 is not a valid variant.
	v.U.Variant = 0
	if _, err := ds.MarshalSSZ(&v); err == nil {
		t.Fatal("selector 0 should be invalid")
	}

	// Decoding a reserved selector fails.
	bad := append([]byte{4, 0, 0, 0, 0}, 7, 0, 0, 0)
	var dst T
	if err := ds.UnmarshalSSZ(&dst, bad); err == nil {
		t.Fatal("decoding selector 0 should fail")
	}
}

// Classic spec unions use 0-based positional selectors; a None marker declared
// first makes selector 0 the empty option serializing as the single byte 0x00.
func TestUnionNoneWireFormat(t *testing.T) {
	type T struct {
		U Union[struct {
			N  None
			F1 uint32
		}]
	}
	ds := NewDynSsz(nil)

	v := T{} // zero value: Variant 0, Data nil = None
	enc, err := ds.MarshalSSZ(&v)
	if err != nil {
		t.Fatalf("marshal None: %v", err)
	}
	// offset (4) + bare selector byte 0
	if !bytes.Equal(enc, []byte{4, 0, 0, 0, 0}) {
		t.Fatalf("None must serialize as the bare selector byte, got: %x", enc)
	}

	var back T
	if err = ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Fatalf("unmarshal None: %v", err)
	}
	if back.U.Variant != 0 || back.U.Data != nil {
		t.Fatalf("None roundtrip mismatch: %+v", back.U)
	}

	// A concrete variant encodes as selector + serialized value.
	v.U.Variant = 1
	v.U.Data = uint32(7)
	enc, err = ds.MarshalSSZ(&v)
	if err != nil {
		t.Fatalf("marshal variant: %v", err)
	}
	if !bytes.Equal(enc, []byte{4, 0, 0, 0, 1, 7, 0, 0, 0}) {
		t.Fatalf("unexpected variant encoding: %x", enc)
	}
	back = T{}
	if err = ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Fatalf("unmarshal variant: %v", err)
	}
	if back.U.Variant != 1 || back.U.Data != uint32(7) {
		t.Fatalf("variant roundtrip mismatch: %+v", back.U)
	}

	// None with attached data is a type mismatch.
	v.U.Variant = 0
	v.U.Data = uint32(7)
	if _, err := ds.MarshalSSZ(&v); err == nil {
		t.Fatal("None with non-nil data should be rejected")
	}

	// Bytes trailing the bare None selector are rejected.
	var b2 T
	if err := ds.UnmarshalSSZ(&b2, []byte{4, 0, 0, 0, 0, 99}); err == nil {
		t.Fatal("trailing bytes after None selector should be rejected")
	}

	// A selector without a declared variant is rejected.
	if err := ds.UnmarshalSSZ(&b2, []byte{4, 0, 0, 0, 9}); err == nil {
		t.Fatal("out-of-range selector should be rejected")
	}

	// An empty union region has no selector byte.
	if err := ds.UnmarshalSSZ(&b2, []byte{4, 0, 0, 0}); err == nil {
		t.Fatal("empty union region should be rejected")
	}
}

// Without a None marker the first descriptor field is selector 0 and carries
// a value like any other variant.
func TestUnionWithoutNone(t *testing.T) {
	type T struct {
		U Union[struct {
			F1 uint32
			F2 []uint8 `ssz-max:"16"`
		}]
	}
	ds := NewDynSsz(nil)

	v := T{}
	v.U.Variant = 0
	v.U.Data = uint32(7)
	enc, err := ds.MarshalSSZ(&v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(enc, []byte{4, 0, 0, 0, 0, 7, 0, 0, 0}) {
		t.Fatalf("unexpected encoding: %x", enc)
	}
	var back T
	if err = ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.U.Variant != 0 || back.U.Data != uint32(7) {
		t.Fatalf("roundtrip mismatch: %+v", back.U)
	}

	// A dynamic variant roundtrips through the offset machinery.
	v.U.Variant = 1
	v.U.Data = []uint8{1, 2, 3}
	enc, err = ds.MarshalSSZ(&v)
	if err != nil {
		t.Fatalf("marshal dynamic variant: %v", err)
	}
	if !bytes.Equal(enc, []byte{4, 0, 0, 0, 1, 1, 2, 3}) {
		t.Fatalf("unexpected dynamic variant encoding: %x", enc)
	}
	back = T{}
	if err = ds.UnmarshalSSZ(&back, enc); err != nil {
		t.Fatalf("unmarshal dynamic variant: %v", err)
	}
	if back.U.Variant != 1 || !reflect.DeepEqual(back.U.Data, []uint8{1, 2, 3}) {
		t.Fatalf("dynamic variant roundtrip mismatch: %+v", back.U)
	}

	// Selector 0 holds a real variant here, so nil data is invalid.
	v.U.Variant = 0
	v.U.Data = nil
	if _, err := ds.MarshalSSZ(&v); err == nil {
		t.Fatal("nil data for a value-carrying selector should be rejected")
	}
}

// The union root is mix_in_selector(hash_tree_root(value), selector); the None
// option contributes a zero chunk as its data root.
func TestUnionHashTreeRoot(t *testing.T) {
	type T = Union[struct {
		N  None
		F1 uint32
	}]
	ds := NewDynSsz(nil)

	mixin := func(dataRoot [32]byte, selector uint8) [32]byte {
		var buf [64]byte
		copy(buf[:32], dataRoot[:])
		buf[32] = selector
		return sha256.Sum256(buf[:])
	}

	// None: mix_in_selector(Bytes32(), 0)
	root, err := ds.HashTreeRoot(&T{})
	if err != nil {
		t.Fatalf("HTR None: %v", err)
	}
	if want := mixin([32]byte{}, 0); root != want {
		t.Fatalf("None root mismatch: got %x, want %x", root, want)
	}

	// uint32 variant: mix_in_selector(chunk(7), 1)
	root, err = ds.HashTreeRoot(&T{Variant: 1, Data: uint32(7)})
	if err != nil {
		t.Fatalf("HTR variant: %v", err)
	}
	var dataChunk [32]byte
	dataChunk[0] = 7
	if want := mixin(dataChunk, 1); root != want {
		t.Fatalf("variant root mismatch: got %x, want %x", root, want)
	}
}

func TestNewUnion(t *testing.T) {
	type D = struct {
		N  None
		F1 uint32
	}

	u, err := NewUnion[D](1, uint32(7))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.Variant != 1 || u.Data != uint32(7) {
		t.Fatalf("unexpected union value: %+v", u)
	}
}

// Classic union marshal error paths require reaching marshalUnion without a
// prior size pass, which happens via MarshalSSZTo with a preallocated buffer.
func TestUnionStreamMarshalErrorPaths(t *testing.T) {
	type D = struct {
		N  None
		F1 uint32
		F2 []byte `ssz-type:"bitlist" ssz-max:"16"`
	}
	ds := NewDynSsz(nil)

	expectErr := func(t *testing.T, u *Union[D]) {
		t.Helper()
		if _, err := ds.MarshalSSZTo(u, make([]byte, 0, 100)); err == nil {
			t.Fatal("expected error")
		}
	}

	t.Run("noneWithData", func(t *testing.T) {
		expectErr(t, &Union[D]{Variant: 0, Data: uint32(1)})
	})
	t.Run("invalidVariant", func(t *testing.T) {
		expectErr(t, &Union[D]{Variant: 9, Data: uint32(1)})
	})
	t.Run("nilData", func(t *testing.T) {
		expectErr(t, &Union[D]{Variant: 1, Data: nil})
	})
	t.Run("typeMismatch", func(t *testing.T) {
		expectErr(t, &Union[D]{Variant: 1, Data: "not a uint32"})
	})
	t.Run("variantMarshalError", func(t *testing.T) {
		// the bitlist variant is missing its termination bit
		expectErr(t, &Union[D]{Variant: 2, Data: []byte{0xff, 0x00}})
	})

	t.Run("streamRoundtrip", func(t *testing.T) {
		var sb bytes.Buffer
		if err := ds.MarshalSSZWriter(&Union[D]{Variant: 1, Data: uint32(7)}, &sb); err != nil {
			t.Fatalf("stream marshal: %v", err)
		}
		if !bytes.Equal(sb.Bytes(), []byte{1, 7, 0, 0, 0}) {
			t.Fatalf("unexpected stream encoding: %x", sb.Bytes())
		}
	})
}

// Classic union hash tree root error paths.
func TestUnionHTRErrorPaths(t *testing.T) {
	type D = struct {
		N  None
		F1 uint32
		F2 []byte `ssz-type:"bitlist" ssz-max:"16"`
	}
	ds := NewDynSsz(nil)

	expectErr := func(t *testing.T, u *Union[D]) {
		t.Helper()
		if _, err := ds.HashTreeRoot(u); err == nil {
			t.Fatal("expected error")
		}
	}

	t.Run("noneWithData", func(t *testing.T) {
		expectErr(t, &Union[D]{Variant: 0, Data: uint32(1)})
	})
	t.Run("invalidVariant", func(t *testing.T) {
		expectErr(t, &Union[D]{Variant: 9, Data: uint32(1)})
	})
	t.Run("nilData", func(t *testing.T) {
		expectErr(t, &Union[D]{Variant: 1, Data: nil})
	})
	t.Run("typeMismatch", func(t *testing.T) {
		expectErr(t, &Union[D]{Variant: 1, Data: "not a uint32"})
	})
	t.Run("variantHTRError", func(t *testing.T) {
		// the bitlist variant is missing its termination bit
		expectErr(t, &Union[D]{Variant: 2, Data: []byte{0xff, 0x00}})
	})
}

// Classic union size path errors: a buffer marshal of a nested union sizes the
// value first, so these surface from the sizing pass.
func TestUnionSizeErrorPaths(t *testing.T) {
	ds := NewDynSsz(nil)

	t.Run("invalidVariant", func(t *testing.T) {
		type T struct {
			U Union[struct {
				F1 uint32
			}]
		}
		v := T{}
		v.U.Variant = 9
		v.U.Data = uint32(1)
		if _, err := ds.MarshalSSZ(&v); err == nil {
			t.Fatal("expected error for invalid variant")
		}
	})

	t.Run("typeMismatch", func(t *testing.T) {
		type T struct {
			U Union[struct {
				F1 uint32
			}]
		}
		v := T{}
		v.U.Variant = 0
		v.U.Data = "not a uint32"
		if _, err := ds.MarshalSSZ(&v); err == nil {
			t.Fatal("expected error for wrong-typed data")
		}
	})

	t.Run("variantSizeError", func(t *testing.T) {
		// the selected variant is a zero-valued CompatibleUnion, which cannot
		// be sized (selector 0 is unassigned there)
		type T struct {
			U Union[struct {
				F1 CompatibleUnion[struct {
					A uint32
				}]
			}]
		}
		v := T{}
		v.U.Variant = 0
		v.U.Data = CompatibleUnion[struct {
			A uint32
		}]{}
		if _, err := ds.MarshalSSZ(&v); err == nil {
			t.Fatal("expected error from variant sizing")
		}
	})
}

// Classic union unmarshal error paths beyond the wire-format golden tests.
func TestUnionUnmarshalErrorPaths(t *testing.T) {
	ds := NewDynSsz(nil)

	type T struct {
		U Union[struct {
			N  None
			F1 uint32
		}]
	}

	t.Run("selectorReadError", func(t *testing.T) {
		// The declared size promises a union region, but the stream ends before
		// the selector byte can be read.
		var dst T
		if err := ds.UnmarshalSSZReader(&dst, bytes.NewReader([]byte{4, 0, 0, 0}), 5); err == nil {
			t.Fatal("expected error for truncated selector stream")
		}
	})

	t.Run("variantDataError", func(t *testing.T) {
		// selector 1 expects 4 bytes of uint32 data, only 2 are present
		var dst T
		if err := ds.UnmarshalSSZ(&dst, []byte{4, 0, 0, 0, 1, 7, 0}); err == nil {
			t.Fatal("expected error for truncated variant data")
		}
	})
}

// Classic unions support view descriptors: the schema union defines the SSZ
// layout while the runtime union holds the data, matched by variant name.
func TestUnionViewDescriptor(t *testing.T) {
	ds := NewDynSsz(nil)
	tc := ds.GetTypeCache()

	type runtimeDesc = struct {
		N  None
		F1 uint32
		F2 uint64
	}
	type viewDesc = struct {
		N  None
		F1 uint32
	}

	t.Run("viewSubset", func(t *testing.T) {
		desc, err := tc.GetTypeDescriptorWithSchema(
			reflect.TypeOf(Union[runtimeDesc]{}),
			reflect.TypeOf(Union[viewDesc]{}),
			nil, nil, nil,
		)
		if err != nil {
			t.Fatalf("view descriptor build failed: %v", err)
		}
		if len(desc.UnionVariants) != 1 {
			t.Fatalf("expected 1 variant in view, got %d", len(desc.UnionVariants))
		}
	})

	t.Run("missingRuntimeVariant", func(t *testing.T) {
		type otherDesc = struct {
			N  None
			F9 uint16
		}
		_, err := tc.GetTypeDescriptorWithSchema(
			reflect.TypeOf(Union[runtimeDesc]{}),
			reflect.TypeOf(Union[otherDesc]{}),
			nil, nil, nil,
		)
		if err == nil || !strings.Contains(err.Error(), "missing variant") {
			t.Fatalf("expected missing-variant error, got: %v", err)
		}
	})

	t.Run("nonGenericRuntime", func(t *testing.T) {
		_, err := tc.GetTypeDescriptorWithSchema(
			reflect.TypeOf(struct{ Variant uint8 }{}),
			reflect.TypeOf(Union[viewDesc]{}),
			nil, nil, nil,
		)
		if err == nil {
			t.Fatal("expected error for non-generic runtime type")
		}
	})

	t.Run("nonStructRuntimeDescriptor", func(t *testing.T) {
		_, err := tc.GetTypeDescriptorWithSchema(
			reflect.TypeOf(Union[uint64]{}),
			reflect.TypeOf(Union[viewDesc]{}),
			nil, nil, nil,
		)
		if err == nil {
			t.Fatal("expected error for non-struct runtime descriptor")
		}
	})
}

// The None marker is only legal as the first descriptor field, needs at least
// one further variant, and positional selectors leave no room for ssz-index.
func TestUnionDescriptorValidation(t *testing.T) {
	ds := NewDynSsz(nil)

	t.Run("noneNotFirst", func(t *testing.T) {
		type T struct {
			U Union[struct {
				F1 uint32
				N  None
			}]
		}
		if _, err := ds.MarshalSSZ(&T{}); err == nil || !strings.Contains(err.Error(), "None") {
			t.Errorf("expected None placement error, got: %v", err)
		}
	})

	t.Run("onlyNone", func(t *testing.T) {
		type T struct {
			U Union[struct {
				N None
			}]
		}
		if _, err := ds.MarshalSSZ(&T{}); err == nil || !strings.Contains(err.Error(), "None") {
			t.Errorf("expected lone-None error, got: %v", err)
		}
	})

	t.Run("sszIndexTag", func(t *testing.T) {
		type T struct {
			U Union[struct {
				F1 uint32 `ssz-index:"1"`
			}]
		}
		v := T{}
		v.U.Variant = 1
		v.U.Data = uint32(1)
		if _, err := ds.MarshalSSZ(&v); err == nil || !strings.Contains(err.Error(), "ssz-index") {
			t.Errorf("expected ssz-index rejection, got: %v", err)
		}
	})

	t.Run("emptyDescriptor", func(t *testing.T) {
		type T struct {
			U Union[struct{}]
		}
		if _, err := ds.MarshalSSZ(&T{}); err == nil {
			t.Error("expected error for descriptor without fields")
		}
	})

	t.Run("unsupportedVariantType", func(t *testing.T) {
		type T struct {
			U Union[struct {
				F1 chan int
			}]
		}
		if _, err := ds.MarshalSSZ(&T{}); err == nil {
			t.Error("expected error for unsupported variant type")
		}
	})
}

// Explicit ssz-index selectors outside 1..127 are rejected at descriptor build.
func TestCompatibleUnionSelectorRangeEnforced(t *testing.T) {
	ds := NewDynSsz(nil)

	t.Run("zero", func(t *testing.T) {
		type T struct {
			U CompatibleUnion[struct {
				F1 uint32 `ssz-index:"0"`
			}]
		}
		v := T{}
		v.U.Variant = 0
		v.U.Data = uint32(1)
		if _, err := ds.MarshalSSZ(&v); err == nil {
			t.Fatal("ssz-index 0 should be rejected")
		}
	})

	t.Run("above127", func(t *testing.T) {
		type T struct {
			U CompatibleUnion[struct {
				F1 uint32 `ssz-index:"128"`
			}]
		}
		v := T{}
		v.U.Variant = 128
		v.U.Data = uint32(1)
		if _, err := ds.MarshalSSZ(&v); err == nil {
			t.Fatal("ssz-index 128 should be rejected")
		}
	})
}
