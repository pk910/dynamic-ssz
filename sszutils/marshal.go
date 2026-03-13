// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
	"unsafe"
)

// ---- Marshal functions ----

// MarshalUint64 marshals a little endian uint64 to dst
func MarshalUint64(dst []byte, i uint64) []byte {
	return binary.LittleEndian.AppendUint64(dst, i)
}

// MarshalUint32 marshals a little endian uint32 to dst
func MarshalUint32(dst []byte, i uint32) []byte {
	return binary.LittleEndian.AppendUint32(dst, i)
}

// MarshalUint16 marshals a little endian uint16 to dst
func MarshalUint16(dst []byte, i uint16) []byte {
	return binary.LittleEndian.AppendUint16(dst, i)
}

// MarshalUint8 marshals a little endian uint8 to dst
func MarshalUint8(dst []byte, i uint8) []byte {
	dst = append(dst, i)
	return dst
}

// MarshalBool marshals a boolean to dst
func MarshalBool(dst []byte, b bool) []byte {
	if b {
		dst = append(dst, 1)
	} else {
		dst = append(dst, 0)
	}
	return dst
}

// MarshalUint64Slice appends the little-endian encoding of a uint64 slice to dst.
// On little-endian architectures (x86, ARM64) this is a single bulk memory copy,
// avoiding per-element encoding overhead.
func MarshalUint64Slice[T ~uint64](dst []byte, s []T) []byte {
	if len(s) == 0 {
		return dst
	}
	return append(dst, unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*8)...)
}

// EncodeUint64Slice encodes a uint64 slice to an Encoder using bulk memory copy.
// On little-endian architectures (x86, ARM64) this avoids per-element EncodeUint64 overhead.
func EncodeUint64Slice[T ~uint64](enc Encoder, s []T) {
	if len(s) == 0 {
		return
	}
	enc.EncodeBytes(unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*8))
}

// MarshalOffset marshals an offset to dst
func MarshalOffset(dst []byte, offset int) []byte {
	return binary.LittleEndian.AppendUint32(dst, uint32(offset))
}

// UpdateOffset updates the offset in dst
func UpdateOffset(dst []byte, offset int) {
	binary.LittleEndian.PutUint32(dst, uint32(offset))
}
