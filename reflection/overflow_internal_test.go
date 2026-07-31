// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package reflection

import (
	"bytes"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/ssztypes"
	"github.com/pk910/dynamic-ssz/sszutils"
)

const overflowLen = math.MaxInt32 + 1 // triggers > math.MaxInt checks on 32-bit platforms

func newCtx() *ReflectionCtx {
	return NewReflectionCtx(nil, nil, false, true, false, 0)
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

// skipUnless32Bit skips the test on platforms where int is wider than 32 bits,
// since uint32 values cannot exceed math.MaxInt on such platforms.
func skipUnless32Bit(t *testing.T) {
	t.Helper()
	if math.MaxInt > math.MaxInt32 {
		t.Skip("overflow checks are only active on 32-bit platforms")
	}
}

// --- marshalVector overflow tests ---

func TestMarshalVectorLenOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType:  ssztypes.SszVectorType,
		Kind:     reflect.Slice,
		Len:      overflowLen,
		ElemDesc: &ssztypes.TypeDescriptor{Size: 1, Kind: reflect.Uint8},
	}
	val := reflect.ValueOf(make([]byte, 0))
	enc := sszutils.NewBufferEncoder(nil)

	err := ctx.marshalType(td, val, enc, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error for Len, got: %v", err)
	}
}

func TestMarshalVectorElemSizeOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType: ssztypes.SszVectorType,
		Kind:    reflect.Slice,
		Len:     2,
		ElemDesc: &ssztypes.TypeDescriptor{
			Size:    overflowLen,
			Kind:    reflect.Struct,
			SszType: ssztypes.SszContainerType,
		},
	}
	val := reflect.ValueOf(make([]byte, 2))
	enc := sszutils.NewBufferEncoder(nil)

	err := ctx.marshalType(td, val, enc, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error for ElemDesc.Size, got: %v", err)
	}
}

// --- marshalDynamicVector overflow test ---

func TestMarshalDynamicVectorLenOverflow(t *testing.T) {
	skipUnless32Bit(t)
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
		Len:      overflowLen,
		ElemDesc: elemDesc,
	}
	val := reflect.ValueOf(make([][]byte, 0))
	enc := sszutils.NewBufferEncoder(nil)

	err := ctx.marshalType(td, val, enc, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error, got: %v", err)
	}
}

// --- unmarshalType fastssz path overflow test ---

func TestUnmarshalTypeFastsszSizeOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ctx := NewReflectionCtx(nil, nil, false, false, false, 0) // noFastSsz=false to enter fastssz path
	td := &ssztypes.TypeDescriptor{
		SszType:        ssztypes.SszCustomType,
		Kind:           reflect.Struct,
		Size:           overflowLen,
		SszCompatFlags: ssztypes.SszCompatFlagFastSSZMarshaler,
		Type:           reflect.TypeOf(stubFastsszUnmarshaler{}),
	}
	val := reflect.New(td.Type).Elem()
	dec := sszutils.NewBufferDecoder(make([]byte, 200))

	err := ctx.unmarshalType(td, val, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error for fastssz Size, got: %v", err)
	}
}

// --- unmarshalType dynamic unmarshal path overflow test ---

func TestUnmarshalTypeDynamicSizeOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType:        ssztypes.SszCustomType,
		Kind:           reflect.Struct,
		Size:           overflowLen,
		SszCompatFlags: ssztypes.SszCompatFlagDynamicUnmarshaler,
		Type:           reflect.TypeOf(stubDynamicUnmarshaler{}),
	}
	val := reflect.New(td.Type).Elem()
	dec := sszutils.NewBufferDecoder(make([]byte, 200))

	err := ctx.unmarshalType(td, val, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error for dynamic unmarshal Size, got: %v", err)
	}
}

// --- unmarshalVector overflow test ---

func TestUnmarshalVectorLenOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType: ssztypes.SszVectorType,
		Kind:    reflect.Slice,
		Len:     overflowLen,
		Type:    reflect.TypeOf([]byte{}),
		ElemDesc: &ssztypes.TypeDescriptor{
			Size: 1, Kind: reflect.Uint8,
			Type: reflect.TypeOf(uint8(0)),
		},
		GoTypeFlags: ssztypes.GoTypeFlagIsByteArray,
	}
	dec := sszutils.NewBufferDecoder(make([]byte, 200))
	val := reflect.New(td.Type).Elem()

	err := ctx.unmarshalType(td, val, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error for vector Len, got: %v", err)
	}
}

