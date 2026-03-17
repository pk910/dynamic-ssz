// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
//
// This file contains code derived from https://github.com/ferranbt/fastssz/blob/v1.0.0/hasher.go
// Copyright (c) 2020 Ferran Borreguero
// Licensed under the MIT License
// The code has been modified for dynamic-ssz needs.

package hasher

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"math/bits"
	"sync"

	"github.com/pk910/dynamic-ssz/sszutils"
	hashtree "github.com/pk910/hashtree-bindings"
)

var debug = false

// DefaultHasherPool is a default hasher pool
var DefaultHasherPool HasherPool

// FastHasherPool is the fast hasher pool that uses the hashtree library if cgo is enabled
var FastHasherPool HasherPool = HasherPool{
	HashFn: hashtree.HashByteSlice,
}

var hasherInitialized bool
var hasherInitMutex sync.Mutex
var zeroHashes [65][32]byte
var zeroHashLevels map[string]int
var zeroBytes []byte

func initHasher() {
	hasherInitMutex.Lock()
	defer hasherInitMutex.Unlock()

	if hasherInitialized {
		return
	}

	hasherInitialized = true
	zeroBytes = sszutils.ZeroBytes()
	zeroHashLevels = make(map[string]int)
	zeroHashLevels[string(zeroBytes[:32])] = 0

	tmp := [64]byte{}
	for i := 0; i < 64; i++ {
		copy(tmp[:32], zeroHashes[i][:])
		copy(tmp[32:], zeroHashes[i][:])
		zeroHashes[i+1] = sha256.Sum256(tmp[:])
		zeroHashLevels[string(zeroHashes[i+1][:])] = i + 1
	}
}

// GetZeroHashLevel returns the merkle tree depth level for a known zero hash.
// Returns the level and true if the hash is a recognized zero hash, or 0 and
// false otherwise.
func GetZeroHashLevel(hash string) (int, bool) {
	level, ok := zeroHashLevels[hash]
	return level, ok
}

// GetZeroHash returns the precomputed zero hash at the given merkle tree depth.
func GetZeroHash(depth int) []byte {
	if !hasherInitialized {
		initHasher()
	}
	return zeroHashes[depth][:]
}

// GetZeroHashes returns the full array of precomputed zero hashes for all 65
// merkle tree depth levels.
func GetZeroHashes() [65][32]byte {
	return zeroHashes
}

func logfn(format string, a ...any) {
	fmt.Printf(format, a...)
}

// HashFn is a function that hashes pairs of 32-byte chunks from input into dst.
// It processes the input as consecutive 64-byte pairs and writes each 32-byte
// hash result into dst.
type HashFn func(dst []byte, input []byte) error

// NativeHashWrapper wraps a hash.Hash function into a HashFn
func NativeHashWrapper(hashFn hash.Hash) HashFn {
	return func(dst []byte, input []byte) error {
		hash := func(dst []byte, src []byte) {
			hashFn.Write(src[:32])
			hashFn.Write(src[32:64])
			hashFn.Sum(dst)
			hashFn.Reset()
		}

		layerLen := len(input) / 32
		if layerLen%2 == 1 {
			layerLen++
		}
		for i := 0; i < layerLen; i += 2 {
			hash(dst[(i/2)*32:][:0], input[i*32:])
		}
		return nil
	}
}

// WithDefaultHasher acquires a Hasher from the FastHasherPool, passes it to
// fn, and returns it to the pool when done. This is a convenience wrapper for
// one-off hashing operations.
func WithDefaultHasher(fn func(hh sszutils.HashWalker) error) error {
	hh := FastHasherPool.Get()
	defer func() {
		FastHasherPool.Put(hh)
	}()

	return fn(hh)
}

// HasherPool may be used for pooling Hashers for similarly typed SSZs.
type HasherPool struct {
	HashFn HashFn
	pool   sync.Pool
}

// Get acquires a Hasher from the pool.
func (hh *HasherPool) Get() *Hasher {
	h := hh.pool.Get()
	if h == nil {
		if hh.HashFn == nil {
			return NewHasher()
		} else {
			return NewHasherWithHashFn(hh.HashFn)
		}
	}
	hasher, _ := h.(*Hasher)
	return hasher
}

// Put releases the Hasher to the pool.
func (hh *HasherPool) Put(h *Hasher) {
	h.Reset()
	hh.pool.Put(h)
}

// ParseBitlist decodes an SSZ-encoded bitlist into its raw bit representation.
//
// SSZ bitlists include a mandatory termination bit: a single `1` bit appended
// immediately after the final data bit, then padded to a full byte. The position
// of this termination bit defines the logical length of the bitlist.
//
// This function performs the inverse transformation:
//  1. Identify the termination bit in the final byte and compute the logical
//     bitlist length (`size`).
//  2. Clear the termination bit, leaving only the actual data bits.
//  3. Trim any trailing zero bytes introduced by SSZ padding.
//  4. Return the compact raw bitlist (no termination bit, no padding) together
//     with its logical size.
//
// The returned `[]byte` contains the data bits packed little-endian in each byte,
// and `size` is the exact number of meaningful bits in that raw bitlist.
func ParseBitlist(dst, buf []byte) ([]byte, uint64) {
	if len(buf) == 0 {
		return dst, 0
	}
	msbLen := bits.Len8(buf[len(buf)-1])
	if msbLen == 0 {
		// No sentinel bit found in last byte — invalid bitlist, treat as empty
		return dst, 0
	}
	msb := uint8(msbLen) - 1
	size := uint64(8*(len(buf)-1) + int(msb))

	dstlen := len(dst)
	dst = append(dst, buf...)
	dst[len(dst)-1] &^= uint8(1 << msb)

	newLen := len(dst)
	for i := len(dst) - 1; i >= dstlen; i-- {
		if dst[i] != 0x00 {
			break
		}
		newLen = i
	}
	res := dst[:newLen]
	return res, size
}

// ParseBitlistWithHasher decodes an SSZ-encoded bitlist using the hasher's
// internal buffer to avoid allocations. It returns the raw bit data and the
// logical bit count, same as ParseBitlist.
//
// IMPORTANT: The returned bitlist slice references the hasher's internal buffer
// spare capacity. It must be consumed (e.g. passed to AppendBytes32) before any
// operation that may grow the buffer, as a reallocation would invalidate it.
func ParseBitlistWithHasher(hw sszutils.HashWalker, buf []byte) ([]byte, uint64) {
	if h, ok := hw.(*Hasher); ok {
		var size uint64
		buflen := len(h.buf)
		h.buf, size = ParseBitlist(h.buf, buf)
		bitlist := h.buf[buflen:]
		// Restore h.tmp to full capacity so subsequent operations (PutUint8, etc.) don't panic
		h.buf = h.buf[:buflen]
		return bitlist, size
	} else {
		var size uint64
		var bitlist []byte
		hw.WithTemp(func(tmp []byte) []byte {
			tmp, size = ParseBitlist(tmp[:0], buf)
			bitlist = tmp
			// Restore tmp to full capacity
			return tmp[:cap(tmp)]
		})
		return bitlist, size
	}
}
