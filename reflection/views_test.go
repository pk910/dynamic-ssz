// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection_test

import (
	"bytes"
	"reflect"
	"testing"

	. "github.com/pk910/dynamic-ssz"
	"github.com/pk910/dynamic-ssz/reflection"
	"github.com/pk910/dynamic-ssz/ssztypes"
)

// viewMarshalWrapper wraps a view-capable type as a nested field so that the
// reflection layer (not the top-level DynSsz interceptor) exercises the
// tryMarshalView / tryUnmarshalView / getSszValueSize / buildRootFromType
// view code paths.
type viewMarshalWrapper struct {
	Prefix uint32
	Inner  TestContainerWithAllViewInterfaces
}

// viewMarshalWrapperViewType1 is the view descriptor for viewMarshalWrapper
// using TestViewType1 as the schema for the Inner field.
type viewMarshalWrapperViewType1 struct {
	Prefix uint32
	Inner  TestViewType1
}

// viewMarshalWrapperViewType2 is the view descriptor for viewMarshalWrapper
// using TestViewType2 as the schema for the Inner field (triggers error paths).
type viewMarshalWrapperViewType2 struct {
	Prefix uint32
	Inner  TestViewType2
}

// viewMarshalWrapperViewUnknown is the view descriptor for viewMarshalWrapper
// using TestViewTypeUnknown (returns nil from view methods, falls through
// to reflection).
type viewMarshalWrapperViewUnknown struct {
	Prefix uint32
	Inner  TestViewTypeUnknown
}

// TestViewMarshalReflectionEncoder exercises the MarshalSSZEncoderView path
// in reflection/marshal.go tryMarshalView via a seekable (buffer) encoder.
func TestViewMarshalReflectionEncoder(t *testing.T) {
	ds := NewDynSsz(nil)

	wrapper := viewMarshalWrapper{
		Prefix: 42,
		Inner:  TestContainerWithAllViewInterfaces{Field0: 123, Field1: 456},
	}

	// Expected SSZ: uint32(42) + uint64(123) + uint32(456)
	// = 0x2a000000 + 0x7b00000000000000 + 0xc8010000
	expectedSSZ := fromHex("0x2a0000007b00000000000000c8010000")

	testCases := []struct {
		name        string
		view        any
		expectError string
	}{
		{
			name: "encoder_success",
			view: (*viewMarshalWrapperViewType1)(nil),
		},
		{
			name:        "encoder_error",
			view:        (*viewMarshalWrapperViewType2)(nil),
			expectError: "test view encoder error",
		},
		{
			name: "encoder_fallback_to_reflection",
			view: (*viewMarshalWrapperViewUnknown)(nil),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// MarshalSSZ uses a seekable (buffer) encoder which prefers
			// MarshalSSZEncoderView
			data, err := ds.MarshalSSZ(&wrapper, WithViewDescriptor(tc.view))

			if tc.expectError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.expectError)
				}
				if !contains(err.Error(), tc.expectError) {
					t.Fatalf("expected error containing %q, got: %v", tc.expectError, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(data, expectedSSZ) {
				t.Fatalf("expected SSZ %x, got %x", expectedSSZ, data)
			}
		})
	}
}

// TestViewMarshalReflectionMarshaler exercises the MarshalSSZDynView path
// in reflection/marshal.go tryMarshalView via a non-seekable (stream) encoder.
func TestViewMarshalReflectionMarshaler(t *testing.T) {
	ds := NewDynSsz(nil)

	wrapper := viewMarshalWrapper{
		Prefix: 42,
		Inner:  TestContainerWithAllViewInterfaces{Field0: 123, Field1: 456},
	}

	expectedSSZ := fromHex("0x2a0000007b00000000000000c8010000")

	testCases := []struct {
		name        string
		view        any
		expectError string
	}{
		{
			name: "marshaler_success",
			view: (*viewMarshalWrapperViewType1)(nil),
		},
		{
			name:        "marshaler_error",
			view:        (*viewMarshalWrapperViewType2)(nil),
			expectError: "test view marshaler error",
		},
		{
			name: "marshaler_fallback_to_reflection",
			view: (*viewMarshalWrapperViewUnknown)(nil),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// MarshalSSZWriter uses a non-seekable (stream) encoder which
			// prefers MarshalSSZDynView over MarshalSSZEncoderView
			var buf bytes.Buffer
			err := ds.MarshalSSZWriter(&wrapper, &buf, WithViewDescriptor(tc.view))

			if tc.expectError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.expectError)
				}
				if !contains(err.Error(), tc.expectError) {
					t.Fatalf("expected error containing %q, got: %v", tc.expectError, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), expectedSSZ) {
				t.Fatalf("expected SSZ %x, got %x", expectedSSZ, buf.Bytes())
			}
		})
	}
}

