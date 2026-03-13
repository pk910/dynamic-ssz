// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// withLowMaxInt temporarily sets platformMaxInt to 100 for testing
// overflow checks on 64-bit systems. It restores the original value when done.
func withLowMaxInt(t *testing.T, fn func()) {
	t.Helper()
	old := platformMaxInt
	platformMaxInt = 100
	t.Cleanup(func() { platformMaxInt = old })
	fn()
}

func newCtx() *ReflectionCtx {
	return NewReflectionCtx(nil, nil, false, true)
}

// stubFastsszUnmarshaler implements sszutils.FastsszUnmarshaler for testing.
type stubFastsszUnmarshaler struct{}

func (s *stubFastsszUnmarshaler) UnmarshalSSZ(buf []byte) error           { return nil }
func (s *stubFastsszUnmarshaler) MarshalSSZTo(dst []byte) ([]byte, error) { return dst, nil }
func (s *stubFastsszUnmarshaler) SizeSSZ() int                            { return 0 }

// stubDynamicUnmarshaler implements sszutils.DynamicUnmarshaler for testing.
type stubDynamicUnmarshaler struct{}

func (s *stubDynamicUnmarshaler) UnmarshalSSZDyn(_ sszutils.DynamicSpecs, buf []byte) error {
	return nil
}

// --- marshalVector overflow tests ---

func TestMarshalVectorLenOverflow(t *testing.T) {
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType:  ssztypes.SszVectorType,
		Kind:     reflect.Slice,
		Len:      200,
		ElemDesc: &ssztypes.TypeDescriptor{Size: 1, Kind: reflect.Uint8},
	}
	val := reflect.ValueOf(make([]byte, 0))
	enc := sszutils.NewBufferEncoder(nil)

	withLowMaxInt(t, func() {
		err := ctx.marshalType(td, val, enc, 0)
		if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
			t.Fatalf("expected overflow error for Len, got: %v", err)
		}
	})
}

func TestMarshalVectorElemSizeOverflow(t *testing.T) {
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType: ssztypes.SszVectorType,
		Kind:    reflect.Slice,
		Len:     2,
		ElemDesc: &ssztypes.TypeDescriptor{
			Size:    200,
			Kind:    reflect.Struct,
			SszType: ssztypes.SszContainerType,
		},
	}
	val := reflect.ValueOf(make([]byte, 2))
	enc := sszutils.NewBufferEncoder(nil)

	withLowMaxInt(t, func() {
		err := ctx.marshalType(td, val, enc, 0)
		if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
			t.Fatalf("expected overflow error for ElemDesc.Size, got: %v", err)
		}
	})
}

// --- marshalDynamicVector overflow test ---

func TestMarshalDynamicVectorLenOverflow(t *testing.T) {
	ctx := newCtx()
	elemDesc := &ssztypes.TypeDescriptor{
		Size:         0,
		Kind:         reflect.Slice,
		SszType:      ssztypes.SszListType,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
	}
	td := &ssztypes.TypeDescriptor{
		SszType:  ssztypes.SszVectorType,
		Kind:     reflect.Slice,
		Len:      200,
		ElemDesc: elemDesc,
	}
	val := reflect.ValueOf(make([][]byte, 0))
	enc := sszutils.NewBufferEncoder(nil)

	withLowMaxInt(t, func() {
		err := ctx.marshalType(td, val, enc, 0)
		if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
			t.Fatalf("expected overflow error, got: %v", err)
		}
	})
}

// --- unmarshalType fastssz path overflow test ---

func TestUnmarshalTypeFastsszSizeOverflow(t *testing.T) {
	ctx := NewReflectionCtx(nil, nil, false, false) // noFastSsz=false to enter fastssz path
	td := &ssztypes.TypeDescriptor{
		SszType:        ssztypes.SszCustomType,
		Kind:           reflect.Struct,
		Size:           200,
		SszCompatFlags: ssztypes.SszCompatFlagFastSSZMarshaler,
		Type:           reflect.TypeOf(stubFastsszUnmarshaler{}),
	}
	val := reflect.New(td.Type).Elem()
	dec := sszutils.NewBufferDecoder(make([]byte, 200))

	withLowMaxInt(t, func() {
		err := ctx.unmarshalType(td, val, dec, 0)
		if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
			t.Fatalf("expected overflow error for fastssz Size, got: %v", err)
		}
	})
}

// --- unmarshalType dynamic unmarshal path overflow test ---

func TestUnmarshalTypeDynamicSizeOverflow(t *testing.T) {
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType:        ssztypes.SszCustomType,
		Kind:           reflect.Struct,
		Size:           200,
		SszCompatFlags: ssztypes.SszCompatFlagDynamicUnmarshaler,
		Type:           reflect.TypeOf(stubDynamicUnmarshaler{}),
	}
	val := reflect.New(td.Type).Elem()
	dec := sszutils.NewBufferDecoder(make([]byte, 200))

	withLowMaxInt(t, func() {
		err := ctx.unmarshalType(td, val, dec, 0)
		if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
			t.Fatalf("expected overflow error for dynamic unmarshal Size, got: %v", err)
		}
	})
}

