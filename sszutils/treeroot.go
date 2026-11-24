// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

import (
	"math"
	"math/bits"
	"unsafe"
)

// CalculateLimit computes the merkle tree chunk limit for a list or vector
// given its maximum capacity, current number of items, and per-item byte size.
//
// ceil(maxCapacity*size/32) is computed in 128-bit so a large ssz-max cannot
// overflow uint64 and collapse to a small limit (which would make unrelated list
// types share a merkleization depth and collide).
func CalculateLimit(maxCapacity, numItems, size uint64) uint64 {
	hi, lo := bits.Mul64(maxCapacity, size)
	lo, carry := bits.Add64(lo, 31, 0)
	hi += carry
	if hi>>5 != 0 {
		// (product+31)/32 exceeds uint64; clamp to the max representable limit.
		return math.MaxUint64
	}
	limit := hi<<59 | lo>>5
	if limit != 0 {
		return limit
	}
	if numItems == 0 {
		return 1
	}
	return numItems
}

// CalculateBitlistLimit computes the merkle tree chunk limit for a bitlist with
// the given maximum bit capacity as ceil(maxSize/256). The computation avoids the
// uint64 overflow that a naive (maxSize+255)/256 hits near math.MaxUint64, which
// would otherwise collapse a huge limit to a tiny one and make unrelated bitlist
// types share a merkleization depth and collide.
func CalculateBitlistLimit(maxSize uint64) uint64 {
	if maxSize > math.MaxUint64-255 {
		return maxSize/256 + 1
	}
	return (maxSize + 255) / 256
}

// NextPowerOfTwo returns the smallest power of two greater than or equal to v.
func NextPowerOfTwo(v uint64) uint64 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v |= v >> 32
	v++
	return v
}

// HashUint64Slice appends the little-endian encoding of a uint64 slice directly
// to a HashWalker's buffer. On little-endian architectures (x86, ARM64) this is
// a single bulk memory copy, avoiding per-element AppendUint64 overhead.
// Big-endian architectures append each element individually to keep the hashed
// representation little-endian.
func HashUint64Slice[T ~uint64](hh HashWalker, s []T) {
	if len(s) == 0 {
		return
	}
	if !hostLittleEndian {
		for _, v := range s {
			hh.AppendUint64(uint64(v))
		}
		return
	}
	b := unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*8)
	if len(b) >= sharedAppendMin {
		if sh, ok := hh.(sharedAppender); ok {
			sh.AppendShared(b)
			return
		}
	}
	hh.Append(b)
}

// sharedAppendMin is the size from which a slice is offered to the hasher
// without a copy.
const sharedAppendMin = 1 << 21

// sharedAppender is implemented by a hasher that can hash appended bytes
// from the caller's memory, which then has to stay unchanged until the
// scope closes.
type sharedAppender interface {
	AppendShared(b []byte)
}

// ParallelWalkMin is the element count from which the elements of a list are
// offered to the hasher for a parallel walk.
const ParallelWalkMin = 2048

// ParallelWalker is an optional capability of a HashWalker: walking the
// elements of the innermost open scope on several goroutines. Each element
// must leave exactly one chunk in the scope, and walk must hash the elements
// [from, to) into the walker it is given without changing the value.
type ParallelWalker interface {
	WalkParallel(n int, walk func(hh HashWalker, from, to int) error) error
}

// WalkElements hashes the n single-leaf elements of the innermost open scope
// of hh by calling walk for ranges of them. A long list goes to the walker's
// parallel walk when it has one; otherwise walk is called once, for all
// elements, with hh.
func WalkElements(hh HashWalker, n int, walk func(hh HashWalker, from, to int) error) error {
	if n >= ParallelWalkMin {
		if pw, ok := hh.(ParallelWalker); ok {
			return pw.WalkParallel(n, walk)
		}
	}
	return walk(hh, 0, n)
}