// TestViewUnmarshalReflectionUnmarshaler exercises the UnmarshalSSZDynView
// path in reflection/unmarshal.go via a seekable (buffer) decoder.
func TestViewUnmarshalReflectionUnmarshaler(t *testing.T) {
	ds := NewDynSsz(nil)

	// SSZ data: uint32(42) + uint64(0x0807060504030201) + uint32(0x0c0b0a09)
	sszData := fromHex("0x2a0000000102030405060708090a0b0c")

	testCases := []struct {
		name        string
		view        any
		expectInner TestContainerWithAllViewInterfaces
		expectError string
	}{
		{
			name:        "unmarshaler_success",
			view:        (*viewMarshalWrapperViewType1)(nil),
			expectInner: TestContainerWithAllViewInterfaces{Field0: 0x0807060504030201, Field1: 0x0c0b0a09},
		},
		{
			name:        "unmarshaler_error",
			view:        (*viewMarshalWrapperViewType2)(nil),
			expectError: "test view unmarshaler error",
		},
		{
			name:        "unmarshaler_fallback_to_reflection",
			view:        (*viewMarshalWrapperViewUnknown)(nil),
			expectInner: TestContainerWithAllViewInterfaces{Field0: 0x0807060504030201, Field1: 0x0c0b0a09},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var wrapper viewMarshalWrapper
			err := ds.UnmarshalSSZ(&wrapper, sszData, WithViewDescriptor(tc.view))

			if tc.expectError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.expectError)
				}
				if !contains(err.Error(), tc.expectError) {
					t.Fatalf("expected error containing %q, got: %v", tc.expectError, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wrapper.Prefix != 42 {
				t.Fatalf("expected Prefix 42, got %d", wrapper.Prefix)
			}
			if wrapper.Inner != tc.expectInner {
				t.Fatalf("expected Inner %+v, got %+v", tc.expectInner, wrapper.Inner)
			}
		})
	}
}

// TestViewUnmarshalReflectionDecoder exercises the UnmarshalSSZDecoderView
// path in reflection/unmarshal.go via a non-seekable (stream) decoder.
func TestViewUnmarshalReflectionDecoder(t *testing.T) {
	ds := NewDynSsz(nil)

	sszData := fromHex("0x2a0000000102030405060708090a0b0c")

	testCases := []struct {
		name        string
		view        any
		expectInner TestContainerWithAllViewInterfaces
		expectError string
	}{
		{
			name:        "decoder_success",
			view:        (*viewMarshalWrapperViewType1)(nil),
			expectInner: TestContainerWithAllViewInterfaces{Field0: 0x0807060504030201, Field1: 0x0c0b0a09},
		},
		{
			name:        "decoder_error",
			view:        (*viewMarshalWrapperViewType2)(nil),
			expectError: "test view decoder error",
		},
		{
			name:        "decoder_fallback_to_reflection",
			view:        (*viewMarshalWrapperViewUnknown)(nil),
			expectInner: TestContainerWithAllViewInterfaces{Field0: 0x0807060504030201, Field1: 0x0c0b0a09},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var wrapper viewMarshalWrapper
			err := ds.UnmarshalSSZReader(
				&wrapper,
				bytes.NewReader(sszData),
				len(sszData),
				WithViewDescriptor(tc.view),
			)

			if tc.expectError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.expectError)
				}
				if !contains(err.Error(), tc.expectError) {
					t.Fatalf("expected error containing %q, got: %v", tc.expectError, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wrapper.Prefix != 42 {
				t.Fatalf("expected Prefix 42, got %d", wrapper.Prefix)
			}
			if wrapper.Inner != tc.expectInner {
				t.Fatalf("expected Inner %+v, got %+v", tc.expectInner, wrapper.Inner)
			}
		})
	}
}

