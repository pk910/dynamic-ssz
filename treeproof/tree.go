// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
//
// This file contains code derived from https://github.com/ferranbt/fastssz/blob/v1.0.0/tree.go
// Copyright (c) 2020 Ferran Borreguero
// Licensed under the MIT License
// The code has been modified for dynamic-ssz proof generation needs.

// Package treeproof provides Merkle tree construction and proof generation for SSZ structures.
//
// This package enables the construction of complete Merkle trees from SSZ-encoded data structures,
// supporting both traditional binary trees and progressive trees for advanced use cases. It provides
// functionality for generating and verifying Merkle proofs against generalized indices.
//
// Key features:
//   - Binary tree construction: Standard SSZ merkleization for fixed-size containers
//   - Progressive tree construction: Advanced merkleization for containers with optional fields
//   - Proof generation: Single and multi-proof generation for any tree node
//   - Proof verification: Standalone verification of generated proofs
//   - Tree visualization: Debug-friendly tree structure display with generalized indices
//
// The package supports the generalized index system used in Ethereum 2.0 for addressing
// nodes within Merkle trees, enabling efficient proof generation for any field or value
// within complex data structures.
package treeproof

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// Proof represents a Merkle proof for a single leaf against a generalized index.
//
// A Merkle proof consists of the leaf value and a sequence of sibling hashes
// needed to reconstruct the path from the leaf to the root. The proof can be
// verified independently to confirm that the leaf value exists at the specified
// generalized index within the tree.
//
// Fields:
//   - Index: The generalized index of the leaf being proven
//   - Leaf: The 32-byte value at the specified index
//   - Hashes: Ordered sequence of sibling hashes for the path to root
type Proof struct {
	Index  int      // Generalized index of the proven leaf
	Leaf   []byte   // 32-byte leaf value
	Hashes [][]byte // Sibling hashes for verification path
}

// Multiproof represents an efficient Merkle proof for multiple leaves.
//
// Instead of generating separate proofs for each leaf, a multiproof consolidates
// the verification data by sharing common intermediate hashes. This is more
// efficient when proving multiple values from the same tree.
//
// Fields:
//   - Indices: The generalized indices of all leaves being proven
//   - Leaves: The 32-byte values at the specified indices (same order as Indices)
//   - Hashes: Shared set of hashes needed to verify all leaves
type Multiproof struct {
	Indices []int    // Generalized indices of proven leaves
	Leaves  [][]byte // 32-byte leaf values (ordered by Indices)
	Hashes  [][]byte // Shared verification hashes
}

// Compress returns a new proof with zero hashes omitted.
// See `CompressedMultiproof` for more info.
func (p *Multiproof) Compress() *CompressedMultiproof {
	compressed := &CompressedMultiproof{
		Indices:    p.Indices,
		Leaves:     p.Leaves,
		Hashes:     make([][]byte, 0, len(p.Hashes)),
		ZeroLevels: make([]int, 0, len(p.Hashes)),
	}

	for _, h := range p.Hashes {
		if l, ok := hasher.GetZeroHashLevel(string(h)); ok {
			compressed.ZeroLevels = append(compressed.ZeroLevels, l)
			compressed.Hashes = append(compressed.Hashes, nil)
		} else {
			compressed.Hashes = append(compressed.Hashes, h)
		}
	}

	return compressed
}

// CompressedMultiproof represents a compressed merkle proof of several leaves.
// Compression is achieved by omitting zero hashes (and their hashes). `ZeroLevels`
// contains information which helps the verifier fill in those hashes.
type CompressedMultiproof struct {
	Indices    []int
	Leaves     [][]byte
	Hashes     [][]byte
	ZeroLevels []int // Stores the level for every omitted zero hash in the proof
}

// Decompress returns a new multiproof, filling in the omitted
// zero hashes. See `CompressedMultiProof` for more info.
func (c *CompressedMultiproof) Decompress() *Multiproof {
	p := &Multiproof{
		Indices: c.Indices,
		Leaves:  c.Leaves,
		Hashes:  make([][]byte, len(c.Hashes)),
	}

	zc := 0
	for i, h := range c.Hashes {
		if h == nil {
			p.Hashes[i] = hasher.GetZeroHash(c.ZeroLevels[zc])
			zc++
		} else {
			p.Hashes[i] = c.Hashes[i]
		}
	}

	return p
}

