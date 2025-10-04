// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.
//go:build cgo
// +build cgo

package cgo

import (
	"fmt"
	"unsafe"

	"github.com/OffchainLabs/hashtree"
)

func HashtreeHashByteSlice(digests []byte, chunks []byte) error {
	if len(chunks) == 0 {
		return nil
	}
	if len(chunks)%64 != 0 {
		return fmt.Errorf("chunks not multiple of 64 bytes")
	}
	if len(digests)%32 != 0 {
		return fmt.Errorf("digests not multiple of 32 bytes")
	}
	if len(digests) < len(chunks)/2 {
		return fmt.Errorf("not enough digest length, need at least %d, got %d", len(chunks)/2, len(digests))
	}
	// We use an unsafe pointer to cast []byte to [][32]byte. The length and
	// capacity of the slice need to be divided accordingly by 32.
	sizeChunks := (len(chunks) >> 5)
	chunkedChunks := unsafe.Slice((*[32]byte)(unsafe.Pointer(&chunks[0])), sizeChunks)

	sizeDigests := (len(digests) >> 5)
	chunkedDigest := unsafe.Slice((*[32]byte)(unsafe.Pointer(&digests[0])), sizeDigests)

	hashtree.Hash(chunkedDigest, chunkedChunks)

	return nil
}