// --- unmarshalDynamicVector overflow test ---

func TestUnmarshalDynamicVectorLenOverflow(t *testing.T) {
	skipUnless32Bit(t)
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
		Len:      overflowLen,
		Type:     reflect.TypeOf([][]byte{}),
		ElemDesc: elemDesc,
	}
	dec := sszutils.NewBufferDecoder(make([]byte, 1000))
	val := reflect.New(td.Type).Elem()

	err := ctx.unmarshalType(td, val, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error for dynamic vector Len, got: %v", err)
	}
}

// --- unmarshalFixedElements overflow test ---

func TestUnmarshalFixedElementsSizeOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ctx := newCtx()
	fieldType := &ssztypes.TypeDescriptor{
		Size:    overflowLen,
		Kind:    reflect.Uint32,
		SszType: ssztypes.SszUint32Type,
		Type:    reflect.TypeOf(uint32(0)),
	}
	dec := sszutils.NewBufferDecoder(make([]byte, 1000))
	newValue := reflect.MakeSlice(reflect.TypeOf([]uint32{}), 5, 5)

	err := ctx.unmarshalFixedElements(fieldType, newValue, 5, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error for field Size, got: %v", err)
	}
}

// --- unmarshalList overflow test ---

func TestUnmarshalListSizeOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ctx := newCtx()
	elemDesc := &ssztypes.TypeDescriptor{
		Size:    overflowLen,
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

	err := ctx.unmarshalType(td, val, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error for list field Size, got: %v", err)
	}
}

// --- buildRootFromVector overflow test ---

func TestBuildRootFromVectorLenOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType: ssztypes.SszVectorType,
		Kind:    reflect.Slice,
		Len:     overflowLen,
		Type:    reflect.TypeOf([]byte{}),
		ElemDesc: &ssztypes.TypeDescriptor{
			Size: 1, Kind: reflect.Uint8,
			Type: reflect.TypeOf(uint8(0)),
		},
		GoTypeFlags: ssztypes.GoTypeFlagIsByteArray,
	}
	val := reflect.ValueOf(make([]byte, 200))
	hh := hasher.NewHasher()

	err := ctx.buildRootFromType(td, val, hh, false, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error for vector Len, got: %v", err)
	}
}

// --- Test that >MaxInt32 sized vectors work on 64-bit using streaming ---

func TestMarshalLargeVectorStreaming(t *testing.T) {
	if math.MaxInt == math.MaxInt32 {
		t.Skip("requires 64-bit platform")
	}

	ctx := newCtx()
	vectorLen := uint32(math.MaxInt32 + 1) // 2GB vector
	td := &ssztypes.TypeDescriptor{
		SszType:     ssztypes.SszVectorType,
		Kind:        reflect.Slice,
		Len:         vectorLen,
		Type:        reflect.TypeOf([]byte{}),
		GoTypeFlags: ssztypes.GoTypeFlagIsByteArray,
		ElemDesc:    &ssztypes.TypeDescriptor{Size: 1, Kind: reflect.Uint8},
	}

	// Use a small source slice — marshalVector will zero-pad the rest.
	// This avoids allocating 2GB of memory.
	src := make([]byte, 16)
	for i := range src {
		src[i] = byte(i + 1)
	}
	val := reflect.ValueOf(src)

	// Use StreamEncoder writing to io.Discard so we don't buffer output.
	enc := sszutils.NewStreamEncoder(io.Discard, 4096)

	err := ctx.marshalType(td, val, enc, 0)
	if err != nil {
		t.Fatalf("unexpected error marshaling large vector: %v", err)
	}

	enc.Flush()
	if werr := enc.GetWriteError(); werr != nil {
		t.Fatalf("unexpected write error: %v", werr)
	}

	expectedPos := int(vectorLen)
	if enc.GetPosition() != expectedPos {
		t.Fatalf("expected position %d, got %d", expectedPos, enc.GetPosition())
	}
}

