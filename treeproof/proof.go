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

	requiredProofIndices := getRequiredIndices(indices)
	if len(requiredProofIndices) != len(proof) {
		return false, fmt.Errorf("number of proof hashes %d and required indices %d mismatch", len(proof), len(requiredProofIndices))
	}

	// Keep leaf indices in descending generalized-index order so verification can
	// walk the tree bottom-up without building another sorted slice.
	leafGenIndices := newDescendingIndexCursor(indices)
	// Pre-size the hash lookup with leaves, proof nodes, and a small allowance
	// for the intermediate parents we will compute during verification.
	hashByIndexCap := len(indices) + len(requiredProofIndices)
	if maxReqIndex := len(requiredProofIndices); maxReqIndex > 0 {
		hashByIndexCap += getPathLength(requiredProofIndices[0])
	} else if leafGenIndices.ok() {
		hashByIndexCap += getPathLength(leafGenIndices.current())
	}
	hashByIndex := make(map[int][32]byte, hashByIndexCap)

	for i, leaf := range leaves {
		hashByIndex[indices[i]] = bytesToChunk(leaf)
	}
	for i, h := range proof {
		hashByIndex[requiredProofIndices[i]] = bytesToChunk(h)
	}

	// The depth of the tree up to the greatest index
	maxIndex := 0
	if leafGenIndices.ok() {
		maxIndex = leafGenIndices.current()
	}
	if len(requiredProofIndices) > 0 && requiredProofIndices[0] > maxIndex {
		maxIndex = requiredProofIndices[0]
	}

	var capacity int
	if maxIndex > 0 {
		capacity = getPathLength(maxIndex)
	}

	// Allocate space for auxiliary keys created when computing intermediate hashes
	// Auxiliary indices are useful to avoid using store all indices to traverse
	// in a single array and sort upon an insertion, which would be inefficient.
	pendingParentIndices := make([]int, 0, capacity)

	// To keep track the current position to inspect in both arrays
	proofPos := 0
	pendingPos := 0

	var tmp [64]byte
	var index int

	// Iter over the tree, computing hashes and storing them
	// in the in-memory database, until the root is reached.
	//
	// EXIT CONDITION: no more indices to use in both arrays
	const (
		sourcePendingParent = 1
		sourceLeaf          = 2
		sourceProof         = 3
	)
	for pendingPos < len(pendingParentIndices) || leafGenIndices.ok() || proofPos < len(requiredProofIndices) {
		// We need to establish from which array we're going to take the next index
		// by taking the largest available generalized index.
		index = 0
		source := 0
		if pendingPos < len(pendingParentIndices) {
			index = pendingParentIndices[pendingPos]
			source = sourcePendingParent
		}
		if leafGenIndices.ok() && leafGenIndices.current() > index {
			index = leafGenIndices.current()
			source = sourceLeaf
		}
		if proofPos < len(requiredProofIndices) && requiredProofIndices[proofPos] > index {
			index = requiredProofIndices[proofPos]
			source = sourceProof
		}

		switch source {
		case sourcePendingParent:
			pendingPos++
		case sourceLeaf:
			leafGenIndices.advance()
		case sourceProof:
			proofPos++
		}

		// Root has been reached
		if index == 1 {
			break
		}

		parentIndex := getParent(index)

		// If the parent is already computed, we don't need to calculate the intermediate hash
		if _, hasParent := hashByIndex[parentIndex]; hasParent {
			continue
		}

		leftIndex := (index | 1) ^ 1
		left, hasLeft := hashByIndex[leftIndex]
		rightIndex := index | 1
		right, hasRight := hashByIndex[rightIndex]

		if !hasRight || !hasLeft {
			return false, fmt.Errorf("proof is missing required nodes, either %d or %d", leftIndex, rightIndex)
		}

		copy(tmp[:32], left[:])
		copy(tmp[32:], right[:])
		hashByIndex[parentIndex] = sha256.Sum256(tmp[:])

		// An intermediate hash has been computed, as such we need to store its index
		// to remember to examine it later
		pendingParentIndices = append(pendingParentIndices, parentIndex)
	}

	res, ok := hashByIndex[1]
	if !ok {
		return false, fmt.Errorf("root was not computed during proof verification")
	}

	return res == bytesToChunk(root), nil
}

