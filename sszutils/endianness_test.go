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

// TestHostLittleEndianDetection cross-checks the hostLittleEndian declaration
// selected by the build tags against an independent unsafe pointer probe of
// the host byte order.
func TestHostLittleEndianDetection(t *testing.T) {
	x := uint16(1)
	actualLittleEndian := *(*byte)(unsafe.Pointer(&x)) == 1
	if hostLittleEndian != actualLittleEndian {
		t.Fatalf("hostLittleEndian = %v, but host byte order is little-endian = %v",
			hostLittleEndian, actualLittleEndian)
	}
}

// checkMarshalUint64Slice verifies the bulk marshal helper produces canonical
// little-endian SSZ bytes.
func checkMarshalUint64Slice(t *testing.T) {
	t.Helper()

	got := MarshalUint64Slice([]byte{0xaa}, endianTestValues)
	want := append([]byte{0xaa}, endianTestReference()...)
	if !bytes.Equal(got, want) {
		t.Fatalf("MarshalUint64Slice mismatch:\ngot  %x\nwant %x", got, want)
	}

	if out := MarshalUint64Slice([]byte{0xaa}, []uint64{}); !bytes.Equal(out, []byte{0xaa}) {
		t.Fatalf("MarshalUint64Slice with empty slice mutated dst: %x", out)
	}
}

func TestMarshalUint64SliceEncoding(t *testing.T) {
	checkMarshalUint64Slice(t)
}

// checkUnmarshalUint64Slice verifies the bulk unmarshal helper decodes
// canonical little-endian SSZ bytes, including short-buffer semantics.
func checkUnmarshalUint64Slice(t *testing.T) {
	t.Helper()

	dst := make([]uint64, len(endianTestValues))
	UnmarshalUint64Slice(dst, endianTestReference())
	for i, v := range endianTestValues {
		if dst[i] != v {
			t.Fatalf("UnmarshalUint64Slice[%d] = %#x, want %#x", i, dst[i], v)
		}
	}

	// A short buffer must only decode the fully covered elements. The 12-byte
	// case ends in the middle of the second element, which must stay untouched
	// rather than being partially overwritten.
	const sentinel = 0xfefefefefefefefe
	for _, bufLen := range []int{8, 12} {
		short := make([]uint64, len(endianTestValues))
		for i := range short {
			short[i] = sentinel
		}
		UnmarshalUint64Slice(short, endianTestReference()[:bufLen])
		if short[0] != endianTestValues[0] {
			t.Fatalf("UnmarshalUint64Slice short buf (len %d) [0] = %#x, want %#x",
				bufLen, short[0], endianTestValues[0])
		}
		for i := 1; i < len(short); i++ {
			if short[i] != sentinel {
				t.Fatalf("UnmarshalUint64Slice short buf (len %d) [%d] = %#x, want untouched sentinel",
					bufLen, i, short[i])
			}
		}
	}

	// Empty destinations must be a no-op regardless of the buffer.
	UnmarshalUint64Slice([]uint64{}, endianTestReference())
}

func TestUnmarshalUint64SliceDecoding(t *testing.T) {
	checkUnmarshalUint64Slice(t)
}

// checkEncodeUint64Slice verifies the encoder-based bulk helper against both
// Encoder implementations.
func checkEncodeUint64Slice(t *testing.T) {
	t.Helper()

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

func TestEncodeUint64SliceEncoding(t *testing.T) {
	checkEncodeUint64Slice(t)
}

// checkDecodeUint64Slice verifies the decoder-based bulk helper against both
// Decoder implementations.
func checkDecodeUint64Slice(t *testing.T) {
	t.Helper()

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

	// A truncated stream must surface the decoder error.
	sdec := NewStreamDecoder(bytes.NewReader(ref[:12]), 12, 0)
	if err := DecodeUint64Slice(sdec, make([]uint64, len(endianTestValues))); err == nil {
		t.Fatal("DecodeUint64Slice with truncated stream: expected error, got nil")
	}
}

func TestDecodeUint64SliceDecoding(t *testing.T) {
	checkDecodeUint64Slice(t)
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

// checkHashUint64Slice verifies the hash helper feeds canonical little-endian
// bytes into the hash walker.
func checkHashUint64Slice(t *testing.T) {
	t.Helper()

	rec := &hashAppendRecorder{}
	HashUint64Slice(rec, endianTestValues)
	if want := endianTestReference(); !bytes.Equal(rec.buf, want) {
		t.Fatalf("HashUint64Slice mismatch:\ngot  %x\nwant %x", rec.buf, want)
	}
}

func TestHashUint64SliceEncoding(t *testing.T) {
	checkHashUint64Slice(t)
}
