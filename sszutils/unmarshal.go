// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"encoding/binary"
	"errors"
	"unsafe"
)

// ---- Unmarshal functions ----

// UnmarshallUint64 unmarshals a little endian uint64 from the src input. A short
// buffer is treated as zero-padded rather than panicking.
func UnmarshallUint64(src []byte) uint64 {
	if len(src) < 8 {
		var b [8]byte
		copy(b[:], src)
		return binary.LittleEndian.Uint64(b[:])
	}
	return binary.LittleEndian.Uint64(src)
}

// UnmarshallUint32 unmarshals a little endian uint32 from the src input. A short
// buffer is treated as zero-padded rather than panicking.
func UnmarshallUint32(src []byte) uint32 {
	if len(src) < 4 {
		var b [4]byte
		copy(b[:], src)
		return binary.LittleEndian.Uint32(b[:])
	}
	return binary.LittleEndian.Uint32(src[:4])
}

// UnmarshallUint16 unmarshals a little endian uint16 from the src input. A short
// buffer is treated as zero-padded rather than panicking.
func UnmarshallUint16(src []byte) uint16 {
	if len(src) < 2 {
		var b [2]byte
		copy(b[:], src)
		return binary.LittleEndian.Uint16(b[:])
	}
	return binary.LittleEndian.Uint16(src[:2])
}

// UnmarshallUint8 unmarshals a little endian uint8 from the src input. An empty
// buffer decodes to zero rather than panicking.
func UnmarshallUint8(src []byte) uint8 {
	if len(src) < 1 {
		return 0
	}
	return src[0]
}

// UnmarshalBool unmarshals a boolean from the src input. An empty buffer decodes
// to false rather than panicking.
func UnmarshalBool(src []byte) bool {
	if len(src) < 1 {
		return false
	}
	return src[0] == 1
}

// UnmarshalUint64Slice decodes little-endian encoded uint64 values from buf into dst.
// On little-endian architectures (x86, ARM64) this is a single bulk memory copy.
// Big-endian architectures decode each element individually. Both paths tolerate
// a short buf by decoding only the fully covered elements, leaving a trailing
// partially covered element untouched; callers are expected to have validated
// the buffer length already.
func UnmarshalUint64Slice[T ~uint64](dst []T, buf []byte) {
	if len(dst) == 0 {
		return
	}
	if !hostLittleEndian {
		n := min(len(dst), len(buf)/8)
		for i := range n {
			dst[i] = T(binary.LittleEndian.Uint64(buf[i*8:]))
		}
		return
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(dst))), len(dst)*8), buf[:len(buf)-len(buf)%8])
}

// UnmarshalFixedBytesSlice bulk-copies a contiguous SSZ byte buffer into dst, a
// slice whose elements are fixed-size byte arrays (e.g. []Root where Root is
// [32]byte). dst must already have its final length. Because such elements are
// laid out contiguously with no padding, the whole slice is decoded with a single
// memory copy, avoiding the per-element copy loop. This is endian-neutral (raw
// bytes). The element size is taken from the element type itself.
func UnmarshalFixedBytesSlice[T any](dst []T, buf []byte) {
	if len(dst) == 0 {
		return
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(dst))), len(dst)*int(unsafe.Sizeof(dst[0]))), buf)
}

// DecodeUint64Slice decodes uint64 values from a Decoder directly into dst using bulk memory copy.
// On little-endian architectures (x86, ARM64) this avoids per-element DecodeUint64
// overhead. Big-endian architectures decode each element individually.
func DecodeUint64Slice[T ~uint64](dec Decoder, dst []T) error {
	if len(dst) == 0 {
		return nil
	}
	if !hostLittleEndian {
		for i := range dst {
			v, err := dec.DecodeUint64()
			if err != nil {
				return err
			}
			dst[i] = T(v)
		}
		return nil
	}
	_, err := dec.DecodeBytes(unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(dst))), len(dst)*8))
	return err
}

// ---- offset functions ----

// ReadOffset reads an offset from buf. A short buffer is treated as zero-padded
// rather than panicking.
func ReadOffset(buf []byte) uint64 {
	if len(buf) < 4 {
		var b [4]byte
		copy(b[:], buf)
		return uint64(binary.LittleEndian.Uint32(b[:]))
	}
	return uint64(binary.LittleEndian.Uint32(buf))
}

// ---- expansion functions ----

// ExpandSlice grows or shrinks src to length size, zeroing any newly added elements.
// For size == 0 it returns a non-nil empty slice.
//
//ssz:mustinline
func ExpandSlice[T any](src []T, size int) []T {
	if size < 0 {
		// Negative sizes are invalid input; return a non-nil empty slice so the
		// slice bounds below cannot underflow.
		return make([]T, 0)
	}
	if len(src) >= size {
		out := src[:size]
		if out == nil {
			// size == 0 and src == nil: return a non-nil empty slice so empty
			// lists always decode to [] rather than nil.
			out = make([]T, 0)
		}
		return out
	}

	if cap(src) < size {
		return make([]T, size)
	}

	prevLen := len(src)
	src = src[:size]
	clear(src[prevLen:])
	return src
}

