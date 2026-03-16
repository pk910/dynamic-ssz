// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
//
// This file contains code derived from https://github.com/ferranbt/fastssz/blob/v1.0.0/hasher.go
// Copyright (c) 2020 Ferran Borreguero
// Licensed under the MIT License
// The code has been modified for dynamic-ssz needs.

// Package hasher provides SSZ merkle tree hashing utilities.
//
// It implements the SSZ hash tree root computation including merkleization,
// mixin with length, progressive merkleization, and bitlist handling.
// The package provides pooled hashers for efficient reuse and supports
// both the native Go sha256 implementation and the accelerated hashtree
// library via cgo.
package hasher

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"math/bits"
	"sync"

	"github.com/pk910/dynamic-ssz/sszutils"
	hashtree "github.com/pk910/hashtree-bindings"
)

// Compile-time check to ensure Hasher implements HashWalker interface
var _ sszutils.HashWalker = (*Hasher)(nil)

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
var trueBytes, falseBytes, zeroBytes []byte

func initHasher() {
	hasherInitMutex.Lock()
	defer hasherInitMutex.Unlock()

	if hasherInitialized {
		return
	}

	hasherInitialized = true
	falseBytes = make([]byte, 32)
	trueBytes = make([]byte, 32)
	zeroBytes = sszutils.ZeroBytes()
	trueBytes[0] = 1
	zeroHashLevels = make(map[string]int)
	zeroHashLevels[string(falseBytes)] = 0

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

// Hasher is a utility tool to hash SSZ structs
type Hasher struct {
	// buffer array to store hashing values
	buf []byte

	// tmp array used for uint64 and bitlist processing
	tmp []byte

	// sha256 hash function
	hash HashFn
}

// NewHasher creates a new Hasher object with sha256 hash
func NewHasher() *Hasher {
	return NewHasherWithHash(sha256.New())
}

// NewHasherWithHash creates a new Hasher object with a custom hash.Hash function
func NewHasherWithHash(hh hash.Hash) *Hasher {
	return NewHasherWithHashFn(NativeHashWrapper(hh))
}

// NewHasherWithHashFn creates a new Hasher object with a custom HashFn function
// defaultHasherBufSize is the default initial capacity for the hasher buffer.
// This is sized to handle most BeaconState HTR operations without regrowth
// (100K validators * 32 bytes per hash = 3.2MB peak).
const defaultHasherBufSize = 4 * 1024 * 1024

func NewHasherWithHashFn(hh HashFn) *Hasher {
	if !hasherInitialized {
		initHasher()
	}

	return &Hasher{
		buf:  make([]byte, 0, defaultHasherBufSize),
		hash: hh,
		tmp:  make([]byte, 64),
	}
}

// WithTemp provides access to the hasher's temporary buffer for use in
// callback fn. The callback receives the tmp buffer and must return a
// (possibly reallocated) replacement.
func (h *Hasher) WithTemp(fn func(tmp []byte) []byte) {
	h.tmp = fn(h.tmp)
}

// Reset resets the Hasher obj
func (h *Hasher) Reset() {
	h.buf = h.buf[:0]
}

// AppendBytes32 appends b to the hash buffer, right-padding with zeros to
// align to a 32-byte boundary.
func (h *Hasher) AppendBytes32(b []byte) {
	h.buf = append(h.buf, b...)
	if rest := len(b) % 32; rest != 0 {
		// pad zero bytes to the left
		h.buf = append(h.buf, zeroBytes[:32-rest]...)
	}
}

// PutUint64 appends a uint64 in 32 bytes
func (h *Hasher) PutUint64(i uint64) {
	n := len(h.buf)
	h.buf = append(h.buf, zeroBytes[:32]...)
	binary.LittleEndian.PutUint64(h.buf[n:], i)
}

// PutUint32 appends a uint32 in 32 bytes
func (h *Hasher) PutUint32(i uint32) {
	n := len(h.buf)
	h.buf = append(h.buf, zeroBytes[:32]...)
	binary.LittleEndian.PutUint32(h.buf[n:], i)
}

// PutUint16 appends a uint16 in 32 bytes
func (h *Hasher) PutUint16(i uint16) {
	n := len(h.buf)
	h.buf = append(h.buf, zeroBytes[:32]...)
	binary.LittleEndian.PutUint16(h.buf[n:], i)
}

// PutUint8 appends a uint8 in 32 bytes
func (h *Hasher) PutUint8(i uint8) {
	n := len(h.buf)
	h.buf = append(h.buf, zeroBytes[:32]...)
	h.buf[n] = i
}

// FillUpTo32 pads the hash buffer with zero bytes to align to a 32-byte
// boundary.
func (h *Hasher) FillUpTo32() {
	// pad zero bytes to the left
	if rest := len(h.buf) % 32; rest != 0 {
		h.buf = append(h.buf, zeroBytes[:32-rest]...)
	}
}

// AppendBool appends a single byte (0 or 1) representing the boolean value
// to the hash buffer without 32-byte padding.
func (h *Hasher) AppendBool(b bool) {
	if b {
		h.buf = append(h.buf, 1)
	} else {
		h.buf = append(h.buf, 0)
	}
}

// AppendUint8 appends a uint8 as a single byte to the hash buffer without
// 32-byte padding.
func (h *Hasher) AppendUint8(i uint8) {
	h.buf = sszutils.MarshalUint8(h.buf, i)
}

// AppendUint16 appends a little-endian uint16 to the hash buffer without
// 32-byte padding.
func (h *Hasher) AppendUint16(i uint16) {
	h.buf = sszutils.MarshalUint16(h.buf, i)
}

// AppendUint32 appends a little-endian uint32 to the hash buffer without
// 32-byte padding.
func (h *Hasher) AppendUint32(i uint32) {
	h.buf = sszutils.MarshalUint32(h.buf, i)
}

// AppendUint64 appends a little-endian uint64 to the hash buffer without
// 32-byte padding.
func (h *Hasher) AppendUint64(i uint64) {
	h.buf = sszutils.MarshalUint64(h.buf, i)
}

// Append appends raw bytes directly to the hash buffer without any padding.
func (h *Hasher) Append(i []byte) {
	h.buf = append(h.buf, i...)
}

// PutRootVector appends an array of roots
func (h *Hasher) PutRootVector(b [][]byte, maxCapacity ...uint64) error {
	indx := h.Index()
	for _, i := range b {
		if len(i) != 32 {
			return fmt.Errorf("bad root")
		}
		h.buf = append(h.buf, i...)
	}

	if len(maxCapacity) == 0 {
		h.Merkleize(indx)
	} else {
		numItems := uint64(len(b))
		limit := sszutils.CalculateLimit(maxCapacity[0], numItems, 32)

		h.MerkleizeWithMixin(indx, numItems, limit)
	}
	return nil
}

// PutUint64Array appends an array of uint64
func (h *Hasher) PutUint64Array(b []uint64, maxCapacity ...uint64) {
	indx := h.Index()
	for _, i := range b {
		h.AppendUint64(i)
	}

	// pad zero bytes to the left
	h.FillUpTo32()

	if len(maxCapacity) == 0 {
		// Array with fixed size
		h.Merkleize(indx)
	} else {
		numItems := uint64(len(b))
		limit := sszutils.CalculateLimit(maxCapacity[0], numItems, 8)

		h.MerkleizeWithMixin(indx, numItems, limit)
	}
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

	dst = append(dst, buf...)
	dst[len(dst)-1] &^= uint8(1 << msb)

	newLen := len(dst)
	for i := len(dst) - 1; i >= 0; i-- {
		if dst[i] != 0x00 {
			break
		}
		newLen = i
	}
	res := dst[:newLen]
	return res, size
}

// ParseBitlistWithHasher decodes an SSZ-encoded bitlist using the hasher's
// temporary buffer to avoid allocations. It returns the raw bit data and the
// logical bit count, same as ParseBitlist.
func ParseBitlistWithHasher(hw sszutils.HashWalker, buf []byte) ([]byte, uint64) {
	if h, ok := hw.(*Hasher); ok {
		var size uint64
		h.tmp, size = ParseBitlist(h.tmp[:0], buf)
		bitlist := h.tmp
		// Restore h.tmp to full capacity so subsequent operations (PutUint8, etc.) don't panic
		h.tmp = h.tmp[:cap(h.tmp)]
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

// PutBitlist appends a ssz bitlist
func (h *Hasher) PutBitlist(bb []byte, maxSize uint64) {
	var size uint64
	h.tmp, size = ParseBitlist(h.tmp[:0], bb)
	bitlist := h.tmp
	h.tmp = h.tmp[:cap(h.tmp)]

	// merkleize the content with mix in length
	indx := h.Index()
	h.AppendBytes32(bitlist)
	h.MerkleizeWithMixin(indx, size, (maxSize+255)/256)
}

// PutProgressiveBitlist appends an SSZ bitlist and merkleizes it using
// progressive merkleization with a length mixin.
func (h *Hasher) PutProgressiveBitlist(bb []byte) {
	var size uint64
	h.tmp, size = ParseBitlist(h.tmp[:0], bb)
	bitlist := h.tmp
	h.tmp = h.tmp[:cap(h.tmp)]

	// merkleize the content with mix in length
	indx := h.Index()
	h.AppendBytes32(bitlist)
	h.MerkleizeProgressiveWithMixin(indx, size)
}

// PutBool appends a boolean
func (h *Hasher) PutBool(b bool) {
	n := len(h.buf)
	h.buf = append(h.buf, zeroBytes[:32]...)
	if b {
		h.buf[n] = 1
	}
}

// PutBytes appends bytes
func (h *Hasher) PutBytes(b []byte) {
	if len(b) <= 32 {
		if len(b) == 32 {
			// Fast path: exactly 32 bytes, no padding needed
			h.buf = append(h.buf, b...)
			return
		}
		h.AppendBytes32(b)
		return
	}

	// if the bytes are longer than 32 we have to
	// merkleize the content
	indx := h.Index()
	h.AppendBytes32(b)
	h.Merkleize(indx)
}

// Index marks the current buffer index
func (h *Hasher) Index() int {
	return len(h.buf)
}

// Merkleize is used to merkleize the last group of the hasher
func (h *Hasher) Merkleize(indx int) {
	inputLen := len(h.buf) - indx
	if inputLen <= 128 {
		if inputLen <= 32 {
			// Fast path: single chunk, no merkleization needed
			if inputLen == 32 {
				return
			}
			// Zero-pad a partial chunk to 32 bytes and keep as single chunk
			if inputLen > 0 {
				h.buf = append(h.buf, zeroBytes[:32-inputLen]...)
				return
			}
			// Empty input: replace with zero hash (depth 0)
			h.buf = append(h.buf[:indx], zeroBytes[:32]...)
			return
		}
		if inputLen <= 64 {
			// Fast path: two chunks, single hash operation
			if inputLen < 64 {
				// Pad to 64 bytes
				h.buf = append(h.buf, zeroBytes[:64-inputLen]...)
			}
			_ = h.hash(h.buf[indx:indx+32], h.buf[indx:indx+64])
			h.buf = h.buf[:indx+32]
			return
		}
		// Fast path: 3-4 chunks, two hash operations
		if inputLen <= 96 {
			// 3 chunks: pad to 4 chunks (128 bytes)
			h.buf = append(h.buf, zeroHashes[0][:]...)
		} else if inputLen < 128 {
			// Partial 4th chunk: pad to 128 bytes
			h.buf = append(h.buf, zeroBytes[:128-inputLen]...)
		}
		// Hash 4 chunks → 2 chunks
		_ = h.hash(h.buf[indx:indx+64], h.buf[indx:indx+128])
		// Hash 2 chunks → 1 chunk
		_ = h.hash(h.buf[indx:indx+32], h.buf[indx:indx+64])
		h.buf = h.buf[:indx+32]
		return
	}
	// Fast path: exactly 8 chunks (256 bytes), three hash operations
	// Common for containers with 8 fields (e.g. Validator)
	if inputLen == 256 {
		_ = h.hash(h.buf[indx:indx+128], h.buf[indx:indx+256])
		_ = h.hash(h.buf[indx:indx+64], h.buf[indx:indx+128])
		_ = h.hash(h.buf[indx:indx+32], h.buf[indx:indx+64])
		h.buf = h.buf[:indx+32]
		return
	}
	// Fast path: exactly 16 chunks (512 bytes), four hash operations
	if inputLen == 512 {
		_ = h.hash(h.buf[indx:indx+256], h.buf[indx:indx+512])
		_ = h.hash(h.buf[indx:indx+128], h.buf[indx:indx+256])
		_ = h.hash(h.buf[indx:indx+64], h.buf[indx:indx+128])
		_ = h.hash(h.buf[indx:indx+32], h.buf[indx:indx+64])
		h.buf = h.buf[:indx+32]
		return
	}

	// merkleizeImpl will expand the `input` by 32 bytes if some hashing depth
	// hits an odd chunk length. But if we're at the end of `h.buf` already,
	// appending to `input` will allocate a new buffer, *not* expand `h.buf`,
	// so the next invocation will realloc, over and over and over. We can pre-
	// emptively cater for that by ensuring that an extra 32 bytes is always
	// available.
	if len(h.buf) == cap(h.buf) {
		h.buf = append(h.buf, zeroBytes[:32]...)
		h.buf = h.buf[:len(h.buf)-32]
	}
	input := h.buf[indx:]

	if debug {
		logfn("merkleize: %x ", input)
	}

	// merkleize the input
	input = h.merkleizeImpl(input[:0], input, 0)
	h.buf = append(h.buf[:indx], input...)

	if debug {
		logfn("-> %x\n", input)
	}
}

// MerkleizeWithMixin is used to merkleize the last group of the hasher
func (h *Hasher) MerkleizeWithMixin(indx int, num, limit uint64) {
	h.FillUpTo32()
	input := h.buf[indx:]

	// merkleize the input
	input = h.merkleizeImpl(input[:0], input, limit)

	// mixin with the size: append 32 zero bytes then write the uint64 value
	n := len(input)
	input = append(input, zeroBytes[:32]...)
	binary.LittleEndian.PutUint64(input[n:], num)

	if debug {
		logfn("merkleize-mixin: %x (%d, %d) ", input, num, limit)
	}

	// input is of the form [<input><size>] of 64 bytes
	_ = h.hash(input, input)
	h.buf = append(h.buf[:indx], input[:32]...)

	if debug {
		logfn("-> %x\n", input[:32])
	}
}

// Hash returns the last 32-byte hash from the buffer.
func (h *Hasher) Hash() []byte {
	start := 0
	if len(h.buf) > 32 {
		start = len(h.buf) - 32
	}
	return h.buf[start:]
}

// HashRoot creates the hash final hash root
func (h *Hasher) HashRoot() (res [32]byte, err error) {
	if len(h.buf) != 32 {
		err = fmt.Errorf("expected 32 byte size")
		return
	}
	copy(res[:], h.buf)
	return
}

func (h *Hasher) getDepth(d uint64) uint8 {
	if d <= 1 {
		return 0
	}
	i := sszutils.NextPowerOfTwo(d)
	return 64 - uint8(bits.LeadingZeros64(i)) - 1
}

func (h *Hasher) merkleizeImpl(dst, input []byte, limit uint64) []byte {
	// count is the number of 32 byte chunks from the input, after right-padding
	// with zeroes to the next multiple of 32 bytes when the input is not aligned
	// to a multiple of 32 bytes.
	count := uint64((len(input) + 31) / 32)
	if limit == 0 {
		limit = count
	} else if count > limit {
		panic(fmt.Sprintf("BUG: count '%d' higher than limit '%d'", count, limit))
	}

	if limit == 0 {
		return append(dst, zeroBytes[:32]...)
	}
	if limit == 1 {
		if count == 1 {
			return append(dst, input[:32]...) //nolint:gosec // G602: callers always pass 32-byte-aligned chunks; count==1 guarantees len(input)>=32
		}
		return append(dst, zeroBytes[:32]...)
	}

	depth := h.getDepth(limit)
	if len(input) == 0 {
		return append(dst, zeroHashes[depth][:]...)
	}

	for i := uint8(0); i < depth; i++ {
		layerLen := len(input) / 32
		oddNodeLength := layerLen%2 == 1

		if oddNodeLength {
			// is odd length
			input = append(input, zeroHashes[i][:]...)
			layerLen++
		}

		outputLen := (layerLen / 2) * 32

		_ = h.hash(input, input)
		input = input[:outputLen]
	}

	return append(dst, input...)
}

// MerkleizeProgressive performs progressive merkleization on the buffer
// contents from indx onwards. Progressive merkleization uses the
// subtree_fill_progressive algorithm from remerkleable, suitable for
// progressive list types.
func (h *Hasher) MerkleizeProgressive(indx int) {
	// merkleizeImpl will expand the `input` by 32 bytes if some hashing depth
	// hits an odd chunk length. But if we're at the end of `h.buf` already,
	// appending to `input` will allocate a new buffer, *not* expand `h.buf`,
	// so the next invocation will realloc, over and over and over. We can pre-
	// emptively cater for that by ensuring that an extra 32 bytes is always
	// available.
	h.buf = append(h.buf, zeroBytes...)
	h.buf = h.buf[:len(h.buf)-len(zeroBytes)]
	input := h.buf[indx:]

	if debug {
		logfn("merkleize-progressive: %x ", input)
	}

	// merkleize the input
	input = h.merkleizeProgressiveImpl(input[:0], input, 0)
	h.buf = append(h.buf[:indx], input...)

	if debug {
		logfn("-> %x\n", input)
	}
}

// MerkleizeProgressiveWithMixin is used to merkleize progressive lists with length mixin
func (h *Hasher) MerkleizeProgressiveWithMixin(indx int, num uint64) {
	h.FillUpTo32()
	input := h.buf[indx:]

	// progressive merkleize the input
	input = h.merkleizeProgressiveImpl(input[:0], input, 0)

	// mixin with the size: append 32 zero bytes then write the uint64 value
	n := len(input)
	input = append(input, zeroBytes[:32]...)
	binary.LittleEndian.PutUint64(input[n:], num)

	if debug {
		logfn("merkleize-progressive-mixin: %x (%d) ", input, num)
	}

	// input is of the form [<progressive_root><size>] of 64 bytes
	_ = h.hash(input, input)
	h.buf = append(h.buf[:indx], input[:32]...)

	if debug {
		logfn("-> %x\n", input[:32])
	}
}

// MerkleizeProgressiveWithActiveFields performs progressive merkleization on
// the buffer from indx onwards, then mixes in the active fields bitvector.
// This is used for progressive container types where only a subset of fields
// may be active.
func (h *Hasher) MerkleizeProgressiveWithActiveFields(indx int, activeFields []byte) {
	h.FillUpTo32()
	input := h.buf[indx:]

	if debug {
		logfn("merkleize-progressive-active-fields: %x ", input)
	}
	// progressive merkleize the input
	input = h.merkleizeProgressiveImpl(input[:0], input, 0)

	if debug {
		logfn("-> %x (%x)", input, activeFields)
	}

	// mixin with the active fields bitvector
	input = append(input, activeFields...)
	if rest := len(activeFields) % 32; rest != 0 {
		// pad zero bytes to the left
		input = append(input, zeroBytes[:32-rest]...)
	}

	// input is of the form [<progressive_root><active_fields>] of 64 bytes
	_ = h.hash(input, input)
	h.buf = append(h.buf[:indx], input[:32]...)

	if debug {
		logfn("-> %x\n", input[:32])
	}
}

func (h *Hasher) merkleizeProgressiveImpl(dst, chunks []byte, depth uint8) []byte {
	count := uint64((len(chunks) + 31) / 32)

	if count == 0 {
		return append(dst, zeroBytes...)
	}

	// This implements subtree_fill_progressive from remerkleable
	// def subtree_fill_progressive(nodes: PyList[Node], depth=0) -> Node:
	//     if len(nodes) == 0:
	//         return zero_node(0)
	//     base_size = 1 << depth
	//     return PairNode(
	//         subtree_fill_to_contents(nodes[:base_size], depth),
	//         subtree_fill_progressive(nodes[base_size:], depth + 2),
	//     )

	// Calculate base_size = 1 << depth (1, 4, 16, 64, 256...)
	baseSize := uint64(1) << depth

	// Split chunks: first baseSize chunks go to LEFT (binary), rest go to RIGHT (progressive)
	splitBytes := baseSize * 32
	splitPoint := len(chunks)
	if splitBytes < uint64(splitPoint) {
		splitPoint = int(splitBytes)
	}

	// Left child: subtree_fill_to_contents(nodes[:base_size], depth) - binary merkleization
	leftChunks := chunks[:splitPoint]

	// Ensure leftChunks are properly padded to 32-byte boundaries
	if len(leftChunks) > 0 && len(leftChunks)%32 != 0 {
		padNeeded := 32 - (len(leftChunks) % 32)
		leftChunks = append(leftChunks, zeroBytes[:padNeeded]...)
	}

	leftRoot := h.merkleizeImpl(leftChunks[:0], leftChunks, baseSize)

	// Right child: subtree_fill_progressive(nodes[base_size:], depth + 2) - recursive progressive
	rightChunks := chunks[splitPoint:]
	var rightRoot []byte
	if len(rightChunks) == 0 {
		rightRoot = zeroHashes[0][:]
	} else {
		// Ensure rightChunks are properly padded to 32-byte boundaries
		if len(rightChunks)%32 != 0 {
			padNeeded := 32 - (len(rightChunks) % 32)
			rightChunks = append(rightChunks, zeroBytes[:padNeeded]...)
		}

		rightRoot = h.merkleizeProgressiveImpl(rightChunks[:0], rightChunks, depth+2)
	}

	if len(h.tmp) < 64 {
		if len(h.tmp) < 32 {
			padNeeded := 32 - len(h.tmp)
			h.tmp = append(h.tmp, zeroBytes[:padNeeded]...)
		}
		padNeeded := 64 - len(h.tmp)
		h.tmp = append(h.tmp, zeroBytes[:padNeeded]...)
	}

	// PairNode(left, right) - hash(left, right)
	copy(h.tmp[:32], leftRoot)
	copy(h.tmp[32:], rightRoot)
	_ = h.hash(h.tmp[:32], h.tmp[0:64])

	return append(dst, h.tmp[:32]...)
}
