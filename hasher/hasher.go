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
	"encoding/binary"
	"fmt"
	"hash"
	"math/bits"

	"github.com/pk910/dynamic-ssz/sszutils"
)

// Compile-time check to ensure Hasher implements HashWalker interface
var _ sszutils.HashWalker = (*Hasher)(nil)

// incrementalBatchSize is the number of same-depth chunks to accumulate before
// collapsing them into the next depth level. Must be a power of 2.
// 256 chunks = 128 pairs per hash call, fits in L1 cache, good cgo amortization.
const incrementalBatchSize = 256

// maxTreeDepth is the maximum supported tree depth for incremental hashing.
// 40 levels covers trees up to 2^40 ≈ 1 trillion leaves.
const maxTreeDepth = 40

// treeLayer tracks the state of one open SSZ object scope (container, list,
// vector, etc.) during hash tree root computation.
//
// Buffer layout for a binary incremental layer (buf[bufIdx...]):
//
//	[depth-N roots] [depth-(N-1) roots] ... [depth-1 roots] [depth-0 leaves]
//	 ← oldest/deepest                               newest/shallowest →
//
// Buffer layout for a progressive incremental layer (buf[bufIdx...]):
//
//	[prog_root_0] [prog_root_1] ... [prog_root_K] [binary subtree chunks...]
//	 ← completed progressive roots →               ← active binary subtree →
//
// counts[d] tracks per-depth chunk counts for the active binary subtree.
// When any depth reaches incrementalBatchSize, chunks are hashed in-place
// to half as many at the next depth. This cascades without memory movement.
type treeLayer struct {
	bufIdx      int  // byte offset where this scope started
	incremental bool // true if opened via StartTree(), supports collapse
	collapsed   bool // true once at least one binary batch has been collapsed
	progressive bool // true if using progressive tree shape

	// Binary collapse state (active subtree)
	counts   [maxTreeDepth]uint32
	maxDepth int

	// Progressive state: completed binary subtree roots stored at the
	// left side of the buffer. Each root has depth = progressiveLevel*2.
	// Progressive base sizes: 1, 4, 16, 64, 256, 1024, ...  (1 << level*2)
	progressiveCount int // number of completed progressive roots at buf left
	progressiveLevel int // current level (determines base_size for active subtree)
}

// Hasher is a utility tool to hash SSZ structs
type Hasher struct {
	// buffer array to store hashing values
	buf []byte

	// tmp array used for uint64 and bitlist processing
	tmp []byte

	// sha256 hash function
	hash HashFn

	// layers is the stack of open SSZ object scopes. StartTree() pushes,
	// Merkleize*() pops.
	layers   []treeLayer
	layerBuf [16]treeLayer // inline backing to avoid heap allocation
}

// NewHasher creates a new Hasher object with sha256 hash

func NewHasher() *Hasher {
	return NewHasherWithHash(sha256.New())
}

func NewHasherWithHash(hh hash.Hash) *Hasher {
	return NewHasherWithHashFn(NativeHashWrapper(hh))
}

func NewHasherWithHashFn(hh HashFn) *Hasher {
	if !hasherInitialized {
		initHasher()
	}

	h := &Hasher{
		hash: hh,
		tmp:  make([]byte, 64),
	}
	h.layers = h.layerBuf[:0]
	return h
}

func (h *Hasher) WithTemp(fn func(tmp []byte) []byte) {
	h.tmp = fn(h.tmp)
}

func (h *Hasher) Reset() {
	h.buf = h.buf[:0]
	h.layers = h.layers[:0]
}

func (h *Hasher) AppendBytes32(b []byte) {
	h.buf = append(h.buf, b...)
	if rest := len(b) % 32; rest != 0 {
		// pad zero bytes to the left
		h.buf = append(h.buf, zeroBytes[:32-rest]...)
	}
}

func (h *Hasher) FillUpTo32() {
	// pad zero bytes to the left
	if rest := len(h.buf) % 32; rest != 0 {
		h.buf = append(h.buf, zeroBytes[:32-rest]...)
	}
}

