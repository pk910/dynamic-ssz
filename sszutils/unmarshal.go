// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import "encoding/binary"

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

// ReadOffset reads an offset from buf
func ReadOffset(buf []byte) uint64 {
	return uint64(binary.LittleEndian.Uint32(buf))
}

// ExpandSlice expands a slice to a byte slice
func ExpandSlice[T any](src []T, size int) []T {
	if len(src) < size {
		src = make([]T, size)
	} else if len(src) > size {
		src = src[:size]
	}

	return src
}

// UnmarshalStaticList unmarshals a list with static items from the src input with a callback function for each item
func UnmarshalStaticList[C any, T any](ctx *C, buf []byte, val *[]T, itemSize int, isArray bool, itemCb func(ctx *C, buf []byte, val *T) error) error {
	itemCount := len(buf) / itemSize
	if len(buf)%itemSize != 0 {
		return ErrUnexpectedEOF
	}

	if !isArray {
		*val = ExpandSlice(*val, itemCount)
	}

	for i := 0; i < itemCount; i++ {
		buf := buf[itemSize*i : itemSize*(i+1)]

		err := itemCb(ctx, buf, &(*val)[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// UnmarshalDynamicList unmarshals a list with dynamic items from the src input with a callback function for each item
func UnmarshalDynamicList[C any, T any](ctx *C, buf []byte, val *[]T, isArray bool, itemCb func(ctx *C, buf []byte, val *T) error) error {
	startOffset := int(0)
	if len(buf) != 0 {
		if len(buf) < 4 {
			return ErrUnexpectedEOF
		}
		startOffset = int(UnmarshallUint32(buf[0:4]))
	}

	itemCount := startOffset / 4
	if startOffset%4 != 0 || len(buf) < startOffset {
		return ErrUnexpectedEOF
	}

	if !isArray {
		*val = ExpandSlice(*val, itemCount)
	}

	for i := 0; i < itemCount; i++ {
		var endOffset int
		if i < itemCount-1 {
			endOffset = int(UnmarshallUint32(buf[(i+1)*4 : (i+2)*4]))
		} else {
			endOffset = len(buf)
		}
		if endOffset < startOffset || endOffset > len(buf) {
			return ErrOffset
		}
		buf := buf[startOffset:endOffset]
		startOffset = endOffset

		err := itemCb(ctx, buf, &(*val)[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// UnmarshalStaticVector unmarshals a vector with static items from the src input with a callback function for each item
func UnmarshalStaticVector[C any, T any](ctx *C, buf []byte, val *[]T, vectorSize int, itemSize int, isArray bool, noBufCheck bool, itemCb func(ctx *C, buf []byte, val *T) error) error {
	if !isArray {
		*val = ExpandSlice(*val, vectorSize)
	}

	if !noBufCheck {
		if len(buf) < vectorSize*itemSize {
			return ErrUnexpectedEOF
		}
	}

	for i := 0; i < vectorSize; i++ {
		buf := buf[i*itemSize : (i+1)*itemSize]

		err := itemCb(ctx, buf, &(*val)[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// UnmarshalDynamicVector unmarshals a vector with dynamic items from the src input with a callback function for each item
func UnmarshalDynamicVector[C any, T any](ctx *C, buf []byte, val *[]T, vectorSize int, isArray bool, noBufCheck bool, itemCb func(ctx *C, buf []byte, val *T) error) error {
	if !isArray {
		*val = ExpandSlice(*val, vectorSize)
	}

	if vectorSize*4 > len(buf) {
		return ErrUnexpectedEOF
	}

	startOffset := int(UnmarshallUint32(buf[0:4]))
	for i := 0; i < vectorSize; i++ {
		var endOffset int

		if i < vectorSize-1 {
			endOffset = int(UnmarshallUint32(buf[(i+1)*4 : (i+2)*4]))
		} else {
			endOffset = len(buf)
		}

		if endOffset < startOffset || endOffset > len(buf) {
			return ErrOffset
		}

		buf := buf[startOffset:endOffset]
		startOffset = endOffset

		err := itemCb(ctx, buf, &(*val)[i])
		if err != nil {
			return err
		}
	}

	return nil
}