// CredibleCount clamps a count declared by the input to what the bytes already
// read can account for, at elemSize bytes per element.
//
// A count taken from an offset or a region length is only a claim until the
// bytes behind it arrive. Sizing an allocation from the claim lets a peer turn a
// few delivered bytes into an arbitrary one; sizing it from what has arrived
// costs the peer the bytes. When the extent is already backed by known input the
// count is returned unchanged.
func CredibleCount(dec Decoder, count, elemSize int) int {
	if count <= 0 {
		return 0
	}
	if dec.LengthKnown() {
		return count
	}
	if elemSize <= 0 {
		return count
	}
	if delivered := dec.Available() / elemSize; delivered < count {
		return delivered
	}
	return count
}

// SizeListSlice sizes dst for a list of count elements of elemSize bytes.
//
// When the region's extent is backed by input known to exist, the slice is
// allocated exactly -- the common case, and what every buffer or known-length
// decode does. Otherwise the count is merely declared by the input, so
// allocating it outright would let a peer turn a few delivered bytes into an
// arbitrary allocation; the slice is instead seeded from the bytes that have
// actually arrived and grows as the rest does.
func SizeListSlice[T any](dec Decoder, dst []T, count, elemSize int) []T {
	if dec.LengthKnown() {
		return ExpandSlice(dst, count)
	}
	return GrowSlice(dst, CredibleCount(dec, count, elemSize))
}

// GrowSlice returns src resized to size, growing capacity geometrically.
//
// ExpandSlice allocates exactly the requested size, which is right when the
// element count is known up front. Decoding a list whose count is only
// discovered at EOF resizes once per element, so it needs amortised growth
// instead.
func GrowSlice[T any](src []T, size int) []T {
	if size < 0 {
		return make([]T, 0)
	}
	if size <= cap(src) {
		prevLen := len(src)
		src = src[:size]
		if src == nil {
			// size == 0 and src == nil: return a non-nil empty slice so an
			// empty list decodes to [] rather than nil, matching ExpandSlice
			// and therefore the buffer and known-length paths.
			return make([]T, 0)
		}
		if prevLen < size {
			clear(src[prevLen:])
		}
		return src
	}

	newCap := cap(src) * 2
	if newCap < size {
		newCap = size
	}
	if newCap < 8 {
		newCap = 8
	}
	out := make([]T, size, newCap)
	copy(out, src)
	return out
}

// DecodeByteListInto consumes a byte list that runs to the end of the current
// region. When the length is known the destination slice is reused; otherwise
// the payload is read to EOF. maxLen caps the result (-1 for no cap) and is
// applied before the payload is allocated.
func DecodeByteListInto(dec Decoder, dst []byte, maxLen int) ([]byte, error) {
	if dec.LengthKnown() {
		n := dec.GetLength()
		if maxLen >= 0 && n > maxLen {
			return nil, ErrListLengthFn(n, maxLen)
		}
		dst = ExpandSlice(dst, n)
		if n > 0 {
			if _, err := dec.DecodeBytes(dst); err != nil {
				return nil, err
			}
		}
		return dst, nil
	}

	buf, err := dec.DecodeRemaining(maxLen)
	if err != nil {
		if maxLen >= 0 && isStreamTooLarge(err) {
			return nil, ErrListLengthFn(maxLen+1, maxLen)
		}
		return nil, err
	}
	return buf, nil
}

// DecodeUint64ListInto consumes a uint64 list that runs to the end of the
// current region, reusing dst when the length is known and growing to EOF
// otherwise. maxLen caps the element count (-1 for no cap).
func DecodeUint64ListInto[T ~uint64](dec Decoder, dst []T, maxLen int) ([]T, error) {
	if dec.LengthKnown() {
		sszLen := dec.GetLength()
		if sszLen%8 != 0 {
			return nil, ErrListNotAlignedFn(sszLen, 8)
		}
		count := sszLen / 8
		if maxLen >= 0 && count > maxLen {
			return nil, ErrListLengthFn(count, maxLen)
		}
		dst = ExpandSlice(dst, count)
		if err := DecodeUint64Slice(dec, dst); err != nil {
			return nil, err
		}
		return dst, nil
	}

	dst = GrowSlice(dst, 0)
	for count := 0; ; count++ {
		more, err := dec.More()
		if err != nil {
			return nil, err
		}
		if !more {
			return dst, nil
		}
		if maxLen >= 0 && count >= maxLen {
			return nil, ErrListLengthFn(count+1, maxLen)
		}
		v, err := dec.DecodeUint64()
		if err != nil {
			return nil, err
		}
		dst = GrowSlice(dst, count+1)
		dst[count] = T(v)
	}
}

// DecodeDelegateBuffer materialises the region a buffer-based delegate needs.
// A delegate takes a []byte and so cannot consume an open region incrementally.
// size < 0 means "the whole region", which for an open region means reading to
// EOF.
func DecodeDelegateBuffer(dec Decoder, size int) ([]byte, error) {
	if size >= 0 {
		return dec.DecodeBytesBuf(size)
	}
	if !dec.LengthKnown() {
		return dec.DecodeRemaining(-1)
	}
	return dec.DecodeBytesBuf(dec.GetLength())
}

// RegionEmpty reports whether the current region holds no data. Generated code
// uses it where emptiness is a semantic discriminator (an absent optional, an
// empty dynamic list) rather than a validation check, since those cannot be
// answered from a region length that is not yet known.
func RegionEmpty(dec Decoder) (bool, error) {
	more, err := dec.More()
	if err != nil {
		return false, err
	}
	return !more, nil
}

func isStreamTooLarge(err error) bool {
	return errors.Is(err, ErrStreamTooLarge)
}
