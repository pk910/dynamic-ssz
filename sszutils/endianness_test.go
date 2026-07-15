// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unsafe"
)

// endianTestValues covers zero, small, large and asymmetric values so a
// byte-swapped encoding can never accidentally match the reference encoding.
var endianTestValues = []uint64{
	0,
	1,
	0x0102030405060708,
	0xdeadbeefcafebabe,
	0xffffffffffffffff,
	42,
}

// endianTestReference is the expected SSZ (little-endian) encoding of
// endianTestValues, built with the per-element scalar encoder.
func endianTestReference() []byte {
	ref := make([]byte, 0, len(endianTestValues)*8)
	for _, v := range endianTestValues {
		ref = binary.LittleEndian.AppendUint64(ref, v)
	}
	return ref
}

// TestHostLittleEndianDetection cross-checks the binary.NativeEndian based
// hostLittleEndian detection against an independent unsafe pointer probe of
// the host byte order.
func TestHostLittleEndianDetection(t *testing.T) {
	x := uint16(1)
	actualLittleEndian := *(*byte)(unsafe.Pointer(&x)) == 1
	if hostLittleEndian != actualLittleEndian {
		t.Fatalf("hostLittleEndian = %v, but host byte order is little-endian = %v",
			hostLittleEndian, actualLittleEndian)
	}
}

// TestMarshalUint64SliceEncoding verifies the bulk marshal helper produces
// canonical little-endian SSZ bytes regardless of host byte order.
func TestMarshalUint64SliceEncoding(t *testing.T) {
	got := MarshalUint64Slice([]byte{0xaa}, endianTestValues)
	want := append([]byte{0xaa}, endianTestReference()...)
	if !bytes.Equal(got, want) {
		t.Fatalf("MarshalUint64Slice mismatch:\ngot  %x\nwant %x", got, want)
	}

	if out := MarshalUint64Slice([]byte{0xaa}, []uint64{}); !bytes.Equal(out, []byte{0xaa}) {
		t.Fatalf("MarshalUint64Slice with empty slice mutated dst: %x", out)
	}
}

// TestUnmarshalUint64SliceDecoding verifies the bulk unmarshal helper decodes
// canonical little-endian SSZ bytes regardless of host byte order.
func TestUnmarshalUint64SliceDecoding(t *testing.T) {
	dst := make([]uint64, len(endianTestValues))
	UnmarshalUint64Slice(dst, endianTestReference())
	for i, v := range endianTestValues {
		if dst[i] != v {
			t.Fatalf("UnmarshalUint64Slice[%d] = %#x, want %#x", i, dst[i], v)
		}
	}

	// A short buffer must only decode the fully covered elements.
	short := make([]uint64, len(endianTestValues))
	UnmarshalUint64Slice(short, endianTestReference()[:8])
	if short[0] != endianTestValues[0] {
		t.Fatalf("UnmarshalUint64Slice short buf[0] = %#x, want %#x", short[0], endianTestValues[0])
	}
	for i := 1; i < len(short); i++ {
		if short[i] != 0 {
			t.Fatalf("UnmarshalUint64Slice short buf[%d] = %#x, want 0", i, short[i])
		}
	}
}

// TestEncodeUint64SliceEncoding verifies the encoder-based bulk helper against
// both Encoder implementations.
func TestEncodeUint64SliceEncoding(t *testing.T) {
	want := endianTestReference()

	enc := NewBufferEncoder(make([]byte, 0, len(want)))
	EncodeUint64Slice(enc, endianTestValues)
	if got := enc.GetBuffer(); !bytes.Equal(got, want) {
		t.Fatalf("EncodeUint64Slice (buffer) mismatch:\ngot  %x\nwant %x", got, want)
	}

	var buf bytes.Buffer
	senc := NewStreamEncoder(&buf, 0)
	EncodeUint64Slice(senc, endianTestValues)
	senc.Flush()
	if err := senc.GetWriteError(); err != nil {
		t.Fatalf("EncodeUint64Slice (stream) write error: %v", err)
	}
	if got := buf.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("EncodeUint64Slice (stream) mismatch:\ngot  %x\nwant %x", got, want)
	}
}

// TestDecodeUint64SliceDecoding verifies the decoder-based bulk helper against
// both Decoder implementations.
func TestDecodeUint64SliceDecoding(t *testing.T) {
	ref := endianTestReference()

	for _, tc := range []struct {
		name string
		dec  Decoder
	}{
		{"buffer", NewBufferDecoder(ref)},
		{"stream", NewStreamDecoder(bytes.NewReader(ref), len(ref), 0)},
	} {
		dst := make([]uint64, len(endianTestValues))
		if err := DecodeUint64Slice(tc.dec, dst); err != nil {
			t.Fatalf("DecodeUint64Slice (%s) error: %v", tc.name, err)
		}
		for i, v := range endianTestValues {
			if dst[i] != v {
				t.Fatalf("DecodeUint64Slice (%s)[%d] = %#x, want %#x", tc.name, i, dst[i], v)
			}
		}
	}
}

// hashAppendRecorder captures Append/AppendUint64 calls of the HashWalker
// interface. The embedded nil interface satisfies the remaining methods, which
// are never invoked by HashUint64Slice.
type hashAppendRecorder struct {
	HashWalker
	buf []byte
}

func (r *hashAppendRecorder) Append(b []byte) {
	r.buf = append(r.buf, b...)
}

func (r *hashAppendRecorder) AppendUint64(i uint64) {
	r.buf = MarshalUint64(r.buf, i)
}

// TestHashUint64SliceEncoding verifies the hash helper feeds canonical
// little-endian bytes into the hash walker regardless of host byte order.
func TestHashUint64SliceEncoding(t *testing.T) {
	rec := &hashAppendRecorder{}
	HashUint64Slice(rec, endianTestValues)
	if want := endianTestReference(); !bytes.Equal(rec.buf, want) {
		t.Fatalf("HashUint64Slice mismatch:\ngot  %x\nwant %x", rec.buf, want)
	}
}