// --- unmarshalVector overflow test ---

func TestUnmarshalVectorLenOverflow(t *testing.T) {
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType: ssztypes.SszVectorType,
		Kind:    reflect.Slice,
		Len:     200,
		Type:    reflect.TypeOf([]byte{}),
		ElemDesc: &ssztypes.TypeDescriptor{
			Size: 1, Kind: reflect.Uint8,
			Type: reflect.TypeOf(uint8(0)),
		},
		GoTypeFlags: ssztypes.GoTypeFlagIsByteArray,
	}
	dec := sszutils.NewBufferDecoder(make([]byte, 200))

	val := reflect.New(td.Type).Elem()

	withLowMaxInt(t, func() {
		err := ctx.unmarshalType(td, val, dec, 0)
		if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
			t.Fatalf("expected overflow error for vector Len, got: %v", err)
		}
	})
}

// --- unmarshalDynamicVector overflow test ---

func TestUnmarshalDynamicVectorLenOverflow(t *testing.T) {
	ctx := newCtx()
	elemDesc := &ssztypes.TypeDescriptor{
		Size:         0,
		Kind:         reflect.Slice,
		SszType:      ssztypes.SszListType,
		SszTypeFlags: ssztypes.SszTypeFlagIsDynamic,
		Type:         reflect.TypeOf([]byte{}),
	}
	td := &ssztypes.TypeDescriptor{
		SszType:  ssztypes.SszVectorType,
		Kind:     reflect.Slice,
		Len:      200,
		Type:     reflect.TypeOf([][]byte{}),
		ElemDesc: elemDesc,
	}
	dec := sszutils.NewBufferDecoder(make([]byte, 1000))

	val := reflect.New(td.Type).Elem()

	withLowMaxInt(t, func() {
		err := ctx.unmarshalType(td, val, dec, 0)
		if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
			t.Fatalf("expected overflow error for dynamic vector Len, got: %v", err)
		}
	})
}

// --- unmarshalFixedElements overflow test ---

func TestUnmarshalFixedElementsSizeOverflow(t *testing.T) {
	ctx := newCtx()
	fieldType := &ssztypes.TypeDescriptor{
		Size:    200,
		Kind:    reflect.Uint32,
		SszType: ssztypes.SszUint32Type,
		Type:    reflect.TypeOf(uint32(0)),
	}
	dec := sszutils.NewBufferDecoder(make([]byte, 1000))
	newValue := reflect.MakeSlice(reflect.TypeOf([]uint32{}), 5, 5)

	withLowMaxInt(t, func() {
		err := ctx.unmarshalFixedElements(fieldType, newValue, 5, dec, 0, "test")
		if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
			t.Fatalf("expected overflow error for field Size, got: %v", err)
		}
	})
}

// --- unmarshalList overflow test ---

func TestUnmarshalListSizeOverflow(t *testing.T) {
	ctx := newCtx()
	elemDesc := &ssztypes.TypeDescriptor{
		Size:    200,
		Kind:    reflect.Struct,
		SszType: ssztypes.SszContainerType,
		Type:    reflect.TypeOf(struct{}{}),
	}
	td := &ssztypes.TypeDescriptor{
		SszType:  ssztypes.SszListType,
		Kind:     reflect.Slice,
		Type:     reflect.TypeOf([]struct{}{}),
		ElemDesc: elemDesc,
	}
	dec := sszutils.NewBufferDecoder(make([]byte, 400))

	val := reflect.New(td.Type).Elem()

	withLowMaxInt(t, func() {
		err := ctx.unmarshalType(td, val, dec, 0)
		if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
			t.Fatalf("expected overflow error for list field Size, got: %v", err)
		}
	})
}

// --- buildRootFromVector overflow test ---

func TestBuildRootFromVectorLenOverflow(t *testing.T) {
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType: ssztypes.SszVectorType,
		Kind:    reflect.Slice,
		Len:     200,
		Type:    reflect.TypeOf([]byte{}),
		ElemDesc: &ssztypes.TypeDescriptor{
			Size: 1, Kind: reflect.Uint8,
			Type: reflect.TypeOf(uint8(0)),
		},
		GoTypeFlags: ssztypes.GoTypeFlagIsByteArray,
	}
	val := reflect.ValueOf(make([]byte, 200))
	hh := hasher.NewHasher()

	withLowMaxInt(t, func() {
		err := ctx.buildRootFromType(td, val, hh, false, 0)
		if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
			t.Fatalf("expected overflow error for vector Len, got: %v", err)
		}
	})
}
