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
	"slices"
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
// Each node is either a leaf node (holding 32 bytes of data) or a branch node
// whose value is the hash of its children, cached by finalization. The tree
// structure follows SSZ merkleization rules and supports both binary and
// progressive tree layouts.
//
// The 32-byte value lives inline and is valid only while hasValue is set:
// always for leaves, and from finalization on for branches. isEmpty marks
// "zero" nodes used for padding incomplete trees. isVisited is scheduling
// state owned by the finalization walk: set while the node waits in a
// pending hash batch, cleared when its value is written.
type Node struct {
	left      *Node    // Left child node (nil for leaves)
	right     *Node    // Right child node (nil for leaves)
	value     [32]byte // Leaf data or cached branch hash, valid iff hasValue
	hasValue  bool     // True once value holds the leaf data or branch hash
	isEmpty   bool     // True if this is a zero-padding node
	isVisited bool     // True while pending in a finalization batch
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
	} else if n.hasValue {
		// Leaf node - show value only (no hash for leaves)
		printNode("VALUE: " + hex.EncodeToString(n.value[:]) + "\n")
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

// NewNodeWithValue initializes a leaf node holding a copy of value, zero
// padded to the full 32-byte chunk. Copying is what makes the node
// independent of the caller's buffer: retaining a reference would let a
// later mutation of a reused scratch buffer silently change every tree and
// root built from it. A value longer than 32 bytes panics — a leaf cannot
// hold it, and truncating would silently corrupt every root built from the
// node; use LeafFromBytes to merkleize longer input into a subtree.
func NewNodeWithValue(value []byte) *Node {
	if len(value) > 32 {
		panic(fmt.Sprintf("NewNodeWithValue: value length %d exceeds the 32-byte leaf chunk", len(value)))
	}
	return newLeaf(value)
}

// newLeaf initializes a leaf node from up to 32 bytes of value, zero padded.
func newLeaf(value []byte) *Node {
	n := &Node{hasValue: true}
	copy(n.value[:], value)
	n.isEmpty = n.value == [32]byte{}
	return n
}

// NewEmptyNode creates an empty (zero-padding) tree node with the given
// precomputed zero-order hash. Empty nodes represent unused positions in the
// binary tree and are marked with isEmpty=true for efficient proof compression.
func NewEmptyNode(zeroOrderHash []byte) *Node {
	n := &Node{hasValue: true, isEmpty: true}
	copy(n.value[:], zeroOrderHash)
	return n
}

// NewNodeWithLR initializes a branch node.
func NewNodeWithLR(left, right *Node) *Node {
	return &Node{left: left, right: right}
}

// TreeFromChunks constructs a tree from leaf values.
// The number of leaves should be a power of 2, and every chunk must be exactly
// 32 bytes: hashPair copies each side into a fixed 64-byte block, so a shorter
// chunk would be zero-extended and a longer one truncated. Either way distinct
// inputs would collide, and a single short chunk would make Node.Hash() return
// fewer than the 32 bytes it documents.
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
		if len(c) != 32 {
			return nil, fmt.Errorf("chunk %d has length %d, want 32", i, len(c))
		}
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
	// A non-positive limit is either a true zero capacity or the int-overflow
	// artifact of a chunk limit above the platform int max (only reachable on
	// 32-bit). Either way an int cannot represent a usable capacity, so any
	// leaves present would be silently dropped, producing a valid-looking root
	// for a different tree. Reject that; callers with such capacities use the
	// uint64 TreeFromNodes64 form directly.
	if limit <= 0 {
		if len(leaves) > 0 {
			return nil, fmt.Errorf("number of leaves %d exceeds limit %d", len(leaves), limit)
		}
		return getEmptyNode(0), nil
	}
	if len(leaves) > limit {
		return nil, fmt.Errorf("number of leaves %d exceeds limit %d", len(leaves), limit)
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
	count := uint64(len(leaves))
	if limit == 0 {
		// No limit: the tree is exactly as deep as the leaves require.
		limit = count
	}

	// A limit below the leaf count describes a value that overflows its own
	// type. The Hasher keeps the depth the limit asks for and lets the surplus
	// chunks fall outside the tree, which leaves the root of the leaves that do
	// fit; mirror that so the Wrapper stays a drop-in HashWalker producing the
	// same root for the same call sequence.
	depth := chunkLimitDepth(limit)
	if depth < 63 {
		if capacity := uint64(1) << uint(depth); count > capacity {
			leaves = leaves[:capacity]
		}
	}

	mainTree, err := treeFromNodesToDepthFn(leaves, depth)
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

// emptyChild returns the zero-padding child one level below an empty node.
// Both children of an empty node are the same zero subtree, so direction does
// not matter. It reports false when the node is already the depth-0 zero leaf
// (a gindex descending past it lies outside the tree's depth).
func emptyChild(n *Node) (*Node, bool) {
	depth, ok := hasher.GetZeroHashLevelBytes(n.value[:])
	if !ok || depth < 1 {
		return nil, false
	}
	return getEmptyNode(depth - 1), true
}

// Get fetches a node with the given general index.
func (n *Node) Get(index int) (*Node, error) {
	if index < 1 {
		return nil, errors.New("Node not found in tree")
	}
	pathLen := getPathLength(index)
	cur := n
	for i := pathLen - 1; i >= 0; i-- {
		if cur.isEmpty {
			// Descending into a zero-padding subtree: synthesize the zero
			// subtree one level down so spec-valid gindices under the padding
			// (proof-of-emptiness) resolve instead of failing.
			child, ok := emptyChild(cur)
			if !ok {
				return nil, errors.New("Node not found in tree")
			}
			cur = child
			continue
		}
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
// Hash finalizes the subtree, so afterwards every branch node holds its
// cached hash and the subtree is read-only. It panics on an incomplete
// tree; Finalize reports that as an error instead.
// A copy is returned for the same reason as in Value: cached empty
// (zero-padding) nodes are shared across trees, so the raw bytes must not
// escape to callers.
func (n *Node) Hash() []byte {
	// The malformed-tree error is deliberately dropped: hashNode below
	// reports the incomplete tree through its documented panic.
	_ = n.finalize(finalizeConfig{})
	return bytes.Clone(hashNode(n))
}

// FinalizeOption configures Finalize.
type FinalizeOption func(*finalizeConfig)

type finalizeConfig struct {
	fn hasher.HashFn
}

// WithHashFn finalizes through the given hash function instead of the
// accelerated default backend, on every path — batched flushes and the
// recursive hashing of trees below the batching threshold alike. An error
// from fn aborts finalization and is returned by Finalize. fn must support
// in-place hashing: dst aliases the front of input.
func WithHashFn(fn hasher.HashFn) FinalizeOption {
	return func(c *finalizeConfig) {
		c.fn = fn
	}
}

// Finalize computes and caches every node hash in the subtree, so afterwards
// the subtree is read-only and safe for concurrent Prove/ProveMulti/Hash/
// Value calls. Hash, Prove and ProveMulti finalize implicitly with default
// options; Finalize is the explicit entry point for configuring the hash
// function.
//
// Any error — a hashing backend failure or a malformed tree (a branch with
// a single nil child) — aborts finalization immediately and is returned.
// Every hash cached before the abort is valid, so a later Finalize resumes
// from it. The default backend cannot fail in practice (it rejects only
// malformed buffer sizes, which finalization never produces), so without
// WithHashFn, Finalize errors only on a malformed tree.
func (n *Node) Finalize(opts ...FinalizeOption) error {
	cfg := finalizeConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return n.finalize(cfg)
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

// Value returns a copy of the node's 32-byte value — leaf data or a
// finalized branch hash — or nil for a branch that has not been hashed yet.
// A copy is returned because cached empty (zero-padding) nodes are shared
// across trees; handing out a mutable reference would let a caller's
// mutation corrupt every other tree and root.
func (n *Node) Value() []byte {
	if !n.hasValue {
		return nil
	}
	return bytes.Clone(n.value[:])
}

func getEmptyNode(depth int) *Node {
	emptyNodeInit.Do(func() {
		for i := range emptyNodeCache {
			emptyNodeCache[i] = NewEmptyNode(hasher.GetZeroHash(i))
		}
	})
	return emptyNodeCache[depth]
}

// batchHashFn compresses a packed sequence of 64-byte sibling pairs into
// 32-byte parent hashes in a single call, using the vectorized hashtree
// backend when available.
var batchHashFn = hasher.FastHasherPool.HashFn

// finalizeThreshold is the unhashed-branch count below which finalize hashes
// recursively: tiny batches pay more in buffer setup and per-batch backend
// calls than the vectorized hashing saves.
const finalizeThreshold = 16

// finalizeBatchPairs is the number of sibling pairs accumulated per depth
// before a batch flushes: small enough to gather cache-warm values, large
// enough to amortize the backend call.
const finalizeBatchPairs = 1024

// finalizeScratch holds a finalize pass's reusable buffers: the per-depth
// pending batches and the shared buffer that gathered pairs are hashed in.
// The buffer is bounded by the flush size; pending storage is bounded per
// depth, so O(depth·finalizeBatchPairs) overall — independent of the node
// count for balanced trees.
type finalizeScratch struct {
	pending [][]*Node
	buf     []byte // finalizeBatchPairs*64 bytes, lazily allocated on first flush
}

var finalizeScratchPool = sync.Pool{
	New: func() any { return new(finalizeScratch) },
}

// release clears the isVisited scheduling flag on every node still pending
// (batches only stay pending when finalization aborted), drops all node
// references (a pooled scratch must not pin a tree) and returns the scratch
// to the pool, keeping slice capacities.
func (s *finalizeScratch) release() {
	for i := range s.pending {
		for _, node := range s.pending[i] {
			node.isVisited = false
		}
		clear(s.pending[i])
		s.pending[i] = s.pending[i][:0]
	}
	finalizeScratchPool.Put(s)
}

// finalize computes and caches the value of every unhashed branch node in
// the subtree rooted at n. Hashing streams through the post-order walk:
// complete branches join their depth's pending batch, full batches flush
// deepest-first so a batch's children are always hashed before its gather.
// Already-hashed subtrees are pruned. Any error — a backend failure or a
// branch with a single nil child — aborts the walk immediately and is
// returned; values are only written from successful hashes, so the tree
// stays consistent and a later finalize resumes from the cached values.
//
// A node met again while it is still pending (isVisited) means the tree is
// a DAG of shared subtrees: all pending batches flush on the spot, which
// hashes the shared node once and caches its value, and the walk continues
// batching the rest of the tree against that cache. Flushing mid-walk is
// safe at any point: pending nodes are completed subtrees — never ancestors
// of the walk position — and their unhashed children always sit one depth
// deeper, so the deepest-first flush order holds.
func (n *Node) finalize(cfg finalizeConfig) error {
	// A cached root value certifies the whole subtree is finalized.
	if n == nil || n.hasValue || (n.left == nil && n.right == nil) {
		return nil
	}
	fn := cfg.fn
	if fn == nil {
		fn = batchHashFn
	}

	scratch, _ := finalizeScratchPool.Get().(*finalizeScratch)
	defer scratch.release()

	total := 0
	malformed := false

	var hashErr error

	// flushFrom hashes every pending batch at depth d or deeper, deepest
	// first: a pending node's unhashed children sit one depth deeper, so
	// this order guarantees their values are cached before the gather.
	flushFrom := func(d int) {
		for dd := len(scratch.pending) - 1; dd >= d; dd-- {
			batch := scratch.pending[dd]
			if len(batch) == 0 {
				continue
			}
			if scratch.buf == nil {
				scratch.buf = make([]byte, finalizeBatchPairs*64)
			}
			if err := hashBatch(batch, scratch.buf, fn); err != nil {
				hashErr = err
				return
			}
			clear(batch)
			scratch.pending[dd] = batch[:0]
		}
	}

	// walk batches every complete unhashed branch in post order, aborting on
	// the first alias, malformed node or flush failure; the abort conditions
	// are re-checked after each child recursion to unwind without batching.
	var walk func(node *Node, depth int)
	walk = func(node *Node, depth int) {
		if node.hasValue || (node.left == nil && node.right == nil) {
			return
		}
		if node.isVisited {
			// The node is pending in a batch: the tree is a DAG sharing this
			// subtree. Flushing everything hashes it once and caches its
			// value; the walk continues batching against the cache.
			flushFrom(0)
			if hashErr != nil || node.hasValue {
				return
			}
			// The flush did not produce a value, so the flag is stale (the
			// node is not pending in any batch): scrub it and walk the
			// subtree normally.
			node.isVisited = false
		}
		if node.left == nil || node.right == nil {
			malformed = true
			return
		}

		walk(node.left, depth+1)
		if malformed || hashErr != nil {
			return
		}
		walk(node.right, depth+1)
		if malformed || hashErr != nil {
			return
		}

		total++
		node.isVisited = true
		for len(scratch.pending) <= depth {
			scratch.pending = append(scratch.pending, nil)
		}
		scratch.pending[depth] = append(scratch.pending[depth], node)
		if len(scratch.pending[depth]) >= finalizeBatchPairs {
			flushFrom(depth)
		}
	}
	walk(n, 0)

	switch {
	case hashErr != nil:
		return hashErr
	case malformed:
		return errors.New("tree is malformed: branch with a single nil child")
	case total < finalizeThreshold:
		// Small trees hash recursively: batch setup costs more than the
		// vectorized backend saves.
		return hashNodeFn(n, fn)
	default:
		flushFrom(0)
		return hashErr
	}
}

// hashNodeFn is the recursive finalization tail, used for trees below the
// batching threshold and for the remainder of an aliased tree. It computes
// and caches the hash of every unhashed branch through fn, pruning at
// cached values, so shared subtrees hash once and the pass stays linear on
// DAGs. A value is only cached after a successful fn call, so an errored
// pass leaves a consistent, resumable tree.
func hashNodeFn(n *Node, fn hasher.HashFn) error {
	if n == nil {
		return errors.New("tree is malformed: nil node")
	}
	if n.hasValue || (n.left == nil && n.right == nil) {
		return nil
	}
	if n.left == nil || n.right == nil {
		return errors.New("tree is malformed: branch with a single nil child")
	}

	if err := hashNodeFn(n.left, fn); err != nil {
		return err
	}
	if err := hashNodeFn(n.right, fn); err != nil {
		return err
	}

	var pair [64]byte
	copy(pair[:32], n.left.value[:])
	copy(pair[32:], n.right.value[:])
	if err := fn(pair[:32], pair[:]); err != nil {
		return err
	}
	copy(n.value[:], pair[:32])
	n.hasValue = true
	n.isVisited = false
	return nil
}

// hashBatch gathers the sibling pairs of batch into buf, compresses them in
// place through one fn call (the output overwrites the front of the gathered
// input), and scatters the results into the nodes, marking each node hashed
// and no longer pending. buf must hold len(batch) 64-byte pairs.
func hashBatch(batch []*Node, buf []byte, fn hasher.HashFn) error {
	for i, node := range batch {
		off := i * 64
		copy(buf[off:off+32], node.left.value[:])
		copy(buf[off+32:off+64], node.right.value[:])
	}
	if err := fn(buf[:len(batch)*32], buf[:len(batch)*64]); err != nil {
		return err
	}
	for i, node := range batch {
		copy(node.value[:], buf[i*32:(i+1)*32])
		node.hasValue = true
		node.isVisited = false
	}
	return nil
}

// hashNode resolves a node's 32-byte value: leaf data, a cached branch
// hash, or — for a branch not yet finalized — the recursively computed hash,
// which is cached on the node. It panics on an incomplete tree (a nil node
// or a branch with a single nil child); the error-reporting paths run
// through finalize instead.
func hashNode(n *Node) []byte {
	if n == nil {
		panic("Tree incomplete")
	}
	if n.hasValue || (n.left == nil && n.right == nil) {
		return n.value[:]
	}
	if n.left == nil || n.right == nil {
		panic("Tree incomplete")
	}

	var input [64]byte
	copy(input[:32], hashNode(n.left))
	copy(input[32:], hashNode(n.right))
	n.value = sha256.Sum256(input[:])
	n.hasValue = true
	return n.value[:]
}

// Prove returns a list of sibling values and hashes needed
// to compute the root hash for a given general index.
//
// Thread-safety: the first Prove/ProveMulti/Hash call on a freshly built tree
// finalizes it (computes and caches all node hashes) and must not run
// concurrently with other calls on the same tree. A finalized tree is
// read-only, so concurrent calls on it are safe.
func (n *Node) Prove(index int) (*Proof, error) {
	if index < 1 {
		return nil, fmt.Errorf("invalid generalized index %d (must be >= 1)", index)
	}
	if err := n.finalize(finalizeConfig{}); err != nil {
		return nil, err
	}
	pathLen := getPathLength(index)
	proof := &Proof{Index: index}
	hashes := make([][]byte, 0, pathLen)

	cur := n
	for i := pathLen - 1; i >= 0; i-- {
		var siblingHash []byte
		switch {
		case cur.isEmpty:
			// Zero-padding subtree: both children are the same zero subtree, so
			// the sibling hash is the zero hash at that level. Synthesizing it
			// lets spec-valid gindices under the padding be proven.
			child, ok := emptyChild(cur)
			if !ok {
				return nil, errors.New("Node not found in tree")
			}
			siblingHash = child.value[:]
			cur = child
		case getPosAtLevel(index, i):
			if cur.left == nil {
				return nil, errors.New("Node not found in tree")
			}
			siblingHash = hashNode(cur.left)
			cur = cur.right
		default:
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
	proof.Leaf = bytes.Clone(hashNode(cur))

	return proof, nil
}

// ProveMulti generates a Multiproof for the given set of generalized indices.
// It collects the leaf values at each index and the minimal set of auxiliary
// hashes needed to reconstruct the root. Returns an error if any index cannot
// be found in the tree.
//
// Thread-safety: like Prove, this finalizes the tree on first use, so the
// first call must not run concurrently with other calls on the same tree
// (see Prove).
func (n *Node) ProveMulti(indices []int) (*Multiproof, error) {
	for _, gi := range indices {
		if gi < 1 {
			return nil, fmt.Errorf("invalid generalized index %d (must be >= 1)", gi)
		}
	}
	if err := n.finalize(finalizeConfig{}); err != nil {
		return nil, err
	}
	reqIndices := getRequiredIndices(indices)
	// Indices is cloned like Leaves and Hashes: storing the caller's slice by
	// reference lets a later reorder or reuse of it silently invalidate a proof
	// that already verified.
	proof := &Multiproof{Indices: slices.Clone(indices), Leaves: make([][]byte, len(indices)), Hashes: make([][]byte, len(reqIndices))}

	// Copy leaf and hash values: empty nodes alias the shared zero-hash table and
	// cached empty nodes are shared across trees, so the returned proof must own
	// its bytes to stay mutation-safe.
	for i, gi := range indices {
		node, err := n.Get(gi)
		if err != nil {
			return nil, err
		}
		// hashNode, not node.value: a branch node that has not been hashed yet
		// carries a nil value, which would silently produce an empty leaf. Prove
		// already resolves it this way.
		proof.Leaves[i] = bytes.Clone(hashNode(node))
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
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], i)
	return newLeaf(buf[:])
}

// LeafFromUint32 creates a 32-byte leaf node from a uint32 value, encoded as
// little-endian in the first 4 bytes with the remaining 28 bytes zero-padded.
func LeafFromUint32(i uint32) *Node {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], i)
	return newLeaf(buf[:])
}

// LeafFromUint16 creates a 32-byte leaf node from a uint16 value, encoded as
// little-endian in the first 2 bytes with the remaining 30 bytes zero-padded.
func LeafFromUint16(i uint16) *Node {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], i)
	return newLeaf(buf[:])
}

// LeafFromUint8 creates a 32-byte leaf node from a uint8 value, stored in the
// first byte with the remaining 31 bytes zero-padded.
func LeafFromUint8(i uint8) *Node {
	return newLeaf([]byte{i})
}

// LeafFromBool creates a 32-byte leaf node from a boolean value, encoded as
// 0x01 (true) or 0x00 (false) in the first byte with 31 bytes zero-padded.
func LeafFromBool(b bool) *Node {
	if b {
		return newLeaf([]byte{1})
	}
	return newLeaf(nil)
}

// LeafFromBytes creates a tree node from a byte slice. A slice of up to 32
// bytes becomes a single leaf, right-padded with zeros, and a longer slice is
// split into 32-byte chunks that are merkleized into a subtree so the node
// still hashes to a single 32-byte root.
func LeafFromBytes(b []byte) *Node {
	l := len(b)
	if l <= 32 {
		return newLeaf(b)
	}

	numChunks := (l + 31) / 32
	leaves := make([]*Node, numChunks)
	for i := range leaves {
		start := i * 32
		leaves[i] = newLeaf(b[start:min(start+32, l)])
	}

	// limit is a power of two, so TreeFromNodes never returns an error here.
	node, _ := TreeFromNodes(leaves, int(sszutils.NextPowerOfTwo(uint64(numChunks))))
	return node
}

// EmptyLeaf creates a leaf node containing 32 zero bytes, representing an
// empty or unset value in the Merkle tree.
func EmptyLeaf() *Node {
	return &Node{hasValue: true, isEmpty: true}
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
		leaves[i] = newLeaf(buf[i*32 : (i+1)*32])
	}

	return leaves
}

func isPowerOfTwo(n int) bool {
	return (n & (n - 1)) == 0
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
