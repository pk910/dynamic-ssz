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

	reqIndices := getRequiredIndices(indices)
	if len(reqIndices) != len(proof) {
		return false, fmt.Errorf("number of proof hashes %d and required indices %d mismatch", len(proof), len(reqIndices))
	}

	// userGenIndices contains all generalised indices between leaves and proof hashes
	// in descending order so we can walk the tree bottom-up without an extra sort.
	leafGenIndices := descendingIndices(indices)
	// Create database of index -> value (hash) from inputs
	db := make(map[int][32]byte, len(indices)+len(reqIndices))

	for i, leaf := range leaves {
		db[indices[i]] = bytesToChunk(leaf)
	}
	for i, h := range proof {
		db[reqIndices[i]] = bytesToChunk(h)
	}

	// The depth of the tree up to the greatest index
	maxIndex := 0
	if len(leafGenIndices) > 0 {
		maxIndex = leafGenIndices[0]
	}
	if len(reqIndices) > 0 && reqIndices[0] > maxIndex {
		maxIndex = reqIndices[0]
	}

	var capacity int
	if maxIndex > 0 {
		capacity = getPathLength(maxIndex)
	}

	// Allocate space for auxiliary keys created when computing intermediate hashes
	// Auxiliary indices are useful to avoid using store all indices to traverse
	// in a single array and sort upon an insertion, which would be inefficient.
	auxGenIndices := make([]int, 0, capacity)

	// To keep track the current position to inspect in both arrays
	posLeaf := 0
	posProof := 0
	posAux := 0

	var tmp [64]byte
	var index int

	// Iter over the tree, computing hashes and storing them
	// in the in-memory database, until the root is reached.
	//
	// EXIT CONDITION: no more indices to use in both arrays
	for posAux < len(auxGenIndices) || posLeaf < len(leafGenIndices) || posProof < len(reqIndices) {
		// We need to establish from which array we're going to take the next index
		// by taking the largest available generalized index.
		index = 0
		source := 0
		if posAux < len(auxGenIndices) {
			index = auxGenIndices[posAux]
			source = 1
		}
		if posLeaf < len(leafGenIndices) && leafGenIndices[posLeaf] > index {
			index = leafGenIndices[posLeaf]
			source = 2
		}
		if posProof < len(reqIndices) && reqIndices[posProof] > index {
			index = reqIndices[posProof]
			source = 3
		}

		switch source {
		case 1:
			posAux++
		case 2:
			posLeaf++
		case 3:
			posProof++
		}

		// Root has been reached
		if index == 1 {
			break
		}

		parentIndex := getParent(index)

		// If the parent is already computed, we don't need to calculate the intermediate hash
		if _, hasParent := db[parentIndex]; hasParent {
			continue
		}

		leftIndex := (index | 1) ^ 1
		left, hasLeft := db[leftIndex]
		rightIndex := index | 1
		right, hasRight := db[rightIndex]

		if !hasRight || !hasLeft {
			return false, fmt.Errorf("proof is missing required nodes, either %d or %d", leftIndex, rightIndex)
		}

		copy(tmp[:32], left[:])
		copy(tmp[32:], right[:])
		db[parentIndex] = sha256.Sum256(tmp[:])

		// An intermediate hash has been computed, as such we need to store its index
		// to remember to examine it later
		auxGenIndices = append(auxGenIndices, parentIndex)
	}

	res, ok := db[1]
	if !ok {
		return false, fmt.Errorf("root was not computed during proof verification")
	}

	return res == bytesToChunk(root), nil
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

	buf := make([]byte, count*32)
	for i := range leaves {
		idx := i
		if reverse {
			idx = count - 1 - i
		}
		copy(buf[idx*32:], leaves[i])
	}
	hh.Append(buf)
	hh.Merkleize(0)

	return true, bytes.Equal(root, hh.Hash())
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

	tmp := leafIndices
	if !sort.IntsAreSorted(leafIndices) {
		tmp = make([]int, len(leafIndices))
		copy(tmp, leafIndices)
		sort.Ints(tmp)
	}

	exists := struct{}{}
	depth := getPathLength(tmp[len(tmp)-1])

	leaves := make(map[int]struct{}, len(tmp))
	for _, leaf := range tmp {
		leaves[leaf] = exists
	}

	requiredCap := len(tmp) * min(depth, 8)
	required := make(map[int]struct{}, requiredCap)
	computed := make(map[int]struct{}, requiredCap)

	for _, leaf := range tmp {
		cur := leaf
		for cur > 1 {
			sibling := getSibling(cur)
			parent := getParent(cur)

			if _, isLeaf := leaves[sibling]; !isLeaf {
				required[sibling] = exists
			}
			computed[parent] = exists
			cur = parent
		}
	}

	// Filter out nodes that will be computed on‑the‑fly.
	requiredList := make([]int, 0, len(required))
	// Remove computed indices from required ones
	for r := range required {
		if _, isComputed := computed[r]; !isComputed {
			requiredList = append(requiredList, r)
		}
	}

	sort.Sort(sort.Reverse(sort.IntSlice(requiredList)))
	return requiredList
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
