// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

// Package sszutils provides shared interfaces, encoding/decoding primitives,
// and utility functions for SSZ (Simple Serialize) operations.
//
// It defines the core interfaces used across the dynamic-ssz library:
// encoder/decoder abstractions for both buffer and stream modes, hash walker
// for merkle tree computation, and compatibility interfaces for interoperating
// with fastssz and dynamic SSZ implementations.
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

// FastsszHashRoot is the interface implemented by types that can compute their
// SSZ hash tree root using fastssz.
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

// DynamicHashRoot is the interface implemented by types that can compute their
// SSZ hash tree root using dynamic specification values and a HashWalker.
type DynamicHashRoot interface {
	HashTreeRootWithDyn(ds DynamicSpecs, hh HashWalker) error
}

// DynamicSpecs is the interface for resolving dynamic specification values at
// runtime. Implementations provide named values (e.g., "SYNC_COMMITTEE_SIZE")
// that control SSZ field sizes.
//
// ResolveSpecValue returns whether the named value exists, its uint64 value,
// and any error. The name may be a simple identifier or a mathematical
// expression referencing other spec values.
type DynamicSpecs interface {
	ResolveSpecValue(name string) (bool, uint64, error)
}

// TreeType specifies the merkle tree shape for an SSZ object scope.
type TreeType uint8

const (
	// TreeTypeBinary is the standard SSZ binary merkle tree.
	TreeTypeBinary TreeType = iota
	// TreeTypeProgressive is the progressive merkle tree (subtree_fill_progressive).
	TreeTypeProgressive
	// TreeTypeNone disables incremental hashing for this scope (testing/debug).
	TreeTypeNone
)

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
	StartTree(treeType TreeType) int
	Collapse() // Hint to collapse accumulated chunks if threshold is reached

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