func (h *Hasher) Append(i []byte) {
	h.buf = append(h.buf, i...)
}

func (h *Hasher) AppendBool(b bool) {
	if b {
		h.buf = append(h.buf, 1)
	} else {
		h.buf = append(h.buf, 0)
	}
}

func (h *Hasher) AppendUint8(i uint8) {
	h.buf = sszutils.MarshalUint8(h.buf, i)
}

func (h *Hasher) AppendUint16(i uint16) {
	h.buf = sszutils.MarshalUint16(h.buf, i)
}

func (h *Hasher) AppendUint32(i uint32) {
	h.buf = sszutils.MarshalUint32(h.buf, i)
}

func (h *Hasher) AppendUint64(i uint64) {
	h.buf = sszutils.MarshalUint64(h.buf, i)
}

func (h *Hasher) PutBool(b bool) {
	if b {
		h.buf = append(h.buf, trueBytes...)
	} else {
		h.buf = append(h.buf, falseBytes...)
	}
}

func (h *Hasher) PutUint8(i uint8) {
	h.tmp[0] = i
	h.AppendBytes32(h.tmp[:1])
}

func (h *Hasher) PutUint16(i uint16) {
	binary.LittleEndian.PutUint16(h.tmp[:2], i)
	h.AppendBytes32(h.tmp[:2])
}

func (h *Hasher) PutUint32(i uint32) {
	binary.LittleEndian.PutUint32(h.tmp[:4], i)
	h.AppendBytes32(h.tmp[:4])
}

func (h *Hasher) PutUint64(i uint64) {
	binary.LittleEndian.PutUint64(h.tmp[:8], i)
	h.AppendBytes32(h.tmp[:8])
}

func (h *Hasher) PutBytes(b []byte) {
	if len(b) <= 32 {
		h.AppendBytes32(b)
		return
	}

	// if the bytes are longer than 32 we have to
	// merkleize the content — use merkleizeImpl directly to avoid
	// interacting with the layer stack (PutBytes is an internal operation,
	// not an SSZ object scope).
	indx := len(h.buf)
	h.AppendBytes32(b)
	input := h.buf[indx:]
	input = h.merkleizeImpl(input[:0], input, 0)
	h.buf = append(h.buf[:indx], input...)
}

// internalMerkleize is like Merkleize but does NOT interact with the layer
// stack. Used by Put* methods which are internal operations, not SSZ scopes.
func (h *Hasher) internalMerkleize(indx int) {
	if len(h.buf) == cap(h.buf) {
		h.buf = append(h.buf, zeroBytes[:32]...)
		h.buf = h.buf[:len(h.buf)-32]
	}
	input := h.buf[indx:]
	input = h.merkleizeImpl(input[:0], input, 0)
	h.buf = append(h.buf[:indx], input...)
}

// internalMerkleizeWithMixin is like MerkleizeWithMixin but does NOT interact
// with the layer stack.
func (h *Hasher) internalMerkleizeWithMixin(indx int, num, limit uint64) {
	h.FillUpTo32()
	input := h.buf[indx:]
	input = h.merkleizeImpl(input[:0], input, limit)

	n := len(input)
	input = append(input, zeroBytes[:32]...)
	binary.LittleEndian.PutUint64(input[n:], num)

	_ = h.hash(input, input)
	h.buf = append(h.buf[:indx], input[:32]...)
}

func (h *Hasher) PutRootVector(b [][]byte, maxCapacity ...uint64) error {
	indx := len(h.buf)
	for _, i := range b {
		if len(i) != 32 {
			return fmt.Errorf("bad root")
		}
		h.buf = append(h.buf, i...)
	}

	if len(maxCapacity) == 0 {
		h.internalMerkleize(indx)
	} else {
		numItems := uint64(len(b))
		limit := sszutils.CalculateLimit(maxCapacity[0], numItems, 32)
		h.internalMerkleizeWithMixin(indx, numItems, limit)
	}
	return nil
}

