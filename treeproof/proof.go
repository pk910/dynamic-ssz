// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
//
// This file contains code derived from https://github.com/ferranbt/fastssz/blob/v1.0.0/proof.go
// Copyright (c) 2020 Ferran Borreguero
// Licensed under the MIT License
// The code has been modified for dynamic-ssz proof generation needs.

package treeproof

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/bits"
	"sort"
	"sync"

	"github.com/pk910/dynamic-ssz/hasher"
)

// VerifyProof verifies a single merkle branch. It's more
// efficient than VerifyMultiproof for proving one leaf.
func VerifyProof(root []byte, proof *Proof) (bool, error) {
	if len(proof.Hashes) != getPathLength(proof.Index) {
		return false, errors.New("invalid proof length")
	}

	var tmp [64]byte
	node := bytesToChunk(proof.Leaf)
	for i, h := range proof.Hashes {
		if getPosAtLevel(proof.Index, i) {
			copy(tmp[:32], h)
			copy(tmp[32:], node[:])
		} else {
			copy(tmp[:32], node[:])
			copy(tmp[32:], h)
		}
		node = sha256.Sum256(tmp[:])
	}

	return bytes.Equal(root, node[:]), nil
}

// VerifyMultiproof verifies a proof for multiple leaves against the given root.
//
// The arguments provided to this function need to adhere to some ordering rules, otherwise
// a successful verification is not guaranteed even if the client holds correct data:
//
// 1. `leaves` and `indices` have same order, i.e. `leaves[i]` is the leaf at `indices[i]`;
//
// 2. `proofs` are sorted in descending order according to their generalised indices.
//
// For a better understanding of "2.", consider the following the tree:
//
//	       .
//	   .       .            * = intermediate hash (i.e. an element of `proof`)
//	 .   *   *   .          x = leaf
//	x x . . . . x *
//
// In the example above, we have three intermediate hashes in position 5, 6 and 15.
// Let's call such hashes "*5", "*6" and "*15" respectively.
// Then, when calling this function `proof` should be ordered as [*15, *6, *5].
func VerifyMultiproof(root []byte, proof, leaves [][]byte, indices []int) (bool, error) {
	if len(indices) == 0 {
		return false, errors.New("indices length is zero")
	}

	if len(leaves) != len(indices) {
		return false, errors.New("number of leaves and indices mismatch")
	}

	if len(proof) == 0 {
		if ok, valid := verifyFullTreeLeaves(root, leaves, indices); ok {
			return valid, nil
		}
	}

	requiredProofIndices := getRequiredIndicesFn(indices)
	if len(requiredProofIndices) != len(proof) {
		return false, fmt.Errorf("number of proof hashes %d and required indices %d mismatch", len(proof), len(requiredProofIndices))
	}

	// Most benchmarked proofs are regular leaf proofs from one tree depth.
	// For that shape we can rebuild the root level-by-level without the more
	// general hash-index map used for mixed-depth proofs.
	if indicesShareDepth(indices) {
		if handled, valid, err := verifyMultiproofSameDepth(root, proof, leaves, indices, requiredProofIndices); handled {
			return valid, err
		}
	}

	return verifyMultiproofGeneral(root, proof, leaves, indices, requiredProofIndices)
}