// Node represents a single node in a Merkle tree constructed from SSZ data.
//
// Each node can be either a leaf node (containing actual data) or a branch node
// (containing the hash of its children). The tree structure follows SSZ merkleization
// rules and supports both binary and progressive tree layouts.
//
// For leaf nodes:
//   - left and right are nil
//   - value contains the 32-byte leaf data
//
// For branch nodes:
//   - left and right point to child nodes
//   - value contains the computed hash of children (cached after first calculation)
//
// The isEmpty field indicates whether this is a "zero" node used for padding
// incomplete trees to maintain proper binary tree structure.
type Node struct {
	left    *Node  // Left child node (nil for leaves)
	right   *Node  // Right child node (nil for leaves)
	isEmpty bool   // True if this is a zero-padding node
	value   []byte // 32-byte value (data for leaves, hash for branches)
}

// Show displays the tree structure in a human-readable format for debugging.
//
// This method prints the complete tree hierarchy starting from this node,
// showing generalized indices, hash values, and the tree structure. It's
// particularly useful for understanding how SSZ data maps to tree nodes
// and for debugging proof generation.
//
// Parameters:
//   - maxDepth: Maximum depth to display (0 for unlimited depth)
//
// Output format:
//   - INDEX: The generalized index of each node
//   - HASH: 32-byte hash for branch nodes (computed from children)
//   - VALUE: 32-byte data for leaf nodes (actual SSZ field data)
//   - EMPTY: Indicates zero-padding nodes with their depth level
//   - LEFT/RIGHT: Tree structure showing child relationships
//
// Example output:
//
//	--- Show node ---
//	INDEX: 1
//	HASH: a1b2c3d4...
//	LEFT:
//	    INDEX: 2
//	    VALUE: e5f6g7h8...
//	RIGHT:
//	    INDEX: 3
//	    HASH: i9j0k1l2...
//	    LEFT:
//	        INDEX: 6
//	        VALUE: m3n4o5p6...
//	    RIGHT:
//	        INDEX: 7
//	        EMPTY: true (depth: 0)
func (n *Node) Show(maxDepth int) {
	fmt.Printf("--- Show node ---\n")
	n.show(0, maxDepth, 1) // Start with index 1 (root)
}

func (n *Node) show(depth int, maxDepth int, index int) {
	space := ""
	for i := 0; i < depth; i++ {
		space += "\t"
	}
	print := func(msgs ...string) {
		for _, msg := range msgs {
			fmt.Printf("%s%s", space, msg)
		}
	}

	// Always print the index first
	print(fmt.Sprintf("INDEX: %d\n", index))

	if n.left != nil || n.right != nil {
		// Branch node - show hash
		print("HASH: " + hex.EncodeToString(n.Hash()) + "\n")
	} else if n.value != nil {
		// Leaf node - show value only (no hash for leaves)
		print("VALUE: " + hex.EncodeToString(n.value) + "\n")
	}

	if n.isEmpty {
		zeroLevel, _ := hasher.GetZeroHashLevel(string(n.Hash()))
		print("EMPTY: true (depth: " + strconv.Itoa(zeroLevel) + ")\n")
	}

	if maxDepth > 0 {
		if depth == maxDepth {
			// only print hash if we are too deep
			print(" ... (max depth reached)\n")
			return
		}
	}

	if n.left != nil {
		print("LEFT: \n")
		n.left.show(depth+1, maxDepth, index*2) // Left child index = parent * 2
	}
	if n.right != nil {
		print("RIGHT: \n")
		n.right.show(depth+1, maxDepth, index*2+1) // Right child index = parent * 2 + 1
	}
}

// NewNodeWithValue initializes a leaf node.
func NewNodeWithValue(value []byte) *Node {
	return &Node{
		left:    nil,
		right:   nil,
		value:   value,
		isEmpty: bytes.Equal(value, sszutils.ZeroBytes()[:32]),
	}
}

func NewEmptyNode(zeroOrderHash []byte) *Node {
	return &Node{left: nil, right: nil, value: zeroOrderHash, isEmpty: true}
}

// NewNodeWithLR initializes a branch node.
func NewNodeWithLR(left, right *Node) *Node {
	return &Node{left: left, right: right, value: nil}
}

// TreeFromChunks constructs a tree from leaf values.
// The number of leaves should be a power of 2.
func TreeFromChunks(chunks [][]byte) (*Node, error) {
	numLeaves := len(chunks)
	if numLeaves == 0 {
		return nil, errors.New("cannot create tree from empty chunks")
	}
	if !isPowerOfTwo(numLeaves) {
		return nil, errors.New("number of leaves should be a power of 2")
	}

	leaves := make([]*Node, numLeaves)
	for i, c := range chunks {
		leaves[i] = NewNodeWithValue(c)
	}
	return TreeFromNodes(leaves, numLeaves)
}