func (h *Hasher) PutUint64Array(b []uint64, maxCapacity ...uint64) {
	indx := len(h.buf)
	for _, i := range b {
		h.AppendUint64(i)
	}

	h.FillUpTo32()

	if len(maxCapacity) == 0 {
		h.internalMerkleize(indx)
	} else {
		numItems := uint64(len(b))
		limit := sszutils.CalculateLimit(maxCapacity[0], numItems, 8)
		h.internalMerkleizeWithMixin(indx, numItems, limit)
	}
}

func (h *Hasher) PutBitlist(bb []byte, maxSize uint64) {
	var size uint64
	h.tmp, size = ParseBitlist(h.tmp[:0], bb)
	bitlist := h.tmp
	h.tmp = h.tmp[:cap(h.tmp)]

	indx := len(h.buf)
	h.AppendBytes32(bitlist)
	h.internalMerkleizeWithMixin(indx, size, (maxSize+255)/256)
}

func (h *Hasher) PutProgressiveBitlist(bb []byte) {
	var size uint64
	h.tmp, size = ParseBitlist(h.tmp[:0], bb)
	bitlist := h.tmp
	h.tmp = h.tmp[:cap(h.tmp)]

	// merkleize the content with mix in length using progressive algorithm
	indx := len(h.buf)
	h.AppendBytes32(bitlist)
	h.FillUpTo32()
	input := h.buf[indx:]
	input = h.merkleizeProgressiveImpl(input[:0], input, 0)

	n := len(input)
	input = append(input, zeroBytes[:32]...)
	binary.LittleEndian.PutUint64(input[n:], size)
	_ = h.hash(input, input)
	h.buf = append(h.buf[:indx], input[:32]...)
}

// StartTree opens a new SSZ object scope and returns the buffer index.
// TreeTypeBinary/Progressive: pushes an incremental layer (supports Collapse).
// TreeTypeNone: returns position only, no layer pushed (testing/debug).
func (h *Hasher) StartTree(treeType sszutils.TreeType) int {
	idx := len(h.buf)
	if treeType == sszutils.TreeTypeNone {
		return idx
	}
	h.layers = append(h.layers, treeLayer{
		bufIdx:      idx,
		incremental: true,
		progressive: treeType == sszutils.TreeTypeProgressive,
	})
	return idx
}

// Index returns the current buffer position and pushes a non-incremental
// layer. This is for legacy/external code that doesn't use StartTree.
// The non-incremental layer blocks Collapse on this scope but is properly
// popped by Merkleize. Collapse on the parent layer is unaffected — it
// only sees completed child roots after the child's Merkleize pops this layer.
func (h *Hasher) Index() int {
	idx := len(h.buf)
	h.layers = append(h.layers, treeLayer{
		bufIdx:      idx,
		incremental: false,
	})
	return idx
}

func (h *Hasher) Collapse() {
	h.maybeCollapseIncremental()
}

func (h *Hasher) topLayer() *treeLayer {
	if len(h.layers) == 0 {
		return nil
	}
	return &h.layers[len(h.layers)-1]
}

func (h *Hasher) popMatchingLayer(indx int) *treeLayer {
	if n := len(h.layers); n > 0 && h.layers[n-1].bufIdx == indx {
		return &h.layers[n-1]
	}
	return nil
}

func (h *Hasher) popTopLayer() {
	if len(h.layers) > 0 {
		h.layers = h.layers[:len(h.layers)-1]
	}
}

func (h *Hasher) maybeCollapseIncremental() {
	layer := h.topLayer()
	if layer == nil || !layer.incremental {
		return
	}

	if layer.progressive {
		h.maybeCollapseProgressive(layer)
	} else {
		h.maybeCollapseBinary(layer)
	}
}

