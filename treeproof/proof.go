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
)

// VerifyProof verifies a single merkle branch. It's more
// efficient than VerifyMultiproof for proving one leaf.
func VerifyProof(root []byte, proof *Proof) (bool, error) {
	if len(proof.Hashes) != getPathLength(proof.Index) {
		return false, errors.New("invalid proof length")
	}

	node := proof.Leaf
	var tmp [64]byte
	for i, h := range proof.Hashes {
		if getPosAtLevel(proof.Index, i) {
			copy(tmp[:32], h)
			copy(tmp[32:], node)
		} else {
			copy(tmp[:32], node)
			copy(tmp[32:], h)
		}
		node = hashFn(tmp[:])
	}

	return bytes.Equal(root, node), nil
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
func VerifyMultiproof(root []byte, proof [][]byte, leaves [][]byte, indices []int) (bool, error) {
	if len(indices) == 0 {
		return false, errors.New("indices length is zero")
	}

	if len(leaves) != len(indices) {
		return false, errors.New("number of leaves and indices mismatch")
	}

	reqIndices := getRequiredIndices(indices)
	if len(reqIndices) != len(proof) {
		return false, fmt.Errorf("number of proof hashes %d and required indices %d mismatch", len(proof), len(reqIndices))
	}

	// userGenIndices contains all generalised indices between leaves and proof hashes
	// i.e., the indices retrieved from the user of this function
	userGenIndices := make([]int, 0, len(indices)+len(reqIndices))
	// Create database of index -> value (hash) from inputs
	db := make(map[int][]byte, len(indices)+len(reqIndices))

	for i, leaf := range leaves {
		db[indices[i]] = leaf
		userGenIndices = append(userGenIndices, indices[i])
	}
	for i, h := range proof {
		db[reqIndices[i]] = h
		userGenIndices = append(userGenIndices, reqIndices[i])
	}

	// Make sure keys are sorted in reverse order since we start from the leaves
	sort.Sort(sort.Reverse(sort.IntSlice(userGenIndices)))

	// The depth of the tree up to the greatest index
	var cap int
	if len(userGenIndices) > 0 {
		cap = getPathLength(userGenIndices[0])
	}

	// Allocate space for auxiliary keys created when computing intermediate hashes
	// Auxiliary indices are useful to avoid using store all indices to traverse
	// in a single array and sort upon an insertion, which would be inefficient.
	auxGenIndices := make([]int, 0, cap)

	// To keep track the current position to inspect in both arrays
	pos := 0
	posAux := 0

	var tmp [64]byte
	var index int

	// Iter over the tree, computing hashes and storing them
	// in the in-memory database, until the root is reached.
	//
	// EXIT CONDITION: no more indices to use in both arrays
	for posAux < len(auxGenIndices) || pos < len(userGenIndices) {
		// We need to establish from which array we're going to take the next index
		//
		// 1. If we've no auxiliary indices yet, we're going to use the generalised ones
		// 2. If we have no more client indices, we're going to use the auxiliary ones
		// 3. If we both, then we're going to compare them and take the biggest one
		if posAux >= len(auxGenIndices) {
			// Case 1: No more auxiliary indices
			index = userGenIndices[pos]
			pos++
		} else if pos >= len(userGenIndices) {
			// Case 2: No more user/proof indices
			index = auxGenIndices[posAux]
			posAux++
		} else if auxGenIndices[posAux] < userGenIndices[pos] {
			// Case 3: Both exist, take the larger (user/proof index)
			index = userGenIndices[pos]
			pos++
		} else {
			// Case 4: Both exist, take the larger (auxiliary index)
			index = auxGenIndices[posAux]
			posAux++
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

		copy(tmp[:32], left)
		copy(tmp[32:], right)
		db[parentIndex] = hashFn(tmp[:])

		// An intermediate hash has been computed, as such we need to store its index
		// to remember to examine it later
		auxGenIndices = append(auxGenIndices, parentIndex)
	}

	res, ok := db[1]
	if !ok {
		return false, fmt.Errorf("root was not computed during proof verification")
	}

	return bytes.Equal(res, root), nil
}

// Returns the position (i.e. false for left, true for right)
// of an index at a given level.
// Level 0 is the actual index's level, Level 1 is the position
// of the parent, etc.
func getPosAtLevel(index int, level int) bool {
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

	// Make a local copy so we can sort if needed (callers often already sorted,
	// but this keeps behavior identical even when they are not).
	tmp := make([]int, len(leafIndices))
	copy(tmp, leafIndices)
	sort.Ints(tmp)

	exists := struct{}{}

	leaves := make(map[int]struct{}, len(tmp))
	for _, leaf := range tmp {
		leaves[leaf] = exists
	}

	required := make(map[int]struct{}, len(tmp))
	computed := make(map[int]struct{}, len(tmp))

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