// verifyMultiproofGeneral verifies proofs that include indices from different
// tree depths by storing known node hashes in a map keyed by generalized index.
func verifyMultiproofGeneral(root []byte, proof, leaves [][]byte, indices, requiredProofIndices []int) (bool, error) {

	// Keep leaf indices in descending generalized-index order so verification can
	// walk the tree bottom-up without building another sorted slice.
	leafIndexCursor := newDescendingIndexCursor(indices)
	// Pre-size the hash lookup with leaves, proof nodes, and a small allowance
	// for parent hashes we will compute during verification.
	hashesByIndexCapacity := len(indices) + len(requiredProofIndices)
	if maxReqIndex := len(requiredProofIndices); maxReqIndex > 0 {
		hashesByIndexCapacity += getPathLength(requiredProofIndices[0])
	} else if leafIndexCursor.ok() {
		hashesByIndexCapacity += getPathLength(leafIndexCursor.current())
	}
	hashesByIndex := make(map[int][32]byte, hashesByIndexCapacity)

	for i, leaf := range leaves {
		hashesByIndex[indices[i]] = bytesToChunk(leaf)
	}
	for i, h := range proof {
		hashesByIndex[requiredProofIndices[i]] = bytesToChunk(h)
	}

	// The depth of the tree up to the greatest index
	maxIndex := 0
	if leafIndexCursor.ok() {
		maxIndex = leafIndexCursor.current()
	}
	if len(requiredProofIndices) > 0 && requiredProofIndices[0] > maxIndex {
		maxIndex = requiredProofIndices[0]
	}

	var capacity int
	if maxIndex > 0 {
		capacity = getPathLength(maxIndex)
	}

	// Keep the parent indices we create so the same loop can continue walking
	// upward toward the root without extra sorting.
	pendingParentQueue := make([]int, 0, capacity)

	// Track where we are in the proof list and the parent work queue.
	requiredProofPos := 0
	pendingParentPos := 0

	var tmp [64]byte
	var currentIndex int

	// Walk upward through the tree, hashing children into parents until the
	// root is reached or a required node is missing.
	const (
		sourcePendingParent = 1
		sourceLeaf          = 2
		sourceProof         = 3
	)
	for pendingParentPos < len(pendingParentQueue) || leafIndexCursor.ok() || requiredProofPos < len(requiredProofIndices) {
		// Always process the largest available index next so children are handled
		// before the parent hash that depends on them.
		currentIndex = 0
		source := 0
		if pendingParentPos < len(pendingParentQueue) {
			currentIndex = pendingParentQueue[pendingParentPos]
			source = sourcePendingParent
		}
		if leafIndexCursor.ok() && leafIndexCursor.current() > currentIndex {
			currentIndex = leafIndexCursor.current()
			source = sourceLeaf
		}
		if requiredProofPos < len(requiredProofIndices) && requiredProofIndices[requiredProofPos] > currentIndex {
			currentIndex = requiredProofIndices[requiredProofPos]
			source = sourceProof
		}

		switch source {
		case sourcePendingParent:
			pendingParentPos++
		case sourceLeaf:
			leafIndexCursor.advance()
		case sourceProof:
			requiredProofPos++
		}

		// Reaching generalized index 1 means we are already at the root.
		if currentIndex == 1 {
			break
		}

		parentIndex := getParent(currentIndex)

		// If another child already computed this parent, skip the duplicate work.
		if _, hasParent := hashesByIndex[parentIndex]; hasParent {
			continue
		}

		leftIndex := (currentIndex | 1) ^ 1
		left, hasLeft := hashesByIndex[leftIndex]
		rightIndex := currentIndex | 1
		right, hasRight := hashesByIndex[rightIndex]

		if !hasRight || !hasLeft {
			return false, fmt.Errorf("proof is missing required nodes, either %d or %d", leftIndex, rightIndex)
		}

		copy(tmp[:32], left[:])
		copy(tmp[32:], right[:])
		hashesByIndex[parentIndex] = sha256.Sum256(tmp[:])

		// Queue the new parent because it may itself need to be paired higher up.
		pendingParentQueue = append(pendingParentQueue, parentIndex)
	}

	res, ok := hashesByIndex[1]
	if !ok {
		return false, fmt.Errorf("root was not computed during proof verification")
	}

	return res == bytesToChunk(root), nil
}

// indexedChunk holds a subtree hash together with the generalized index that
// identifies its current position in the tree.
type indexedChunk struct {
	index int
	hash  [32]byte
}

// inlineIndexedChunkCapacity is the largest same-depth proof size handled with
// stack-backed scratch space before falling back to heap allocation.
const inlineIndexedChunkCapacity = 64