// TreeFromNodes constructs a tree from leaf nodes.
// This is useful for merging subtrees.
// The limit should be a power of 2.
// Adjacent sibling nodes will be filled with zero order hashes that have been precomputed based on the tree depth.
func TreeFromNodes(leaves []*Node, limit int) (*Node, error) {
	numLeaves := len(leaves)

	if limit <= 0 {
		return NewEmptyNode(sszutils.ZeroBytes()[:32]), nil
	}

	depth := floorLog2(limit)
	zeroOrderHashes := getZeroOrderHashes(depth)

	// there are no leaves, return a zero order hash node
	if numLeaves == 0 {
		return NewEmptyNode(zeroOrderHashes[0]), nil
	}

	// now we know numLeaves are at least 1.

	// if the max leaf limit is 1, return the one leaf we have
	if limit == 1 {
		return leaves[0], nil
	}
	// if the max leaf limit is 2
	if limit == 2 {
		// but we only have 1 leaf, add a zero order hash as the right node
		if numLeaves == 1 {
			return NewNodeWithLR(leaves[0], NewEmptyNode(zeroOrderHashes[1])), nil
		}
		// otherwise return the two nodes we have
		return NewNodeWithLR(leaves[0], leaves[1]), nil
	}

	if !isPowerOfTwo(limit) {
		return nil, errors.New("number of leaves should be a power of 2")
	}

	leavesStart := powerTwo(depth)
	leafIndex := numLeaves - 1

	// compute a safe size for nodes slice (1-indexed)
	// max index we'll access: compute the subtree area; be generous to avoid out-of-range
	maxPotentialIndex := (leavesStart + numLeaves) * 2 + 4
	nodes := make([]*Node, maxPotentialIndex)

	nodesStartIndex := leavesStart
	nodesEndIndex := nodesStartIndex + numLeaves - 1

	// for each tree level
	for k := depth; k >= 0; k-- {
		for i := nodesEndIndex; i >= nodesStartIndex; i-- {
			// leaf node, add to slice
			if k == depth {
				// defensive check but should always be valid
				if leafIndex < 0 {
					return nil, errors.New("invalid leaf indexing")
				}
				nodes[i] = leaves[leafIndex]
				leafIndex--
			} else { // branch node, compute
				leftIndex := i * 2
				rightIndex := i*2 + 1
				// both nodes are empty, unexpected condition
				if (leftIndex >= len(nodes) || nodes[leftIndex] == nil) && (rightIndex >= len(nodes) || nodes[rightIndex] == nil) {
					return nil, errors.New("unexpected empty right and left nodes")
				}
				// node with empty right node, add zero order hash as right node and mark right node as empty
				if leftIndex < len(nodes) && nodes[leftIndex] != nil && (rightIndex >= len(nodes) || nodes[rightIndex] == nil) {
					nodes[i] = NewNodeWithLR(nodes[leftIndex], NewEmptyNode(zeroOrderHashes[k+1]))
				}
				// node with left and right child
				if leftIndex < len(nodes) && nodes[leftIndex] != nil && rightIndex < len(nodes) && nodes[rightIndex] != nil {
					nodes[i] = NewNodeWithLR(nodes[leftIndex], nodes[rightIndex])
				}
			}
		}
		nodesStartIndex = nodesStartIndex / 2
		nodesEndIndex = nodesEndIndex / 2
	}

	rootNode := nodes[1]

	if rootNode == nil {
		return nil, errors.New("tree root node could not be computed")
	}

	return rootNode, nil
}

// TreeFromNodesProgressive constructs a progressive tree from leaf nodes.
// This implements the progressive merkleization algorithm where chunks are split
// using base_size pattern (1, 4, 16, 64...) rather than even binary splits.
// Based on subtree_fill_progressive from remerkleable.
func TreeFromNodesProgressive(leaves []*Node) (*Node, error) {
	if len(leaves) == 0 {
		return NewEmptyNode(sszutils.ZeroBytes()[:32]), nil
	}

	return treeFromNodesProgressiveImpl(leaves, 0)
}

