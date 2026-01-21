// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

// FastsszMarshaler is the interface implemented by types that can marshal themselves into valid SZZ using fastssz.
type FastsszMarshaler interface {
	MarshalSSZTo(dst []byte) ([]byte, error)
	MarshalSSZ() ([]byte, error)
	SizeSSZ() int
}

// FastsszUnmarshaler is the interface implemented by types that can unmarshal a SSZ description of themselves
type FastsszUnmarshaler interface {
	UnmarshalSSZ(buf []byte) error
}

type FastsszHashRoot interface {
	HashTreeRoot() ([32]byte, error)
}

// DynamicMarshaler is the interface implemented by types that can marshal themselves using dynamic SSZ
type DynamicMarshaler interface {
	MarshalSSZDyn(ds DynamicSpecs, buf []byte) ([]byte, error)
}

// DynamicEncoder is the interface implemented by types that can marshal themselves using dynamic SSZ and an encoder
type DynamicEncoder interface {
	MarshalSSZEncoder(ds DynamicSpecs, encoder Encoder) error
}

// DynamicUnmarshaler is the interface implemented by types that can unmarshal using dynamic SSZ
type DynamicUnmarshaler interface {
	UnmarshalSSZDyn(ds DynamicSpecs, buf []byte) error
}

// DynamicDecoder is the interface implemented by types that can unmarshal using dynamic SSZ and a decoder
type DynamicDecoder interface {
	UnmarshalSSZDecoder(ds DynamicSpecs, decoder Decoder) error
}

// DynamicSizer is the interface implemented by types that can calculate their own SSZ size dynamically
type DynamicSizer interface {
	SizeSSZDyn(ds DynamicSpecs) int
}

// DynamicHashRoot is the interface implemented by types that can calculate their own SSZ hash tree root dynamically
type DynamicHashRoot interface {
	HashTreeRootWithDyn(ds DynamicSpecs, hh HashWalker) error
}

// DynamicViewMarshaler is the interface implemented by types that can marshal themselves using dynamic SSZ and a view.
// Returns nil if the view is not supported, otherwise returns the marshal function.
type DynamicViewMarshaler interface {
	MarshalSSZDynView(view any) func(ds DynamicSpecs, buf []byte) ([]byte, error)
}

// DynamicViewEncoder is the interface implemented by types that can marshal themselves using dynamic SSZ, an encoder, and a view.
// Returns nil if the view is not supported, otherwise returns the encode function.
type DynamicViewEncoder interface {
	MarshalSSZEncoderView(view any) func(ds DynamicSpecs, encoder Encoder) error
}

// DynamicViewUnmarshaler is the interface implemented by types that can unmarshal using dynamic SSZ and a view.
// Returns nil if the view is not supported, otherwise returns the unmarshal function.
type DynamicViewUnmarshaler interface {
	UnmarshalSSZDynView(view any) func(ds DynamicSpecs, buf []byte) error
}

// DynamicViewDecoder is the interface implemented by types that can unmarshal using dynamic SSZ, a decoder, and a view.
// Returns nil if the view is not supported, otherwise returns the decode function.
type DynamicViewDecoder interface {
	UnmarshalSSZDecoderView(view any) func(ds DynamicSpecs, decoder Decoder) error
}

// DynamicViewSizer is the interface implemented by types that can calculate their own SSZ size dynamically with a view.
// Returns nil if the view is not supported, otherwise returns the size function.
type DynamicViewSizer interface {
	SizeSSZDynView(view any) func(ds DynamicSpecs) int
}

// DynamicViewHashRoot is the interface implemented by types that can calculate their own SSZ hash tree root dynamically.
// Returns nil if the view is not supported, otherwise returns the hash function.
type DynamicViewHashRoot interface {
	HashTreeRootWithDynView(view any) func(ds DynamicSpecs, hh HashWalker) error
}

// DynamicSpecs is the interface for a dynamic SSZ encoder/decoder that provides
// specification values for dynamic sizing.
type DynamicSpecs interface {
	ResolveSpecValue(name string) (bool, uint64, error)
}

// HashWalker is our own interface that mirrors fastssz.HashWalker
// This allows us to avoid importing fastssz directly while still being
// compatible with types that implement HashTreeRootWith
type HashWalker interface {
	// Hash returns the latest hash generated during merkleize
	Hash() []byte

	// Methods for appending single values
	AppendBool(b bool)
	AppendUint8(i uint8)
	AppendUint16(i uint16)
	AppendUint32(i uint32)
	AppendUint64(i uint64)
	AppendBytes32(b []byte)

	// Methods for putting values into the buffer
	PutUint64Array(b []uint64, maxCapacity ...uint64)
	PutUint64(i uint64)
	PutUint32(i uint32)
	PutUint16(i uint16)
	PutUint8(i uint8)
	PutBitlist(bb []byte, maxSize uint64)
	PutProgressiveBitlist(bb []byte)
	PutBool(b bool)
	PutBytes(b []byte)

	// Buffer manipulation methods
	FillUpTo32()
	Append(i []byte)
	Index() int

	// temporary buffer methods
	WithTemp(func(tmp []byte) []byte)

	// Merkleization methods
	Merkleize(indx int)
	MerkleizeWithMixin(indx int, num, limit uint64)
	MerkleizeProgressive(indx int)
	MerkleizeProgressiveWithMixin(indx int, num uint64)
	MerkleizeProgressiveWithActiveFields(indx int, activeFields []byte)

	// HashRoot methods
	HashRoot() ([32]byte, error)
}
