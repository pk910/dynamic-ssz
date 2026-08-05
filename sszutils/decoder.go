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
// # Decoder state after an error
//
// A decoder must not be reused once a decode has failed. Generated decoders
// return from inside a pushed region without popping it, so the limit stack is
// left unbalanced and every subsequent read is clamped to a stale region. This
// predates open regions — the leaked entry used to be a bounded limit and is
// now the open marker — and the reflection walk does pop before returning, so
// the two paths differ in what they leave behind. Discard the decoder and build
// a new one; the library's own entry points already do.
//
// Inside an open region GetLength cannot report the true remaining length.
// It deliberately does not return a sentinel: it reports the remaining
// *allowance* (a finite, plausible upper bound derived from the decoder's
// maximum stream size) so that code which has not been taught about open
// regions degrades to a clean ErrUnexpectedEOF at the real end of input rather
// than deriving an absurd element count or allocation size. Use LengthKnown to
// distinguish the two cases, More to test for remaining data, and
// DecodeRemaining to consume a region of unknown extent.
//
// Decoder is not a supported third-party implementation interface: it exists
// for dynamic-ssz generated code and the library's own decoders, and its
// method set may change between minor releases.
type Decoder interface {
	Seekable() bool   // can use DecodeOffsetAt() and SkipBytes()
	GetPosition() int // return current position
	GetLength() int   // return remaining length (allowance inside an open region)

	// RegionOpen reports whether the current region has no declared end and so
	// runs to EOF. Only the trailing dynamic child at each nesting level can be
	// open, and only while decoding input of unknown length.
	RegionOpen() bool

	// LengthKnown reports whether GetLength is backed by input known to exist,
	// and so may be used to size an allocation.
	//
	// A declared extent is not enough. Inside input of unknown length an offset
	// is validated against the allowance rather than against delivered bytes,
	// so a peer can declare a 500 MB field in a 12-byte payload. The extent is
	// trustworthy once the region fits inside an established total length --
	// supplied by the caller, or discovered at EOF -- or once it fits in bytes
	// already read, which a peer cannot fake without sending them. Reaching EOF
	// alone is not enough: it makes the length exact but leaves a region derived
	// from an earlier offset still claiming bytes that never arrived.
	LengthKnown() bool

	// Available reports how many bytes of the current region are already in
	// memory. It is a lower bound on what the region really contains, so an
	// allocation sized from it is backed by input that demonstrably arrived.
	Available() int

	// More reports whether the current region holds at least one more byte.
	// Inside an open region it probes the underlying reader, so it may fail.
	More() (bool, error)

	// DecodeRemaining consumes the rest of the current region (to the region
	// limit, or to EOF for an open region) and returns it in a newly allocated
	// slice the caller may retain. If maxLen is non-negative and the payload
	// exceeds it, decoding fails instead of allocating. A bounded region the
	// input cannot fill fails with ErrUnexpectedEOF.
	DecodeRemaining(maxLen int) ([]byte, error)

	// PushOpenLimit pushes a region that extends to the end of the input.
	// For decoders with a known length, and for open regions nested inside a
	// bounded one, this is equivalent to PushLimit(GetLength()).
	PushOpenLimit()

	// FinishRegion pops the current region, asserting it was fully consumed.
	//
	// For a bounded region this is PopLimit() == 0. An open region has no
	// known end, so it is established by probing for further input -- which
	// matters because a variable-size type can still stop short: a union
	// selecting a fixed-size variant, an absent optional, or an optional-list
	// with a fixed element all consume less than the region they were given.
	FinishRegion() error

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