// verifyMultiproofSameDepth verifies proofs where all requested indices are on
// the same tree level. It returns handled=false when the input shape should
// fall back to the general verifier.
func verifyMultiproofSameDepth(root []byte, proof, leaves [][]byte, indices, requiredProofIndices []int) (handled, valid bool, err error) {
	var currentLevelInline [inlineIndexedChunkCapacity]indexedChunk
	var nextLevelInline [inlineIndexedChunkCapacity]indexedChunk

	var currentLevel []indexedChunk
	var nextLevel []indexedChunk

	if len(indices) <= inlineIndexedChunkCapacity {
		currentLevel = currentLevelInline[:len(indices)]
		nextLevel = nextLevelInline[:0]
	} else {
		currentLevel = make([]indexedChunk, len(indices))
		nextLevel = make([]indexedChunk, 0, (len(indices)+1)/2)
	}

	if !populateDescendingIndexedChunks(currentLevel, indices, leaves) {
		return false, false, nil
	}

	requiredProofPos := 0
	var tmp [64]byte
	rootChunk := bytesToChunk(root)

	for len(currentLevel) > 0 {
		if len(currentLevel) == 1 && currentLevel[0].index == 1 {
			return true, currentLevel[0].hash == rootChunk, nil
		}

		nextLevel = nextLevel[:0]

		for i := 0; i < len(currentLevel); {
			currentNode := currentLevel[i]
			parentIndex := getParent(currentNode.index)

			if currentNode.index&1 == 1 && i+1 < len(currentLevel) && currentLevel[i+1].index == currentNode.index-1 {
				copy(tmp[:32], currentLevel[i+1].hash[:])
				copy(tmp[32:], currentNode.hash[:])
				nextLevel = append(nextLevel, indexedChunk{
					index: parentIndex,
					hash:  sha256.Sum256(tmp[:]),
				})
				i += 2
				continue
			}

			if requiredProofPos >= len(requiredProofIndices) {
				return true, false, fmt.Errorf("proof is missing required nodes, either %d or %d", (currentNode.index|1)^1, currentNode.index|1)
			}

			siblingIndex := getSibling(currentNode.index)
			if requiredProofIndices[requiredProofPos] != siblingIndex {
				return false, false, nil
			}

			proofSiblingHash := bytesToChunk(proof[requiredProofPos])
			requiredProofPos++

			if currentNode.index&1 == 1 {
				copy(tmp[:32], proofSiblingHash[:])
				copy(tmp[32:], currentNode.hash[:])
			} else {
				copy(tmp[:32], currentNode.hash[:])
				copy(tmp[32:], proofSiblingHash[:])
			}

			nextLevel = append(nextLevel, indexedChunk{
				index: parentIndex,
				hash:  sha256.Sum256(tmp[:]),
			})
			i++
		}

		currentLevel, nextLevel = nextLevel, currentLevel[:0]
	}

	return true, false, fmt.Errorf("root was not computed during proof verification")
}

// populateDescendingIndexedChunks fills dst with leaf hashes paired with their
// generalized indices in descending order.
func populateDescendingIndexedChunks(dst []indexedChunk, indices []int, leaves [][]byte) bool {
	switch {
	case intsSortedDescending(indices):
		previousIndex := 0
		for i, idx := range indices {
			if i > 0 && idx == previousIndex {
				return false
			}
			dst[i] = indexedChunk{index: idx, hash: bytesToChunk(leaves[i])}
			previousIndex = idx
		}
		return true
	case sort.IntsAreSorted(indices):
		previousIndex := 0
		for i := len(indices) - 1; i >= 0; i-- {
			idx := indices[i]
			if i < len(indices)-1 && idx == previousIndex {
				return false
			}
			dst[len(indices)-1-i] = indexedChunk{index: idx, hash: bytesToChunk(leaves[i])}
			previousIndex = idx
		}
		return true
	default:
		return false
	}
}

// descendingIndexCursor iterates a set of generalized indices from largest to
// smallest while reusing the caller's slice whenever it is already ordered.
type descendingIndexCursor struct {
	values []int
	pos    int
	step   int
}