// treeFromNodesProgressiveImpl implements the recursive progressive tree construction
func treeFromNodesProgressiveImpl(leaves []*Node, depth int) (*Node, error) {
	if len(leaves) == 0 {
		return NewEmptyNode(sszutils.ZeroBytes()[:32]), nil
	}

	// Calculate base_size = 1 << depth (1, 4, 16, 64, 256...)
	baseSize := 1 << depth

	// Split nodes: first baseSize nodes go to RIGHT (binary), rest go to LEFT (progressive)
	splitPoint := baseSize
	if splitPoint > len(leaves) {
		splitPoint = len(leaves)
	}

	// Right child: binary merkleization of first baseSize nodes
	rightNodes := leaves[:splitPoint]
	rightChild, err := TreeFromNodes(rightNodes, baseSize)
	if err != nil {
		return nil, err
	}

	// Left child: recursive progressive merkleization of remaining nodes
	leftNodes := leaves[splitPoint:]
	var leftChild *Node
	if len(leftNodes) == 0 {
		leftChild = NewEmptyNode(sszutils.ZeroBytes()[:32])
	} else {
		leftChild, err = treeFromNodesProgressiveImpl(leftNodes, depth+2)
		if err != nil {
			return nil, err
		}
	}

	// Return PairNode(left, right)
	return NewNodeWithLR(leftChild, rightChild), nil
}

func TreeFromNodesWithMixin(leaves []*Node, num, limit int) (*Node, error) {
	if !isPowerOfTwo(limit) {
		limit = int(sszutils.NextPowerOfTwo(uint64(limit)))
	}

	mainTree, err := TreeFromNodes(leaves, limit)
	if err != nil {
		return nil, err
	}

	// Mixin len
	countLeaf := LeafFromUint64(uint64(num))
	node := NewNodeWithLR(mainTree, countLeaf)
	return node, nil
}

// TreeFromNodesProgressiveWithMixin constructs a progressive tree with length mixin.
// The progressive tree is created first, then mixed with the length value.
func TreeFromNodesProgressiveWithMixin(leaves []*Node, num int) (*Node, error) {
	mainTree, err := TreeFromNodesProgressive(leaves)
	if err != nil {
		return nil, err
	}

	// Mixin length (same as binary version)
	countLeaf := LeafFromUint64(uint64(num))
	node := NewNodeWithLR(mainTree, countLeaf)
	return node, nil
}

// TreeFromNodesProgressiveWithActiveFields constructs a progressive tree with active fields bitvector.
// The progressive tree is created first, then mixed with the active fields.
func TreeFromNodesProgressiveWithActiveFields(leaves []*Node, activeFields []byte) (*Node, error) {
	mainTree, err := TreeFromNodesProgressive(leaves)
	if err != nil {
		return nil, err
	}

	// Mixin active fields bitvector (convert to 32-byte padded leaf)
	activeFieldsLeaf := LeafFromBytes(activeFields)
	node := NewNodeWithLR(mainTree, activeFieldsLeaf)
	return node, nil
}

// Get fetches a node with the given general index.
func (n *Node) Get(index int) (*Node, error) {
	pathLen := getPathLength(index)
	cur := n
	for i := pathLen - 1; i >= 0; i-- {
		if isRight := getPosAtLevel(index, i); isRight {
			cur = cur.right
		} else {
			cur = cur.left
		}
		if cur == nil {
			return nil, errors.New("Node not found in tree")
		}
	}

	return cur, nil
}

// Hash returns the hash of the subtree with the given Node as its root.
// If root has no children, it returns root's value (not its hash).
func (n *Node) Hash() []byte {
	// TODO: handle special cases: empty root, one non-empty node
	return hashNode(n)
}

// Left returns the left child node, or nil if this is a leaf.
func (n *Node) Left() *Node {
	return n.left
}

// Right returns the right child node, or nil if this is a leaf.
func (n *Node) Right() *Node {
	return n.right
}

// IsLeaf returns true if this node has no children (is a leaf node).
func (n *Node) IsLeaf() bool {
	return n.left == nil && n.right == nil
}

// IsEmpty returns true if this node represents zero-padding.
func (n *Node) IsEmpty() bool {
	return n.isEmpty
}

// Value returns the raw 32-byte value stored in this node.
func (n *Node) Value() []byte {
	return n.value
}

func hashNode(n *Node) []byte {
	if n.left == nil && n.right == nil {
		return n.value
	}

	if n.left == nil {
		panic("Tree incomplete")
	}

	if n.value != nil {
		// This value has already been hashed, don't do the work again.
		return n.value
	}

	if n.right.isEmpty {
		result := hashFn(append(hashNode(n.left), n.right.value...))
		n.value = result // Set the hash result on each node so that proofs can be generated for any level
		return result
	}

	result := hashFn(append(hashNode(n.left), hashNode(n.right)...))
	n.value = result
	return result
}

