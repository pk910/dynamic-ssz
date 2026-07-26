// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

// Decoder is the interface for reading SSZ-encoded data. It supports both
// seekable (buffer-backed) and non-seekable (stream-backed) implementations.
//
// # Open regions
//
// A stream decoder created without a known total length (see
// NewUnknownStreamDecoder) decodes SSZ data whose size is only discovered at
// EOF. SSZ is self-delimiting for every region except the trailing one, so
// "unknown length" propagates down a single chain: the last dynamic child at
// each nesting level. Such a region is called an *open* region and is entered
// with PushOpenLimit.
//
// Inside an open region GetLength cannot report the true remaining length.
// It deliberately does not return a sentinel: it reports the remaining
// *allowance* (a finite, plausible upper bound derived from the decoder's
// maximum stream size) so that code which has not been taught about open
// regions degrades to a clean ErrUnexpectedEOF at the real end of input rather
// than deriving an absurd element count or allocation size. Use LengthKnown to
// distinguish the two cases, More to test for remaining data, and
// DecodeRemaining to consume a region of unknown extent.
type Decoder interface {
	Seekable() bool   // can use DecodeOffsetAt() and SkipBytes()
	GetPosition() int // return current position
	GetLength() int   // return remaining length (allowance inside an open region)

	// LengthKnown reports whether GetLength returns the exact number of bytes
	// remaining in the current region. It is false only inside an open region
	// of a stream decoder whose total length is not yet known.
	LengthKnown() bool

	// More reports whether the current region holds at least one more byte.
	// Inside an open region it probes the underlying reader, so it may fail.
	More() (bool, error)

	// DecodeRemaining consumes the rest of the current region (to the region
	// limit, or to EOF for an open region) and returns it in a newly allocated
	// slice the caller may retain. If max is non-negative and the payload
	// exceeds it, decoding fails instead of allocating.
	DecodeRemaining(max int) ([]byte, error)

	// PushOpenLimit pushes a region that extends to the end of the input.
	// For decoders with a known length, and for open regions nested inside a
	// bounded one, this is equivalent to PushLimit(GetLength()).
	PushOpenLimit()

	PushLimit(limit int)
	PopLimit() int
	DecodeBool() (bool, error)
	DecodeUint8() (uint8, error)
	DecodeUint16() (uint16, error)
	DecodeUint32() (uint32, error)
	DecodeUint64() (uint64, error)
	DecodeBytes(buf []byte) ([]byte, error)
	DecodeBytesBuf(len int) ([]byte, error)
	DecodeOffset() (uint32, error)
	DecodeOffsetAt(pos int) uint32
	SkipBytes(n int)
}
