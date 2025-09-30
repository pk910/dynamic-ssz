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

// DynamicUnmarshaler is the interface implemented by types that can unmarshal using dynamic SSZ
type DynamicUnmarshaler interface {
	UnmarshalSSZDyn(ds DynamicSpecs, buf []byte) error
}

// DynamicSizer is the interface implemented by types that can calculate their own SSZ size dynamically
type DynamicSizer interface {
	SizeSSZDyn(ds DynamicSpecs) int
}

type DynamicHashRoot interface {
	HashTreeRootDyn(ds DynamicSpecs, hh HashWalker) error
}

// DynamicSsz is the interface for a dynamic SSZ encoder/decoder
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
}
