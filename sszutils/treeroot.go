// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package sszutils

// CalculateLimit computes the merkle tree chunk limit for a list or vector
// given its maximum capacity, current number of items, and per-item byte size.
func CalculateLimit(maxCapacity, numItems, size uint64) uint64 {
	limit := (maxCapacity*size + 31) / 32
	if limit != 0 {
		return limit
	}
	if numItems == 0 {
		return 1
	}
	return numItems
}

// NextPowerOfTwo returns the smallest power of two greater than or equal to v.
func NextPowerOfTwo(v uint64) uint {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v++
	return uint(v)
}
