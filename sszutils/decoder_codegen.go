// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import "errors"

// Helpers shared by generated decoders. They keep the emitted code compact by
// absorbing the difference between a region of known extent and an open region
// whose end is only discovered at EOF.

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
// the payload is read to EOF. max caps the result (-1 for no cap) and is
// applied before the payload is allocated.
func DecodeByteListInto(dec Decoder, dst []byte, max int) ([]byte, error) {
	if dec.LengthKnown() {
		n := dec.GetLength()
		if max >= 0 && n > max {
			return nil, ErrListLengthFn(n, max)
		}
		dst = ExpandSlice(dst, n)
		if n > 0 {
			if _, err := dec.DecodeBytes(dst); err != nil {
				return nil, err
			}
		}
		return dst, nil
	}

	buf, err := dec.DecodeRemaining(max)
	if err != nil {
		if max >= 0 && isStreamTooLarge(err) {
			return nil, ErrListLengthFn(max+1, max)
		}
		return nil, err
	}
	return buf, nil
}

// DecodeUint64ListInto consumes a uint64 list that runs to the end of the
// current region, reusing dst when the length is known and growing to EOF
// otherwise. max caps the element count (-1 for no cap).
func DecodeUint64ListInto[T ~uint64](dec Decoder, dst []T, max int) ([]T, error) {
	if dec.LengthKnown() {
		sszLen := dec.GetLength()
		if sszLen%8 != 0 {
			return nil, ErrListNotAlignedFn(sszLen, 8)
		}
		count := sszLen / 8
		if max >= 0 && count > max {
			return nil, ErrListLengthFn(count, max)
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
		if max >= 0 && count >= max {
			return nil, ErrListLengthFn(count+1, max)
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