func (h *Hasher) maybeCollapseBinary(layer *treeLayer) {
	regionStart := h.binaryRegionStart(layer)
	totalChunks := (len(h.buf) - regionStart) / 32
	if totalChunks < incrementalBatchSize {
		return
	}

	if !layer.collapsed {
		layer.collapsed = true
		layer.counts[0] = uint32(totalChunks)
	} else {
		h.syncCollapseState(layer)
	}

	for d := 0; d < maxTreeDepth-1; d++ {
		if layer.counts[d] < incrementalBatchSize {
			break
		}

		count := int(layer.counts[d])
		batchCount := (count / incrementalBatchSize) * incrementalBatchSize
		batchBytes := batchCount * 32
		produced := batchCount / 2

		// Find start of depth-d group (after all higher-depth groups)
		dStart := h.binaryRegionStart(layer)
		for dd := layer.maxDepth; dd > d; dd-- {
			dStart += int(layer.counts[dd]) * 32
		}

		// Hash leftmost batchCount entries in-place
		_ = h.hash(h.buf[dStart:dStart+batchBytes/2], h.buf[dStart:dStart+batchBytes])

		// Shift tail (remainder of depth-d + all lower depths) left
		afterBatch := dStart + batchBytes
		afterProduced := dStart + produced*32
		tailLen := len(h.buf) - afterBatch
		if tailLen > 0 && afterProduced != afterBatch {
			copy(h.buf[afterProduced:], h.buf[afterBatch:afterBatch+tailLen])
		}
		h.buf = h.buf[:afterProduced+tailLen]

		layer.counts[d] -= uint32(batchCount)
		layer.counts[d+1] += uint32(produced)
		if d+1 > layer.maxDepth {
			layer.maxDepth = d + 1
		}
	}
}

func (h *Hasher) maybeCollapseProgressive(layer *treeLayer) {
	// Sync collapse state so counts reflect all buffer data
	if layer.collapsed {
		h.syncCollapseState(layer)
	} else {
		// Not yet collapsed — compute d0 count from buffer
		regionStart := h.binaryRegionStart(layer)
		chunks := (len(h.buf) - regionStart) / 32
		if chunks > 0 {
			layer.counts[0] = uint32(chunks)
		}
	}

	readPos := h.activeSubtreeStart(layer)
	writePos := readPos
	finalized := false

	// Step 1: finalize complete progressive groups
	for {
		// Compute leaf count from current counts
		var leafCount uint64
		for d := 0; d <= layer.maxDepth; d++ {
			leafCount += uint64(layer.counts[d]) << uint(d)
		}

		baseSize := progressiveBaseSize(layer.progressiveLevel)
		if leafCount < baseSize {
			break
		}
		finalized = true

		// Consume exactly baseSize leaves from buf[readPos:] left-to-right
		// (greedy from highest depth). Track consumed counts for collapseAllDepths.
		consumePos := readPos
		consumed := uint64(0)
		var consumedCounts [maxTreeDepth]uint32
		consumedMaxDepth := 0
		for d := layer.maxDepth; d >= 0; d-- {
			for layer.counts[d] > 0 && consumed+uint64(1<<uint(d)) <= baseSize {
				consumed += uint64(1 << uint(d))
				layer.counts[d]--
				consumedCounts[d]++
				if d > consumedMaxDepth {
					consumedMaxDepth = d
				}
				consumePos += 32
			}
			if consumed == baseSize {
				break
			}
		}

		if consumed == 0 {
			break // safety
		}

		// Merkleize the consumed entries to a single root.
		// Merkleize the consumed entries to a root using collapseAllDepths.
		// It works within buf[readPos:consumePos] only (bufEnd parameter),
		// leaving the unconsumed tail untouched. Root lands at buf[readPos].
		tmpLayer := treeLayer{
			bufIdx:    readPos,
			collapsed: consumedMaxDepth > 0,
			counts:    consumedCounts,
			maxDepth:  consumedMaxDepth,
		}
		h.collapseAllDepths(&tmpLayer, readPos, consumePos, baseSize)
		if writePos != readPos {
			copy(h.buf[writePos:writePos+32], h.buf[readPos:readPos+32])
		}
		writePos += 32
		readPos = consumePos

		layer.progressiveCount++
		layer.progressiveLevel++

		// Update maxDepth (some depths may now be empty)
		for layer.maxDepth > 0 && layer.counts[layer.maxDepth] == 0 {
			layer.maxDepth--
		}
	}

	if !finalized {
		// No groups finalized — try binary collapse directly
		h.maybeCollapseBinary(layer)
		return
	}

	// Step 2: compact remainder from readPos to writePos with depth-aware hash-copy
	// Remainder is buf[readPos:len(h.buf)] with layout [high-depth...low-depth]
	var newCounts [maxTreeDepth]uint32
	newMaxDepth := 0

	for d := layer.maxDepth; d >= 0; d-- {
		n := int(layer.counts[d])
		if n == 0 {
			continue
		}

		pairs := n / 2
		odd := n % 2

		if pairs > 0 {
			_ = h.hash(h.buf[writePos:writePos+pairs*32], h.buf[readPos:readPos+pairs*2*32])
			writePos += pairs * 32
			readPos += pairs * 2 * 32
			newCounts[d+1] += uint32(pairs)
			if d+1 > newMaxDepth {
				newMaxDepth = d + 1
			}
		}

		if odd == 1 {
			copy(h.buf[writePos:writePos+32], h.buf[readPos:readPos+32])
			writePos += 32
			readPos += 32
			newCounts[d] = 1
			if d > newMaxDepth {
				newMaxDepth = d
			}
		}
	}

	h.buf = h.buf[:writePos]
	layer.counts = newCounts
	layer.maxDepth = newMaxDepth
	layer.collapsed = true // compacted data has mixed depths

	// Step 3: binary collapse on the compacted remainder
	h.maybeCollapseBinary(layer)
}

