// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import "encoding/binary"

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

// MarshalStaticList marshals a list with static items from the src input with a callback function for each item
func MarshalStaticList[C any, T any](ctx *C, dst []byte, val *[]T, itemSize int, isArray bool, itemCb func(ctx *C, dst []byte, val *T) ([]byte, error)) ([]byte, error) {
	vlen := len(*val)
	for i := 0; i < vlen; i++ {
		if buf, err := itemCb(ctx, dst, &(*val)[i]); err != nil {
			return nil, err
		} else {
			dst = buf
		}
	}

	return dst, nil
}

// MarshalDynamicList marshals a list with dynamic items from the src input with a callback function for each item
func MarshalDynamicList[C any, T any](ctx *C, dst []byte, val *[]T, isArray bool, itemCb func(ctx *C, dst []byte, val *T) ([]byte, error)) ([]byte, error) {
	dstlen := len(dst)
	vlen := len(*val)
	dst = AppendZeroPadding(dst, vlen*4)

	for i := 0; i < vlen; i++ {
		UpdateOffset(dst[dstlen+(i*4):dstlen+((i+1)*4)], len(dst)-dstlen)
		if buf, err := itemCb(ctx, dst, &(*val)[i]); err != nil {
			return nil, err
		} else {
			dst = buf
		}
	}

	return dst, nil
}

// UnmarshalStaticVector unmarshals a vector with static items from the src input with a callback function for each item
func MarshalStaticVector[C any, T any](ctx *C, dst []byte, val *[]T, vectorSize int, itemSize int, isArray bool, itemCb func(ctx *C, dst []byte, val *T) ([]byte, error)) ([]byte, error) {
	len := len(*val)
	if len > vectorSize {
		if !isArray {
			return nil, ErrVectorLength
		}
		len = vectorSize
	}

	for i := 0; i < len; i++ {
		if buf, err := itemCb(ctx, dst, &(*val)[i]); err != nil {
			return nil, err
		} else {
			dst = buf
		}
	}

	if !isArray && len < vectorSize {
		dst = AppendZeroPadding(dst, (vectorSize-len)*itemSize)
	}

	return dst, nil
}

// MarshalDynamicVector marshals a vector with dynamic items from the src input with a callback function for each item
func MarshalDynamicVector[C any, T any](ctx *C, dst []byte, val *[]T, vectorSize int, isArray bool, itemCb func(ctx *C, dst []byte, val *T) ([]byte, error)) ([]byte, error) {
	vallen := len(*val)
	if vallen > vectorSize {
		if !isArray {
			return nil, ErrVectorLength
		}
		vallen = vectorSize
	}

	dstlen := len(dst)
	dst = AppendZeroPadding(dst, vectorSize*4)
	for i := 0; i < vallen; i++ {
		UpdateOffset(dst[dstlen+(i*4):dstlen+((i+1)*4)], len(dst)-dstlen)
		if buf, err := itemCb(ctx, dst, &(*val)[i]); err != nil {
			return nil, err
		} else {
			dst = buf
		}
	}

	if !isArray && vectorSize > vallen {
		zeroItem := new(T)
		for i := vallen; i < vectorSize; i++ {
			UpdateOffset(dst[dstlen+(i*4):dstlen+((i+1)*4)], len(dst)-dstlen)
			if buf, err := itemCb(ctx, dst, zeroItem); err != nil {
				return nil, err
			} else {
				dst = buf
			}
		}
	}

	return dst, nil
}
