// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
	"unsafe"
)

// ---- Unmarshal functions ----

// UnmarshallUint64 unmarshals a little endian uint64 from the src input
func UnmarshallUint64(src []byte) uint64 {
	return binary.LittleEndian.Uint64(src)
}

// UnmarshallUint32 unmarshals a little endian uint32 from the src input
func UnmarshallUint32(src []byte) uint32 {
	return binary.LittleEndian.Uint32(src[:4])
}

// UnmarshallUint16 unmarshals a little endian uint16 from the src input
func UnmarshallUint16(src []byte) uint16 {
	return binary.LittleEndian.Uint16(src[:2])
}

// UnmarshallUint8 unmarshals a little endian uint8 from the src input
func UnmarshallUint8(src []byte) uint8 {
	return src[0]
}

// UnmarshalBool unmarshals a boolean from the src input
func UnmarshalBool(src []byte) bool {
	return src[0] == 1
}

// UnmarshalUint64Slice decodes little-endian encoded uint64 values from buf into dst.
// On little-endian architectures (x86, ARM64) this is a single bulk memory copy.
func UnmarshalUint64Slice[T ~uint64](dst []T, buf []byte) {
	if len(dst) == 0 {
		return
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(dst))), len(dst)*8), buf)
}

// DecodeUint64Slice decodes uint64 values from a Decoder directly into dst using bulk memory copy.
// On little-endian architectures (x86, ARM64) this avoids per-element DecodeUint64 overhead.
func DecodeUint64Slice[T ~uint64](dec Decoder, dst []T) error {
	if len(dst) == 0 {
		return nil
	}
	_, err := dec.DecodeBytes(unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(dst))), len(dst)*8))
	return err
}

// ---- offset functions ----

// ReadOffset reads an offset from buf
func ReadOffset(buf []byte) uint64 {
	return uint64(binary.LittleEndian.Uint32(buf))
}

// ---- expansion functions ----

// ExpandSlice ensures the slice has exactly the requested length. It reuses
// the existing backing array when the capacity is sufficient, avoiding a
// heap allocation for repeated unmarshal calls on the same target.
func ExpandSlice[T any](src []T, size int) []T {
	if cap(src) >= size {
		return src[:size]
	}
	return make([]T, size)
}