func progressiveBaseSize(level int) uint64 {
	return 1 << (uint(level) * 2)
}

func (h *Hasher) activeSubtreeStart(layer *treeLayer) int {
	return layer.bufIdx + layer.progressiveCount*32
}

func (h *Hasher) binaryRegionStart(layer *treeLayer) int {
	if layer.progressive {
		return h.activeSubtreeStart(layer)
	}
	return layer.bufIdx
}

func (h *Hasher) syncCollapseState(layer *treeLayer) {
	h.syncCollapseStateWithEnd(layer, len(h.buf))
}

func (h *Hasher) syncCollapseStateWithEnd(layer *treeLayer, bufEnd int) {
	totalChunks := (bufEnd - h.binaryRegionStart(layer)) / 32
	var accounted int
	for d := 0; d <= layer.maxDepth; d++ {
		accounted += int(layer.counts[d])
	}
	if diff := totalChunks - accounted; diff > 0 {
		layer.counts[0] += uint32(diff)
	}
}

func (h *Hasher) collapseAllDepths(layer *treeLayer, indx int, bufEnd int, limit uint64) {
	h.syncCollapseStateWithEnd(layer, bufEnd)

	for {
		lowestDepth := -1
		for d := 0; d <= layer.maxDepth; d++ {
			if layer.counts[d] > 0 {
				lowestDepth = d
				break
			}
		}
		if lowestDepth < 0 {
			break
		}

		count := int(layer.counts[lowestDepth])

		if count == 1 {
			done := true
			for d := lowestDepth + 1; d <= layer.maxDepth; d++ {
				if layer.counts[d] > 0 {
					done = false
					break
				}
			}
			if done {
				break
			}
		}

		if count%2 == 1 {
			// Need space for zero hash pad. Use the buffer at bufEnd
			// (which is within our working region or just past it).
			copy(h.buf[bufEnd:bufEnd+32], zeroHashes[lowestDepth][:])
			count++
			bufEnd += 32
		}

		chunkBytes := count * 32
		batchStart := bufEnd - chunkBytes
		_ = h.hash(h.buf[batchStart:batchStart+chunkBytes/2], h.buf[batchStart:batchStart+chunkBytes])
		bufEnd = batchStart + chunkBytes/2

		layer.counts[lowestDepth] = 0
		layer.counts[lowestDepth+1] += uint32(count / 2)
		if lowestDepth+1 > layer.maxDepth {
			layer.maxDepth = lowestDepth + 1
		}
	}

	// Expand to target depth if a limit is specified
	if limit > 0 {
		targetDepth := h.getDepth(limit)
		currentDepth := uint8(0)
		for d := 0; d <= layer.maxDepth; d++ {
			if layer.counts[d] == 1 {
				currentDepth = uint8(d)
				break
			}
		}
		// Expand using 64-byte workspace at bufEnd (safe — caller ensures space)
		pos := bufEnd - 32
		for currentDepth < targetDepth {
			copy(h.buf[pos+32:pos+64], zeroHashes[currentDepth][:])
			_ = h.hash(h.buf[pos:pos+32], h.buf[pos:pos+64])
			currentDepth++
		}
		bufEnd = pos + 32
	}

	// Move final root to indx (within the working region, no tail corruption)
	if bufEnd-32 != indx {
		copy(h.buf[indx:indx+32], h.buf[bufEnd-32:bufEnd])
	}
	// Don't truncate h.buf — caller manages buffer length
}