// TestViewSizerReflection exercises the SizeSSZDynView path in
// reflection/sszsize.go getSszValueSize by calling the reflection layer
// directly, bypassing the DynSsz top-level view sizer interceptor.
func TestViewSizerReflection(t *testing.T) {
	ds := NewDynSsz(nil)

	// Build a type descriptor with view (runtime != schema) that has the
	// DynamicViewSizer flag set on the runtime type.
	desc, err := ds.GetTypeCache().GetTypeDescriptorWithSchema(
		reflect.TypeOf(TestContainerWithAllViewInterfaces{}),
		reflect.TypeOf(TestViewType1{}),
		nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("failed to get type descriptor: %v", err)
	}

	// Verify the descriptor has the expected flags.
	if desc.GoTypeFlags&ssztypes.GoTypeFlagIsView == 0 {
		t.Fatal("expected GoTypeFlagIsView to be set")
	}
	if desc.SszCompatFlags&ssztypes.SszCompatFlagDynamicViewSizer == 0 {
		t.Fatal("expected SszCompatFlagDynamicViewSizer to be set")
	}

	container := TestContainerWithAllViewInterfaces{Field0: 123, Field1: 456}
	ctx := reflection.NewReflectionCtx(ds, nil, false, true)

	size, err := ctx.SizeSSZ(desc, reflect.ValueOf(container))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The view sizer for TestViewType1 returns 12.
	if size != 12 {
		t.Fatalf("expected size 12, got %d", size)
	}
}

// TestViewHashRootReflection exercises the HashTreeRootWithDynView path in
// reflection/treeroot.go buildRootFromType via a nested view field.
func TestViewHashRootReflection(t *testing.T) {
	ds := NewDynSsz(nil)

	wrapper := viewMarshalWrapper{
		Prefix: 42,
		Inner: TestContainerWithAllViewInterfaces{
			Field0: 0x0807060504030201,
			Field1: 0x0c0b0a09,
		},
	}

	testCases := []struct {
		name        string
		view        any
		expectError string
	}{
		{
			name: "hash_root_success",
			view: (*viewMarshalWrapperViewType1)(nil),
		},
		{
			name:        "hash_root_error",
			view:        (*viewMarshalWrapperViewType2)(nil),
			expectError: "test view hash root error",
		},
		{
			name: "hash_root_fallback_to_reflection",
			view: (*viewMarshalWrapperViewUnknown)(nil),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := ds.HashTreeRoot(&wrapper, WithViewDescriptor(tc.view))

			if tc.expectError != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.expectError)
				}
				if !contains(err.Error(), tc.expectError) {
					t.Fatalf("expected error containing %q, got: %v", tc.expectError, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if hash == [32]byte{} {
				t.Fatal("hash should not be zero")
			}
		})
	}
}

// TestViewHashRootReflectionVerbose exercises the verbose logging branch
// inside the view hash root path (treeroot.go lines 73-75).
func TestViewHashRootReflectionVerbose(t *testing.T) {
	ds := NewDynSsz(nil, WithVerbose(), WithLogCb(func(format string, args ...any) {}))

	wrapper := viewMarshalWrapper{
		Prefix: 42,
		Inner: TestContainerWithAllViewInterfaces{
			Field0: 0x0807060504030201,
			Field1: 0x0c0b0a09,
		},
	}

	hash, err := ds.HashTreeRoot(
		&wrapper,
		WithViewDescriptor((*viewMarshalWrapperViewType1)(nil)),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == [32]byte{} {
		t.Fatal("hash should not be zero")
	}
}