// getZeroOrderHashes precomputes zero order hashes to create an easy map lookup
// for zero leafs and their parent nodes.
func getZeroOrderHashes(depth int) [][]byte {
	res := make([][]byte, depth+1)
	emptyValue := make([]byte, 32)
	res[depth] = emptyValue
	for i := depth - 1; i >= 0; i-- {
		res[i] = hashFn(append(res[i+1], res[i+1]...))
	}
	return res
}

// Prove returns a list of sibling values and hashes needed
// to compute the root hash for a given general index.
func (n *Node) Prove(index int) (*Proof, error) {
	pathLen := getPathLength(index)
	proof := &Proof{Index: index}
	hashes := make([][]byte, 0, pathLen)

	cur := n
	for i := pathLen - 1; i >= 0; i-- {
		var siblingHash []byte
		if isRight := getPosAtLevel(index, i); isRight {
			if cur.left == nil {
				return nil, errors.New("Node not found in tree")
			}
			siblingHash = hashNode(cur.left)
			cur = cur.right
		} else {
			if cur.right == nil {
				return nil, errors.New("Node not found in tree")
			}
			siblingHash = hashNode(cur.right)
			cur = cur.left
		}
		hashes = append(hashes, siblingHash)
		if cur == nil {
			return nil, errors.New("Node not found in tree")
		}
	}

	for i, j := 0, len(hashes)-1; i < j; i, j = i+1, j-1 {
		hashes[i], hashes[j] = hashes[j], hashes[i]
	}

	proof.Hashes = hashes
	if cur.value == nil {
		// This is an intermediate node without a value; add the hash to it so that we're providing a suitable leaf value.
		cur.value = hashNode(cur)
	}
	proof.Leaf = cur.value

	return proof, nil
}

func (n *Node) ProveMulti(indices []int) (*Multiproof, error) {
	reqIndices := getRequiredIndices(indices)
	proof := &Multiproof{Indices: indices, Leaves: make([][]byte, len(indices)), Hashes: make([][]byte, len(reqIndices))}

	for i, gi := range indices {
		node, err := n.Get(gi)
		if err != nil {
			return nil, err
		}
		proof.Leaves[i] = node.value
	}

	for i, gi := range reqIndices {
		cur, err := n.Get(gi)
		if err != nil {
			return nil, err
		}
		proof.Hashes[i] = hashNode(cur)
	}

	return proof, nil
}

func LeafFromUint64(i uint64) *Node {
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint64(buf[:8], i)
	return NewNodeWithValue(buf)
}

func LeafFromUint32(i uint32) *Node {
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf[:4], i)
	return NewNodeWithValue(buf)
}

func LeafFromUint16(i uint16) *Node {
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint16(buf[:2], i)
	return NewNodeWithValue(buf)
}

func LeafFromUint8(i uint8) *Node {
	buf := make([]byte, 32)
	buf[0] = byte(i)
	return NewNodeWithValue(buf)
}

func LeafFromBool(b bool) *Node {
	buf := make([]byte, 32)
	if b {
		buf[0] = 1
	}
	return NewNodeWithValue(buf)
}

func LeafFromBytes(b []byte) *Node {
	l := len(b)
	if l > 32 {
		panic("Unimplemented")
	}

	if l == 32 {
		return NewNodeWithValue(b[:])
	}

	// < 32
	return NewNodeWithValue(append(b, sszutils.ZeroBytes()[:32-l]...))
}

func EmptyLeaf() *Node {
	return NewNodeWithValue(sszutils.ZeroBytes()[:32])
}

func LeavesFromUint64(items []uint64) []*Node {
	if len(items) == 0 {
		return []*Node{}
	}

	numLeaves := (len(items)*8 + 31) / 32
	buf := make([]byte, numLeaves*32)
	for i, v := range items {
		binary.LittleEndian.PutUint64(buf[i*8:(i+1)*8], v)
	}

	leaves := make([]*Node, numLeaves)
	for i := 0; i < numLeaves; i++ {
		v := buf[i*32 : (i+1)*32]
		leaves[i] = NewNodeWithValue(v)
	}

	return leaves
}

func isPowerOfTwo(n int) bool {
	return (n & (n - 1)) == 0
}

func floorLog2(n int) int {
	return int(math.Floor(math.Log2(float64(n))))
}

func powerTwo(n int) int {
	return int(math.Pow(2, float64(n)))
}
