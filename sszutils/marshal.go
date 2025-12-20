// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
	"io"
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
	dst = append(dst, byte(i))
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

// MarshalOffset marshals an offset to dst
func MarshalOffset(dst []byte, offset int) []byte {
	return binary.LittleEndian.AppendUint32(dst, uint32(offset))
}

// UpdateOffset updates the offset in dst
func UpdateOffset(dst []byte, offset int) {
	binary.LittleEndian.PutUint32(dst, uint32(offset))
}

// MarshalUint64Writer writes a little endian uint64 to dst
func MarshalUint64Writer(dst io.Writer, i uint64) error {
	return binary.Write(dst, binary.LittleEndian, i)
}

// MarshalUint32Writer writes a little endian uint32 to dst
func MarshalUint32Writer(dst io.Writer, i uint32) error {
	return binary.Write(dst, binary.LittleEndian, i)
}

// MarshalUint16Writer writes a little endian uint16 to dst
func MarshalUint16Writer(dst io.Writer, i uint16) error {
	return binary.Write(dst, binary.LittleEndian, i)
}

// MarshalUint8Writer writes a little endian uint8 to dst
func MarshalUint8Writer(dst io.Writer, i uint8) error {
	_, err := dst.Write([]byte{byte(i)})
	return err
}

// MarshalBoolWriter writes a boolean to dst
func MarshalBoolWriter(dst io.Writer, b bool) error {
	if b {
		_, err := dst.Write([]byte{1})
		return err
	} else {
		_, err := dst.Write([]byte{0})
		return err
	}
}

// MarshalOffsetWriter writes an offset to dst
func MarshalOffsetWriter(dst io.Writer, offset uint32) error {
	return binary.Write(dst, binary.LittleEndian, offset)
}