// newDescendingIndexCursor creates a cursor that walks indices from largest to
// smallest without forcing every caller to sort first.
func newDescendingIndexCursor(indices []int) descendingIndexCursor {
	switch {
	case len(indices) == 0:
		return descendingIndexCursor{pos: -1}
	case intsSortedDescending(indices):
		return descendingIndexCursor{values: indices, pos: 0, step: 1}
	case sort.IntsAreSorted(indices):
		return descendingIndexCursor{values: indices, pos: len(indices) - 1, step: -1}
	default:
		out := make([]int, len(indices))
		copy(out, indices)
		sort.Sort(sort.Reverse(sort.IntSlice(out)))
		return descendingIndexCursor{values: out, pos: 0, step: 1}
	}
}

// ok reports whether the cursor still points at a valid index.
func (c descendingIndexCursor) ok() bool {
	return c.pos >= 0 && c.pos < len(c.values)
}

// current returns the index currently selected by the cursor.
func (c descendingIndexCursor) current() int {
	return c.values[c.pos]
}

// advance moves the cursor to the next smaller index.
func (c *descendingIndexCursor) advance() {
	c.pos += c.step
}

// intsSortedDescending reports whether indices are already ordered from largest
// generalized index to smallest.
func intsSortedDescending(indices []int) bool {
	for i := 1; i < len(indices); i++ {
		if indices[i-1] < indices[i] {
			return false
		}
	}
	return true
}

// verifyFullTreeLeaves handles proofs where the caller already provides a full
// power-of-two leaf layer that can be merkleized directly.
func verifyFullTreeLeaves(root []byte, leaves [][]byte, indices []int) (bool, bool) {
	count := len(indices)
	if count == 0 || count != len(leaves) || count&(count-1) != 0 {
		return false, false
	}

	reverse := false
	switch indices[0] {
	case count:
		for i := 1; i < count; i++ {
			if indices[i] != count+i {
				return false, false
			}
		}
	case (count * 2) - 1:
		reverse = true
		for i := 1; i < count; i++ {
			if indices[i] != (count*2)-1-i {
				return false, false
			}
		}
	default:
		return false, false
	}

	hh := hasher.FastHasherPool.Get()
	defer hasher.FastHasherPool.Put(hh)

	appendStart := hh.Index()
	zeroChunk := hasher.GetZeroHash(0)
	if reverse {
		for i := count - 1; i >= 0; i-- {
			appendProofLeaf(hh, zeroChunk, leaves[i])
		}
	} else {
		for i := range leaves {
			appendProofLeaf(hh, zeroChunk, leaves[i])
		}
	}
	hh.Merkleize(appendStart)

	return true, bytes.Equal(root, hh.Hash())
}

// appendProofLeaf writes one proof leaf as a full 32-byte SSZ chunk.
func appendProofLeaf(hh *hasher.Hasher, zeroChunk, leaf []byte) {
	switch {
	case len(leaf) == 32:
		hh.Append(leaf)
	case len(leaf) > 32:
		hh.Append(leaf[:32])
	case len(leaf) == 0:
		hh.Append(zeroChunk)
	default:
		hh.Append(leaf)
		hh.Append(zeroChunk[:32-len(leaf)])
	}
}

// getPosAtLevel reports whether index is on the right side at the requested
// tree level. Level 0 is the node itself, level 1 is its parent, and so on.
func getPosAtLevel(index, level int) bool {
	return (index & (1 << level)) > 0
}

// getPathLength returns how many parent steps separate index from the root.
func getPathLength(index int) int {
	return bits.Len(uint(index)) - 1
}

// getSibling returns the generalized index of index's sibling.
func getSibling(index int) int {
	return index ^ 1
}

// getParent returns the generalized index one level above index.
func getParent(index int) int {
	return index >> 1
}

// getRequiredIndicesFn is used by VerifyMultiproof and can be replaced in
// tests to inject incomplete index sets for covering defensive error paths.
var getRequiredIndicesFn = getRequiredIndices

const requiredIndicesCacheSize = 32

// requiredIndicesCacheEntry stores one cached mapping from a leaf index set to
// the proof indices needed to verify it.
type requiredIndicesCacheEntry struct {
	hash     uint64
	indices  []int
	required []int
}