func (h *Hasher) collapseProgressiveLayer(layer *treeLayer, indx int) {
	// 1. Finalize all complete progressive groups and compact remainder.
	h.maybeCollapseProgressive(layer)

	// 2. Handle the partial remainder (the last unfilled group).
	subtreeStart := h.activeSubtreeStart(layer)
	activeChunks := (len(h.buf) - subtreeStart) / 32

	if activeChunks > 0 {
		baseSize := progressiveBaseSize(layer.progressiveLevel)

		// Sync collapse state if not yet tracked
		if !layer.collapsed {
			layer.counts[0] = uint32(activeChunks)
		} else {
			h.syncCollapseState(layer)
		}
		h.collapseAllDepths(layer, subtreeStart, len(h.buf), baseSize)
		h.buf = h.buf[:subtreeStart+32]
		layer.progressiveCount++
	}

	// 3. Fold progressive roots right-to-left.
	nRoots := layer.progressiveCount
	if nRoots == 0 {
		h.buf = h.buf[:indx]
		h.buf = append(h.buf, zeroBytes[:32]...)
		return
	}

	// accumulator starts as zero_node(0)
	copy(h.tmp[32:64], zeroHashes[0][:])
	for i := nRoots - 1; i >= 0; i-- {
		rootPos := indx + i*32
		copy(h.tmp[:32], h.buf[rootPos:rootPos+32])
		_ = h.hash(h.tmp[:32], h.tmp[:64])
		copy(h.tmp[32:64], h.tmp[:32])
	}

	h.buf = h.buf[:indx]
	h.buf = append(h.buf, h.tmp[:32]...)
}

func (h *Hasher) Merkleize(indx int) {
	layer := h.popMatchingLayer(indx)

	if layer != nil && layer.collapsed {
		h.collapseAllDepths(layer, indx, len(h.buf), 0)
		h.buf = h.buf[:indx+32]
		h.popTopLayer()
		return
	}
	if layer != nil {
		h.popTopLayer()
	}

	// Standard path
	if len(h.buf) == cap(h.buf) {
		h.buf = append(h.buf, zeroBytes[:32]...)
		h.buf = h.buf[:len(h.buf)-32]
	}
	input := h.buf[indx:]

	if debug {
		logfn("merkleize: %x ", input)
	}

	input = h.merkleizeImpl(input[:0], input, 0)
	h.buf = append(h.buf[:indx], input...)

	if debug {
		logfn("-> %x\n", input)
	}
}