// descendingIndexCursor iterates a set of generalized indices from largest to
// smallest while reusing the caller's slice whenever it is already ordered.
type descendingIndexCursor struct {
	values []int
	pos    int
	step   int
}

func descendingIndices(indices []int) []int {
	switch {
	case intsSortedDescending(indices):
		return indices
	case sort.IntsAreSorted(indices):
		out := make([]int, len(indices))
		for i := range indices {
			out[i] = indices[len(indices)-1-i]
		}
		return out
	default:
		out := make([]int, len(indices))
		copy(out, indices)
		sort.Sort(sort.Reverse(sort.IntSlice(out)))
		return out
	}
}

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

func (c descendingIndexCursor) ok() bool {
	return c.pos >= 0 && c.pos < len(c.values)
}

func (c descendingIndexCursor) current() int {
	return c.values[c.pos]
}

func (c *descendingIndexCursor) advance() {
	c.pos += c.step
}

func intsSortedDescending(indices []int) bool {
	for i := 1; i < len(indices); i++ {
		if indices[i-1] < indices[i] {
			return false
		}
	}
	return true
}

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
func appendProofLeaf(hh *hasher.Hasher, zeroChunk []byte, leaf []byte) {
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

// Returns the position (i.e. false for left, true for right)
// of an index at a given level.
// Level 0 is the actual index's level, Level 1 is the position
// of the parent, etc.
func getPosAtLevel(index, level int) bool {
	return (index & (1 << level)) > 0
}

// Returns the length of the path to a node represented by its generalized index.
func getPathLength(index int) int {
	return bits.Len(uint(index)) - 1
}

// Returns the generalized index for a node's sibling.
func getSibling(index int) int {
	return index ^ 1
}

// Returns the generalized index for a node's parent.
func getParent(index int) int {
	return index >> 1
}

// Returns generalized indices for all nodes in the tree that are
// required to prove the given leaf indices. The returned indices
// are in a decreasing order.
func getRequiredIndices(leafIndices []int) []int {
	if len(leafIndices) == 0 {
		return nil
	}

	// Walk upward level by level. At each level, siblings that are not already
	// part of the current frontier must come from the proof.
	current := descendingUniqueIndices(leafIndices)
	depth := getPathLength(current[0])
	required := make([]int, 0, len(current)*min(depth, 8))
	next := make([]int, 0, len(current))

	for len(current) > 0 && current[0] > 1 {
		next = next[:0]
		producedPos := 0

		for i := 0; i < len(current); {
			idx := current[i]
			parent := getParent(idx)

			if idx&1 == 1 && i+1 < len(current) && current[i+1] == idx-1 {
				next = append(next, parent)
				i += 2
				continue
			}

			sibling := getSibling(idx)
			for producedPos < len(next) && next[producedPos] > sibling {
				producedPos++
			}
			if producedPos >= len(next) || next[producedPos] != sibling {
				required = append(required, sibling)
			}

			next = append(next, parent)
			i++
		}

		current, next = next, current[:0]
	}

	return required
}

// descendingUniqueIndices returns generalized indices in descending order with
// duplicates removed so each tree node is processed once.
func descendingUniqueIndices(indices []int) []int {
	sorted := descendingIndices(indices)
	unique := make([]int, 0, len(sorted))
	prev := 0
	for i, idx := range sorted {
		if i == 0 || idx != prev {
			unique = append(unique, idx)
			prev = idx
		}
	}
	return unique
}

func hashFn(data []byte) []byte {
	res := sha256.Sum256(data)
	return res[:]
}

func bytesToChunk(src []byte) [32]byte {
	var chunk [32]byte
	copy(chunk[:], src)
	return chunk
}