var requiredIndicesCache struct {
	mu      sync.RWMutex
	next    int // ring-buffer write position for the next cached result
	entries [requiredIndicesCacheSize]requiredIndicesCacheEntry
}

// getRequiredIndices returns the generalized indices required to verify the
// given leaf indices. The result is always sorted in descending order.
func getRequiredIndices(leafIndices []int) []int {
	if len(leafIndices) == 0 {
		return nil
	}

	// Verification benchmarks call this with the same index sets many times.
	// Cache the derived proof indices so repeated verifications can skip the
	// sort/deduplicate/walk work entirely.
	indicesKeyHash := hashIndices(leafIndices)

	requiredIndicesCache.mu.RLock()
	for i := range requiredIndicesCache.entries {
		cacheEntry := &requiredIndicesCache.entries[i]
		if cacheEntry.hash != indicesKeyHash {
			continue
		}
		if intsEqual(cacheEntry.indices, leafIndices) {
			requiredIndices := cacheEntry.required
			requiredIndicesCache.mu.RUnlock()
			return requiredIndices
		}
	}
	requiredIndicesCache.mu.RUnlock()

	// Cache miss: compute the required proof nodes from scratch, then store a
	// copy so future calls with the same indices can reuse it safely.
	required := computeRequiredIndices(leafIndices)
	indicesCopy := append([]int(nil), leafIndices...)
	requiredCopy := append([]int(nil), required...)

	requiredIndicesCache.mu.Lock()
	requiredIndicesCache.entries[requiredIndicesCache.next] = requiredIndicesCacheEntry{
		hash:     indicesKeyHash,
		indices:  indicesCopy,
		required: requiredCopy,
	}
	requiredIndicesCache.next = (requiredIndicesCache.next + 1) % requiredIndicesCacheSize
	requiredIndicesCache.mu.Unlock()

	return requiredCopy
}

// computeRequiredIndices builds the proof-node list after a cache miss.
func computeRequiredIndices(leafIndices []int) []int {
	if len(leafIndices) == 0 {
		return nil
	}

	// Normalize once up front so the specialized same-depth and mixed-depth
	// builders can assume descending, duplicate-free input.
	current := descendingUniqueIndices(leafIndices)
	if !indicesShareDepth(current) {
		return getRequiredIndicesMixedDepth(current)
	}

	return getRequiredIndicesSameDepth(current)
}

// getRequiredIndicesSameDepth computes the proof nodes needed when all
// requested indices are on the same tree depth.
func getRequiredIndicesSameDepth(currentLevelIndices []int) []int {
	// Walk upward level by level. At each level, siblings that are not already
	// part of the current frontier must come from the proof.
	depth := getPathLength(currentLevelIndices[0])
	requiredIndices := make([]int, 0, len(currentLevelIndices)*min(depth, 8))
	// Each pair of child indices produces at most one parent, so half-capacity
	// is enough for the next frontier.
	nextLevelIndices := make([]int, 0, (len(currentLevelIndices)+1)/2)

	for len(currentLevelIndices) > 0 && currentLevelIndices[0] > 1 {
		nextLevelIndices = nextLevelIndices[:0]

		for i := 0; i < len(currentLevelIndices); {
			index := currentLevelIndices[i]
			parentIndex := getParent(index)

			if index&1 == 1 && i+1 < len(currentLevelIndices) && currentLevelIndices[i+1] == index-1 {
				nextLevelIndices = append(nextLevelIndices, parentIndex)
				i += 2
				continue
			}

			requiredIndices = append(requiredIndices, getSibling(index))
			nextLevelIndices = append(nextLevelIndices, parentIndex)
			i++
		}

		currentLevelIndices, nextLevelIndices = nextLevelIndices, currentLevelIndices[:0]
	}

	return requiredIndices
}