// --- unknown-length decode overflow tests ---
//
// These guards live on the read-to-EOF path, which derives allocations from
// ssz-max rather than from a region length. Like the others here they are only
// active on 32-bit platforms, where a uint64 limit can exceed math.MaxInt.

func TestUnmarshalListUntilEOFLimitOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ctx := newCtx()
	elemDesc := &ssztypes.TypeDescriptor{
		Size:    4,
		Kind:    reflect.Uint32,
		SszType: ssztypes.SszUint32Type,
		Type:    reflect.TypeOf(uint32(0)),
	}
	td := &ssztypes.TypeDescriptor{
		SszType:      ssztypes.SszListType,
		SszTypeFlags: ssztypes.SszTypeFlagHasLimit,
		Limit:        math.MaxUint64,
		Kind:         reflect.Slice,
		Type:         reflect.TypeOf([]uint32{}),
		ElemDesc:     elemDesc,
	}
	// An open region selects the read-to-EOF path, where the limit is the cap.
	dec := sszutils.NewUnknownStreamDecoder(strings.NewReader("12345678"), 8, 0)
	dec.PushOpenLimit()
	val := reflect.New(td.Type).Elem()

	err := ctx.unmarshalType(td, val, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error for list limit, got: %v", err)
	}
}

func TestUnmarshalBitlistLimitOverflow(t *testing.T) {
	skipUnless32Bit(t)
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType:      ssztypes.SszBitlistType,
		SszTypeFlags: ssztypes.SszTypeFlagHasLimit,
		Limit:        math.MaxUint64,
		Kind:         reflect.Slice,
		Type:         reflect.TypeOf([]byte{}),
	}
	dec := sszutils.NewBufferDecoder([]byte{0x01})
	val := reflect.New(td.Type).Elem()

	err := ctx.unmarshalType(td, val, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error for bitlist limit, got: %v", err)
	}
}

// delegationBuffer sizes a delegate's buffer from the type's own size.
func TestDelegationBufferSizeOverflow(t *testing.T) {
	skipUnless32Bit(t)
	td := &ssztypes.TypeDescriptor{
		Size:    overflowLen,
		SszType: ssztypes.SszContainerType,
		Kind:    reflect.Struct,
		Type:    reflect.TypeOf(struct{}{}),
	}
	dec := sszutils.NewBufferDecoder(make([]byte, 8))

	_, err := delegationBuffer(td, dec)
	if err == nil || !strings.Contains(err.Error(), "exceeds platform int max") {
		t.Fatalf("expected overflow error for delegate size, got: %v", err)
	}
}

// bigIntLimitBytes reports "no cap" rather than overflowing when the declared
// ssz-max cannot be represented as an int.
func TestBigIntLimitBytesOverflow(t *testing.T) {
	skipUnless32Bit(t)
	td := &ssztypes.TypeDescriptor{
		SszType:      ssztypes.SszBigIntType,
		SszTypeFlags: ssztypes.SszTypeFlagHasLimit,
		Limit:        math.MaxUint64,
	}
	if _, ok := bigIntLimitBytes(td); ok {
		t.Fatal("expected no representable byte cap for an out-of-range limit")
	}
}

// --- open-region streaming decode error branches ---
//
// These reach the read-to-EOF and offset-region error paths that only the
// unknown-length stream decoder exercises. Unlike the overflow guards above they
// are not platform-gated.

func uint32ElemDesc() *ssztypes.TypeDescriptor {
	return &ssztypes.TypeDescriptor{
		Size:    4,
		Kind:    reflect.Uint32,
		SszType: ssztypes.SszUint32Type,
		Type:    reflect.TypeOf(uint32(0)),
	}
}

