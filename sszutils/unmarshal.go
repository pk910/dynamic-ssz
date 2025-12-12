// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
	"io"
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
	return uint8(src[0])
}

// UnmarshalBool unmarshals a boolean from the src input
func UnmarshalBool(src []byte) bool {
	return src[0] == 1
}

// ---- offset functions ----

// ReadOffset reads an offset from buf
func ReadOffsetReader(reader io.Reader) (uint64, error) {
	var buf [4]byte
	if read, err := io.ReadFull(reader, buf[:]); err != nil || read != 4 {
		return 0, ErrUnexpectedEOF
	}
	return uint64(binary.LittleEndian.Uint32(buf[:])), nil
}

// UnmarshallUint64 unmarshals a little endian uint64 from the src input
func UnmarshallUint64Reader(reader io.Reader) (uint64, error) {
	var buf [8]byte
	if read, err := io.ReadFull(reader, buf[:]); err != nil || read != 8 {
		return 0, ErrUnexpectedEOF
	}
	return binary.LittleEndian.Uint64(buf[:]), nil
}

// UnmarshallUint32 unmarshals a little endian uint32 from the src input
func UnmarshallUint32Reader(reader io.Reader) (uint32, error) {
	var buf [4]byte
	if read, err := io.ReadFull(reader, buf[:]); err != nil || read != 4 {
		return 0, ErrUnexpectedEOF
	}
	return binary.LittleEndian.Uint32(buf[:]), nil
}

// UnmarshallUint16 unmarshals a little endian uint16 from the src input
func UnmarshallUint16Reader(reader io.Reader) (uint16, error) {
	var buf [2]byte
	if read, err := io.ReadFull(reader, buf[:]); err != nil || read != 2 {
		return 0, ErrUnexpectedEOF
	}
	return binary.LittleEndian.Uint16(buf[:]), nil
}

// UnmarshallUint8 unmarshals a little endian uint8 from the src input
func UnmarshallUint8Reader(reader io.Reader) (uint8, error) {
	var buf [1]byte
	if read, err := io.ReadFull(reader, buf[:]); err != nil || read != 1 {
		return 0, ErrUnexpectedEOF
	}
	return uint8(buf[0]), nil
}

// UnmarshalBool unmarshals a boolean from the src input
func UnmarshalBoolReader(reader io.Reader) (bool, error) {
	var buf [1]byte
	if read, err := io.ReadFull(reader, buf[:]); err != nil || read != 1 {
		return false, ErrUnexpectedEOF
	}
	if buf[0] != 1 && buf[0] != 0 {
		return false, ErrInvalidValueRange
	}
	return buf[0] == 1, nil
}

// ---- offset functions ----

// ReadOffset reads an offset from buf
func ReadOffset(buf []byte) uint64 {
	return uint64(binary.LittleEndian.Uint32(buf))
}

// ---- expansion functions ----

// ExpandSlice expands a slice to a byte slice
func ExpandSlice[T any](src []T, size int) []T {
	if len(src) < size {
		src = make([]T, size)
	} else if len(src) > size {
		src = src[:size]
	}

	return src
}