func (h *Hasher) MerkleizeWithMixin(indx int, num, limit uint64) {
	h.FillUpTo32()

	layer := h.popMatchingLayer(indx)

	if layer != nil && layer.collapsed {
		h.collapseAllDepths(layer, indx, len(h.buf), limit)
		h.buf = h.buf[:indx+32]
		h.popTopLayer()
	} else {
		if layer != nil {
			h.popTopLayer()
		}
		// Standard merkleize
		input := h.buf[indx:]
		input = h.merkleizeImpl(input[:0], input, limit)
		h.buf = append(h.buf[:indx], input...)
	}

	// Mixin with the size
	input := h.buf[indx : indx+32]
	n := len(input)
	input = append(input, zeroBytes[:32]...)
	binary.LittleEndian.PutUint64(input[n:], num)

	if debug {
		logfn("merkleize-mixin: %x (%d, %d) ", input, num, limit)
	}

	_ = h.hash(input, input)
	h.buf = append(h.buf[:indx], input[:32]...)

	if debug {
		logfn("-> %x\n", input[:32])
	}
}

func (h *Hasher) MerkleizeProgressive(indx int) {
	layer := h.popMatchingLayer(indx)

	if layer != nil && layer.progressive {
		h.collapseProgressiveLayer(layer, indx)
		h.popTopLayer()
		return
	}
	if layer != nil {
		h.popTopLayer()
	}

	// Standard path (no incremental progressive data)
	h.buf = append(h.buf, zeroBytes...)
	h.buf = h.buf[:len(h.buf)-len(zeroBytes)]
	input := h.buf[indx:]

	if debug {
		logfn("merkleize-progressive: %x ", input)
	}

	input = h.merkleizeProgressiveImpl(input[:0], input, 0)
	h.buf = append(h.buf[:indx], input...)

	if debug {
		logfn("-> %x\n", input)
	}
}

func (h *Hasher) MerkleizeProgressiveWithMixin(indx int, num uint64) {
	layer := h.popMatchingLayer(indx)

	if layer != nil && layer.progressive && layer.progressiveCount > 0 {
		h.FillUpTo32()
		h.collapseProgressiveLayer(layer, indx)
		h.popTopLayer()
	} else {
		if layer != nil {
			h.popTopLayer()
		}
		h.FillUpTo32()
		input := h.buf[indx:]
		input = h.merkleizeProgressiveImpl(input[:0], input, 0)
		h.buf = append(h.buf[:indx], input...)
	}

	// Now buf[indx:indx+32] has the progressive root. Mixin with size.
	input := h.buf[indx : indx+32]

	// mixin with the size (same as MerkleizeWithMixin)
	output := h.tmp[:0]
	output = sszutils.MarshalUint64(output, num)
	input = append(input, output...)
	input = append(input, zeroBytes[:24]...)

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

func (h *Hasher) MerkleizeProgressiveWithActiveFields(indx int, activeFields []byte) {
	layer := h.popMatchingLayer(indx)

	if layer != nil && layer.progressive && layer.progressiveCount > 0 {
		h.FillUpTo32()
		h.collapseProgressiveLayer(layer, indx)
		h.popTopLayer()
	} else {
		if layer != nil {
			h.popTopLayer()
		}
		h.FillUpTo32()
		input := h.buf[indx:]
		if debug {
			logfn("merkleize-progressive-active-fields: %x ", input)
		}
		input = h.merkleizeProgressiveImpl(input[:0], input, 0)
		h.buf = append(h.buf[:indx], input...)
	}

	// Now buf[indx:indx+32] has the progressive root.
	input := h.buf[indx : indx+32]

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

func (h *Hasher) Hash() []byte {
	start := 0
	if len(h.buf) > 32 {
		start = len(h.buf) - 32
	}
	return h.buf[start:]
}

func (h *Hasher) HashRoot() (res [32]byte, err error) {
	if len(h.buf) != 32 {
		err = fmt.Errorf("expected 32 byte size")
		return
	}
	copy(res[:], h.buf)
	return
}