// A bounded but not-yet-buffered region reports LengthKnown()==false, so the
// read-to-EOF list path runs, yet RegionOpen()==false keeps the alignment and
// ssz-max checks active. A 5-byte region is not a multiple of the 4-byte element.
func TestUnmarshalListUntilEOFNotAligned(t *testing.T) {
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType:  ssztypes.SszListType,
		Kind:     reflect.Slice,
		Type:     reflect.TypeOf([]uint32{}),
		ElemDesc: uint32ElemDesc(),
	}
	dec := sszutils.NewUnknownStreamDecoder(strings.NewReader("12345"), 8, 0)
	dec.PushLimit(5)
	val := reflect.New(td.Type).Elem()
	err := ctx.unmarshalType(td, val, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "not a multiple of element size") {
		t.Fatalf("expected list-not-aligned error, got: %v", err)
	}
}

// An 8-byte bounded region declares two 4-byte elements, but the list max is 1.
func TestUnmarshalListUntilEOFTooLong(t *testing.T) {
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType:      ssztypes.SszListType,
		SszTypeFlags: ssztypes.SszTypeFlagHasLimit,
		Limit:        1,
		Kind:         reflect.Slice,
		Type:         reflect.TypeOf([]uint32{}),
		ElemDesc:     uint32ElemDesc(),
	}
	dec := sszutils.NewUnknownStreamDecoder(strings.NewReader("12345678"), 8, 0)
	dec.PushLimit(8)
	val := reflect.New(td.Type).Elem()
	err := ctx.unmarshalType(td, val, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("expected list-too-long error, got: %v", err)
	}
}

// An element that claims an 8-byte size but decodes as a 4-byte uint32 under-
// consumes its slot and trips the static-element consumption check.
func TestUnmarshalListUntilEOFStaticElementNotConsumed(t *testing.T) {
	ctx := newCtx()
	elemDesc := &ssztypes.TypeDescriptor{
		Size:    8,
		Kind:    reflect.Uint32,
		SszType: ssztypes.SszUint32Type,
		Type:    reflect.TypeOf(uint32(0)),
	}
	td := &ssztypes.TypeDescriptor{
		SszType:  ssztypes.SszListType,
		Kind:     reflect.Slice,
		Type:     reflect.TypeOf([]uint32{}),
		ElemDesc: elemDesc,
	}
	dec := sszutils.NewUnknownStreamDecoder(strings.NewReader("12345678"), 8, 0)
	dec.PushOpenLimit()
	val := reflect.New(td.Type).Elem()
	err := ctx.unmarshalType(td, val, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "element consumed to position") {
		t.Fatalf("expected static-element-not-consumed error, got: %v", err)
	}
}

// The trailing element of an open-region dynamic vector runs to EOF, so surplus
// bytes after the element surface as trailing data when its region is finished.
func TestUnmarshalDynamicVectorOpenTrailing(t *testing.T) {
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType:  ssztypes.SszVectorType,
		Kind:     reflect.Slice,
		Len:      1,
		Type:     reflect.TypeOf([]uint32{}),
		ElemDesc: uint32ElemDesc(),
	}
	// one offset (=4) + a 4-byte element + 2 trailing bytes.
	data := []byte{4, 0, 0, 0, 1, 0, 0, 0, 0xFF, 0xFF}
	dec := sszutils.NewUnknownStreamDecoder(bytes.NewReader(data), 64, 0)
	dec.PushOpenLimit()
	val := reflect.New(td.Type).Elem()
	err := ctx.unmarshalDynamicVector(td, val, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("expected trailing-data error, got: %v", err)
	}
}

// Same trailing-data path for a dynamic list: firstOffset=4 selects one element,
// followed by a 4-byte element and 2 trailing bytes.
func TestUnmarshalDynamicListOpenTrailing(t *testing.T) {
	ctx := newCtx()
	td := &ssztypes.TypeDescriptor{
		SszType:  ssztypes.SszListType,
		Kind:     reflect.Slice,
		Type:     reflect.TypeOf([]uint32{}),
		ElemDesc: uint32ElemDesc(),
	}
	data := []byte{4, 0, 0, 0, 1, 0, 0, 0, 0xFF, 0xFF}
	dec := sszutils.NewUnknownStreamDecoder(bytes.NewReader(data), 64, 0)
	dec.PushOpenLimit()
	val := reflect.New(td.Type).Elem()
	err := ctx.unmarshalDynamicList(td, val, dec, 0)
	if err == nil || !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("expected trailing-data error, got: %v", err)
	}
}
