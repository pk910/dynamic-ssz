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
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"strconv"
	"sync"

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
		if l, ok := hasher.GetZeroHashLevelBytes(h); ok {
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
			level := 0
			if zc < len(c.ZeroLevels) {
				level = c.ZeroLevels[zc]
			}
			p.Hashes[i] = hasher.GetZeroHash(level)
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

var (
	emptyNodeInit  sync.Once
	emptyNodeCache [65]*Node
)

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

func (n *Node) show(depth, maxDepth, index int) {
	space := ""
	for i := 0; i < depth; i++ {
		space += "\t"
	}
	printNode := func(msgs ...string) {
		for _, msg := range msgs {
			fmt.Printf("%s%s", space, msg)
		}
	}

	// Always print the index first
	printNode(fmt.Sprintf("INDEX: %d\n", index))

	if n.left != nil || n.right != nil {
		// Branch node - show hash
		printNode("HASH: " + hex.EncodeToString(n.Hash()) + "\n")
	} else if n.value != nil {
		// Leaf node - show value only (no hash for leaves)
		printNode("VALUE: " + hex.EncodeToString(n.value) + "\n")
	}

	if n.isEmpty {
		zeroLevel, _ := hasher.GetZeroHashLevelBytes(n.Hash())
		printNode("EMPTY: true (depth: " + strconv.Itoa(zeroLevel) + ")\n")
	}

	if maxDepth > 0 {
		if depth == maxDepth {
			// only print hash if we are too deep
			printNode(" ... (max depth reached)\n")
			return
		}
	}

	if n.left != nil {
		printNode("LEFT: \n")
		n.left.show(depth+1, maxDepth, index*2) // Left child index = parent * 2
	}
	if n.right != nil {
		printNode("RIGHT: \n")
		n.right.show(depth+1, maxDepth, index*2+1) // Right child index = parent * 2 + 1
	}
}

// NewNodeWithValue initializes a leaf node.
func NewNodeWithValue(value []byte) *Node {
	return &Node{
		left:    nil,
		right:   nil,
		value:   value,
		isEmpty: isZeroLeafValue(value),
	}
}

// NewEmptyNode creates an empty (zero-padding) tree node with the given
// precomputed zero-order hash. Empty nodes represent unused positions in the
// binary tree and are marked with isEmpty=true for efficient proof compression.
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

// treeFromNodesToDepthFn is the injectable core tree builder. Internal callers
// route binary-tree construction through it so tests can replace it to exercise
// otherwise-unreachable defensive error paths.
var treeFromNodesToDepthFn = treeFromNodesToDepth

// TreeFromNodes constructs a tree from leaf nodes.
// This is useful for merging subtrees.
// The limit should be a power of 2.
// Adjacent sibling nodes will be filled with zero order hashes that have been precomputed based on the tree depth.
func TreeFromNodes(leaves []*Node, limit int) (*Node, error) {
	// Excess leaves would be dropped silently, producing a valid-looking root for
	// a different tree. A negative limit is an int-overflow artifact (a chunk
	// limit above the platform int max, only possible on 32-bit) handled as an
	// empty capacity, not treated as excess.
	if limit >= 0 && len(leaves) > limit {
		return nil, fmt.Errorf("number of leaves %d exceeds limit %d", len(leaves), limit)
	}
	if limit <= 0 {
		return getEmptyNode(0), nil
	}
	return TreeFromNodes64(leaves, uint64(limit))
}

// TreeFromNodes64 is the uint64 form of TreeFromNodes and carries the canonical
// logic. limit is the leaf capacity (0/1 or a power of two); capacities up to
// 2^63 are representable. The tree is built from its depth (log2(limit)), so
// construction is O(depth·leaves) regardless of how large the capacity is.
func TreeFromNodes64(leaves []*Node, limit uint64) (*Node, error) {
	if limit == 0 {
		// A zero capacity cannot hold any leaf; accepting one would silently drop
		// it and produce a valid-looking root for a different tree.
		if len(leaves) > 0 {
			return nil, fmt.Errorf("number of leaves %d exceeds limit %d", len(leaves), 0)
		}
		return getEmptyNode(0), nil
	}
	if uint64(len(leaves)) > limit {
		return nil, fmt.Errorf("number of leaves %d exceeds limit %d", len(leaves), limit)
	}
	if limit > 1 && limit&(limit-1) != 0 {
		return nil, errors.New("number of leaves should be a power of 2")
	}
	return treeFromNodesToDepthFn(leaves, chunkLimitDepth(limit))
}

// treeFromNodesToDepth builds a binary Merkle tree from leaves padded with
// zero-order-hash subtrees to a capacity of 2^depth leaves. depth ranges over
// [0, 64]; driving construction by the depth (rather than a 2^depth leaf count)
// keeps it overflow-free for capacities beyond the platform int range.
func treeFromNodesToDepth(leaves []*Node, depth int) (*Node, error) {
	numLeaves := len(leaves)

	// Reject excess leaves (silently dropped otherwise) when 2^depth is
	// representable; for depth >= 63 the capacity dwarfs any real leaf count.
	if depth >= 0 && depth < 63 && numLeaves > (1<<uint(depth)) {
		return nil, fmt.Errorf("number of leaves %d exceeds limit %d", numLeaves, 1<<uint(depth))
	}

	// there are no leaves, return a zero order hash node
	if numLeaves == 0 {
		if depth < 0 {
			depth = 0
		}
		return getEmptyNode(depth), nil
	}

	// A nil leaf cannot be hashed and would panic later in Hash(); reject it at
	// construction (before any leaf is returned) so the tree is always complete.
	for i := range leaves {
		if leaves[i] == nil {
			return nil, fmt.Errorf("leaf at index %d is nil", i)
		}
	}

	// depth 0 is a single-leaf capacity: return the sole leaf.
	if depth <= 0 {
		return leaves[0], nil
	}

	firstLevelCount := (numLeaves + 1) / 2
	activeCount := firstLevelCount
	totalBranches := firstLevelCount
	for d := depth - 1; d > 0; d-- {
		activeCount = (activeCount + 1) / 2
		totalBranches += activeCount
	}

	// Build all branch nodes inside one slice so we do not allocate a new Node
	// for every parent we create.
	branchNodes := make([]Node, totalBranches)
	current := make([]*Node, firstLevelCount)
	next := make([]*Node, firstLevelCount)
	branchPos := 0

	for i := range firstLevelCount {
		leftIdx := i * 2
		rightIdx := i*2 + 1

		var left, right *Node
		left = leaves[leftIdx]
		if rightIdx < numLeaves {
			right = leaves[rightIdx]
		} else {
			right = getEmptyNode(0)
		}
		branchNodes[branchPos] = Node{left: left, right: right}
		current[i] = &branchNodes[branchPos]
		branchPos++
	}

	// Reuse the same two pointer slices for each level and just swap their roles
	// as we move up the tree.
	activeCount = firstLevelCount
	for d := depth - 1; d > 0; d-- {
		nextLevelCount := (activeCount + 1) / 2
		for i := range nextLevelCount {
			leftIdx := i * 2
			rightIdx := i*2 + 1

			var left, right *Node
			left = current[leftIdx]
			if rightIdx < activeCount {
				right = current[rightIdx]
			} else {
				right = getEmptyNode(depth - d)
			}
			branchNodes[branchPos] = Node{left: left, right: right}
			next[i] = &branchNodes[branchPos]
			branchPos++
		}
		current, next = next[:nextLevelCount], current
		activeCount = nextLevelCount
	}

	return current[0], nil
}

// chunkLimitDepth returns ceil(log2(limit)) clamped to [0, 64] — the depth of a
// binary Merkle tree padded to `limit` chunks. It mirrors hasher.getDepth so the
// tree wrapper and the streaming hasher agree on tree shape (and therefore root)
// for every limit, including capacities above the platform int range.
func chunkLimitDepth(limit uint64) int {
	if limit <= 1 {
		return 0
	}
	p := sszutils.NextPowerOfTwo(limit)
	if p == 0 {
		// limit exceeds 2^63, so the next power of two (2^64) overflows uint64.
		return 64
	}
	return bits.TrailingZeros64(p)
}

// TreeFromNodesProgressive constructs a progressive tree from leaf nodes.
// This implements the progressive merkleization algorithm where chunks are split
// using base_size pattern (1, 4, 16, 64...) rather than even binary splits.
// Based on subtree_fill_progressive from remerkleable.
func TreeFromNodesProgressive(leaves []*Node) (*Node, error) {
	if len(leaves) == 0 {
		return getEmptyNode(0), nil
	}

	// A nil leaf cannot be hashed and would panic later in Hash(); reject it at
	// construction so the tree is always complete.
	for i := range leaves {
		if leaves[i] == nil {
			return nil, fmt.Errorf("leaf at index %d is nil", i)
		}
	}

	return treeFromNodesProgressiveImpl(leaves, 0)
}

// treeFromNodesProgressiveImpl implements the recursive progressive tree construction
func treeFromNodesProgressiveImpl(leaves []*Node, depth int) (*Node, error) {
	if len(leaves) == 0 {
		return getEmptyNode(0), nil
	}

	// Calculate base_size = 1 << depth (1, 4, 16, 64, 256...)
	baseSize := 1 << depth

	// Split nodes: first baseSize nodes go to LEFT (binary), rest go to RIGHT (progressive)
	splitPoint := baseSize
	if splitPoint > len(leaves) {
		splitPoint = len(leaves)
	}

	// Left child: binary merkleization of first baseSize nodes. baseSize is
	// 1<<depth, so the binary subtree has exactly this depth.
	leftNodes := leaves[:splitPoint]
	leftChild, err := treeFromNodesToDepthFn(leftNodes, depth)
	if err != nil {
		return nil, err
	}

	// Right child: recursive progressive merkleization of remaining nodes
	rightNodes := leaves[splitPoint:]
	var rightChild *Node
	if len(rightNodes) == 0 {
		rightChild = getEmptyNode(0)
	} else {
		rightChild, err = treeFromNodesProgressiveImpl(rightNodes, depth+2)
		if err != nil {
			return nil, err
		}
	}

	// Return PairNode(left, right)
	return NewNodeWithLR(leftChild, rightChild), nil
}

// TreeFromNodesWithMixin constructs a Merkle tree from leaves and mixes in the
// element count as a right sibling of the root. This is the standard SSZ
// merkleization for lists, where the tree root is hash(merkle_root || length).
// The limit is rounded up to the next power of two if not already one.
func TreeFromNodesWithMixin(leaves []*Node, num, limit int) (*Node, error) {
	if limit < 0 {
		// int-overflow artifact (32-bit): treat as an empty capacity.
		limit = 0
	}
	if num < 0 {
		num = 0
	}
	return TreeFromNodesWithMixin64(leaves, uint64(num), uint64(limit))
}

// TreeFromNodesWithMixin64 is the uint64 form of TreeFromNodesWithMixin and
// carries the canonical logic: it builds the list tree padded to `limit` chunks
// (rounded up to a power of two via the tree depth) and mixes in the element
// count as the right sibling of the root.
func TreeFromNodesWithMixin64(leaves []*Node, num, limit uint64) (*Node, error) {
	if limit == 0 && len(leaves) > 0 {
		// A zero capacity cannot hold any leaf (see TreeFromNodes64).
		return nil, fmt.Errorf("number of leaves %d exceeds limit %d", len(leaves), 0)
	}
	mainTree, err := treeFromNodesToDepthFn(leaves, chunkLimitDepth(limit))
	if err != nil {
		return nil, err
	}

	// Mixin len
	countLeaf := LeafFromUint64(num)
	node := NewNodeWithLR(mainTree, countLeaf)
	return node, nil
}

// TreeFromNodesProgressiveWithMixin constructs a progressive tree with length mixin.
// The progressive tree is created first, then mixed with the length value.
func TreeFromNodesProgressiveWithMixin(leaves []*Node, num int) (*Node, error) {
	if num < 0 {
		num = 0
	}
	return TreeFromNodesProgressiveWithMixin64(leaves, uint64(num))
}

// TreeFromNodesProgressiveWithMixin64 is the uint64 form of
// TreeFromNodesProgressiveWithMixin.
func TreeFromNodesProgressiveWithMixin64(leaves []*Node, num uint64) (*Node, error) {
	mainTree, err := TreeFromNodesProgressive(leaves)
	if err != nil {
		return nil, err
	}

	// Mixin length (same as binary version)
	countLeaf := LeafFromUint64(num)
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
	if index < 1 {
		return nil, errors.New("Node not found in tree")
	}
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

// Value returns a copy of the 32-byte value stored in this node. A copy is
// returned because empty (zero-padding) nodes alias the process-wide zero-hash
// table and cached empty nodes are shared across trees; handing out the raw
// slice would let a caller's mutation corrupt every other tree and root.
func (n *Node) Value() []byte {
	return bytes.Clone(n.value)
}

func getEmptyNode(depth int) *Node {
	emptyNodeInit.Do(func() {
		for i := range emptyNodeCache {
			emptyNodeCache[i] = NewEmptyNode(hasher.GetZeroHash(i))
		}
	})
	return emptyNodeCache[depth]
}

func hashNode(n *Node) []byte {
	if n == nil {
		panic("Tree incomplete")
	}

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

	if n.right == nil {
		panic("Tree incomplete")
	}

	if n.right.isEmpty {
		result := hashPair(hashNode(n.left), n.right.value)
		n.value = result // Set the hash result on each node so that proofs can be generated for any level
		return result
	}

	result := hashPair(hashNode(n.left), hashNode(n.right))
	n.value = result
	return result
}

// Prove returns a list of sibling values and hashes needed
// to compute the root hash for a given general index.
func (n *Node) Prove(index int) (*Proof, error) {
	if index < 1 {
		return nil, fmt.Errorf("invalid generalized index %d (must be >= 1)", index)
	}
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
		// Copy the sibling hash: empty siblings alias the shared zero-hash table,
		// and the caller must be free to mutate the returned proof.
		hashes = append(hashes, bytes.Clone(siblingHash))
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
	proof.Leaf = bytes.Clone(cur.value)

	return proof, nil
}

// ProveMulti generates a Multiproof for the given set of generalized indices.
// It collects the leaf values at each index and the minimal set of auxiliary
// hashes needed to reconstruct the root. Returns an error if any index cannot
// be found in the tree.
func (n *Node) ProveMulti(indices []int) (*Multiproof, error) {
	for _, gi := range indices {
		if gi < 1 {
			return nil, fmt.Errorf("invalid generalized index %d (must be >= 1)", gi)
		}
	}
	reqIndices := getRequiredIndices(indices)
	proof := &Multiproof{Indices: indices, Leaves: make([][]byte, len(indices)), Hashes: make([][]byte, len(reqIndices))}

	// Copy leaf and hash values: empty nodes alias the shared zero-hash table and
	// cached empty nodes are shared across trees, so the returned proof must own
	// its bytes to stay mutation-safe.
	for i, gi := range indices {
		node, err := n.Get(gi)
		if err != nil {
			return nil, err
		}
		proof.Leaves[i] = bytes.Clone(node.value)
	}

	for i, gi := range reqIndices {
		cur, err := n.Get(gi)
		if err != nil {
			return nil, err
		}
		proof.Hashes[i] = bytes.Clone(hashNode(cur))
	}

	return proof, nil
}

// LeafFromUint64 creates a 32-byte leaf node from a uint64 value, encoded as
// little-endian in the first 8 bytes with the remaining 24 bytes zero-padded.
func LeafFromUint64(i uint64) *Node {
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint64(buf[:8], i)
	return NewNodeWithValue(buf)
}

// LeafFromUint32 creates a 32-byte leaf node from a uint32 value, encoded as
// little-endian in the first 4 bytes with the remaining 28 bytes zero-padded.
func LeafFromUint32(i uint32) *Node {
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf[:4], i)
	return NewNodeWithValue(buf)
}

// LeafFromUint16 creates a 32-byte leaf node from a uint16 value, encoded as
// little-endian in the first 2 bytes with the remaining 30 bytes zero-padded.
func LeafFromUint16(i uint16) *Node {
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint16(buf[:2], i)
	return NewNodeWithValue(buf)
}

// LeafFromUint8 creates a 32-byte leaf node from a uint8 value, stored in the
// first byte with the remaining 31 bytes zero-padded.
func LeafFromUint8(i uint8) *Node {
	buf := make([]byte, 32)
	buf[0] = i
	return NewNodeWithValue(buf)
}

// LeafFromBool creates a 32-byte leaf node from a boolean value, encoded as
// 0x01 (true) or 0x00 (false) in the first byte with 31 bytes zero-padded.
func LeafFromBool(b bool) *Node {
	buf := make([]byte, 32)
	if b {
		buf[0] = 1
	}
	return NewNodeWithValue(buf)
}

// LeafFromBytes creates a tree node from a byte slice. A slice of 32 bytes
// becomes a single leaf, a shorter slice is right-padded with zeros, and a
// longer slice is split into 32-byte chunks that are merkleized into a subtree
// so the node still hashes to a single 32-byte root.
func LeafFromBytes(b []byte) *Node {
	l := len(b)
	if l == 32 {
		return NewNodeWithValue(b)
	}
	if l < 32 {
		return NewNodeWithValue(append(b, sszutils.ZeroBytes()[:32-l]...))
	}

	numChunks := (l + 31) / 32
	leaves := make([]*Node, numChunks)
	for i := range leaves {
		start := i * 32
		if end := start + 32; end <= l {
			leaves[i] = NewNodeWithValue(b[start:end])
		} else {
			chunk := make([]byte, 32)
			copy(chunk, b[start:])
			leaves[i] = NewNodeWithValue(chunk)
		}
	}

	// limit is a power of two, so TreeFromNodes never returns an error here.
	node, _ := TreeFromNodes(leaves, int(sszutils.NextPowerOfTwo(uint64(numChunks))))
	return node
}

// EmptyLeaf creates a leaf node containing 32 zero bytes, representing an
// empty or unset value in the Merkle tree.
func EmptyLeaf() *Node {
	return NewNodeWithValue(sszutils.ZeroBytes()[:32])
}

// LeavesFromUint64 packs a slice of uint64 values into leaf nodes, with 4
// values per 32-byte leaf (8 bytes each, little-endian). The final leaf is
// zero-padded if the number of items is not a multiple of 4.
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

func isZeroLeafValue(value []byte) bool {
	if len(value) != 32 {
		return false
	}
	for _, b := range value {
		if b != 0 {
			return false
		}
	}
	return true
}

func hashPair(left, right []byte) []byte {
	var input [64]byte
	copy(input[:32], left)
	copy(input[32:], right)

	sum := sha256.Sum256(input[:])
	out := make([]byte, 32)
	copy(out, sum[:])
	return out
}

func floorLog2(n int) int {
	return bits.Len(uint(n)) - 1
}

func powerTwo(n int) int {
	return int(math.Pow(2, float64(n)))
}