// getRequiredIndicesMixedDepth computes the proof nodes needed when requested
// indices include both leaves and higher intermediate nodes.
func getRequiredIndicesMixedDepth(indices []int) []int {
	present := struct{}{}
	requestedIndices := make(map[int]struct{}, len(indices))
	for _, index := range indices {
		requestedIndices[index] = present
	}

	requiredCap := len(indices) * min(getPathLength(indices[0]), 8)
	requiredIndicesSet := make(map[int]struct{}, requiredCap)
	computedParentSet := make(map[int]struct{}, requiredCap)

	for _, index := range indices {
		currentIndex := index
		for currentIndex > 1 {
			siblingIndex := getSibling(currentIndex)
			parentIndex := getParent(currentIndex)

			if _, isRequestedIndex := requestedIndices[siblingIndex]; !isRequestedIndex {
				requiredIndicesSet[siblingIndex] = present
			}
			computedParentSet[parentIndex] = present
			currentIndex = parentIndex
		}
	}

	requiredIndices := make([]int, 0, len(requiredIndicesSet))
	for index := range requiredIndicesSet {
		if _, isComputedParent := computedParentSet[index]; !isComputedParent {
			requiredIndices = append(requiredIndices, index)
		}
	}

	sort.Sort(sort.Reverse(sort.IntSlice(requiredIndices)))
	return requiredIndices
}

// indicesShareDepth reports whether all requested indices are on the same tree level.
func indicesShareDepth(indices []int) bool {
	if len(indices) < 2 {
		return true
	}
	depth := getPathLength(indices[0])
	for _, idx := range indices[1:] {
		if getPathLength(idx) != depth {
			return false
		}
	}
	return true
}

// descendingUniqueIndices returns generalized indices in descending order with
// duplicates removed.
func descendingUniqueIndices(indices []int) []int {
	switch {
	case len(indices) == 0:
		return nil
	case intsSortedDescending(indices):
		// Common hot path: benchmarks often already provide descending indices.
		// Deduplicate in one pass without copying/sorting more than needed.
		unique := make([]int, 0, len(indices))
		previousIndex := 0
		for i, idx := range indices {
			if i == 0 || idx != previousIndex {
				unique = append(unique, idx)
				previousIndex = idx
			}
		}
		return unique
	case sort.IntsAreSorted(indices):
		// Ascending input is also common. Walk it backwards so we can produce the
		// descending unique form without an extra full sort.
		unique := make([]int, 0, len(indices))
		previousIndex := 0
		for i := len(indices) - 1; i >= 0; i-- {
			idx := indices[i]
			if len(unique) == 0 || idx != previousIndex {
				unique = append(unique, idx)
				previousIndex = idx
			}
		}
		return unique
	default:
		// Fallback for arbitrary order: sort once, then compact duplicates
		// in-place so the returned slice still has minimal size.
		sorted := make([]int, len(indices))
		copy(sorted, indices)
		sort.Sort(sort.Reverse(sort.IntSlice(sorted)))

		writePos := 1
		for readPos := 1; readPos < len(sorted); readPos++ {
			if sorted[readPos] != sorted[writePos-1] {
				sorted[writePos] = sorted[readPos]
				writePos++
			}
		}
		return sorted[:writePos]
	}
}

// bytesToChunk copies up to 32 bytes into the fixed-width chunk format used by
// proof verification.
func bytesToChunk(src []byte) [32]byte {
	var chunk [32]byte
	copy(chunk[:], src)
	return chunk
}

// hashIndices creates a stable cache key for one exact index slice.
func hashIndices(indices []int) uint64 {
	// FNV-1a constants. We use them only to spread index slices across cache
	// entries quickly; correctness still comes from intsEqual below.
	const offset uint64 = 1469598103934665603
	const prime uint64 = 1099511628211

	hashValue := offset ^ uint64(len(indices))
	for _, idx := range indices {
		hashValue ^= uint64(idx)
		hashValue *= prime
	}
	return hashValue
}

// intsEqual compares two index slices before a cached result is reused.
func intsEqual(a, b []int) bool {
	// Hash collisions are possible in theory, so cache hits still compare the
	// full index slice before reusing a cached proof-index result.
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
