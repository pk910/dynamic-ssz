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
	"sync/atomic"

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

// NewNodeWithValue initializes a leaf node holding a copy of value. Copying is
// what makes the node independent of the caller's buffer: retaining the slice
// would let a later mutation of a reused scratch buffer silently change every
// tree and root built from it.
func NewNodeWithValue(value []byte) *Node {
	return newOwnedLeaf(bytes.Clone(value))
}

// newOwnedLeaf initializes a leaf node over a buffer the library owns and never
// hands back, so no defensive copy is needed.
func newOwnedLeaf(value []byte) *Node {
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
	depth, ok := hasher.GetZeroHashLevelBytes(n.value)
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
// cached hash and the subtree is read-only.
// A copy is returned for the same reason as in Value: empty (zero-padding)
// nodes alias the process-wide zero-hash table and cached empty nodes are
// shared across trees, so the raw slice must not escape to callers.
func (n *Node) Hash() []byte {
	// TODO: handle special cases: empty root, one non-empty node
	_ = n.finalize(finalizeConfig{}) // never fails without a custom hash function
	return bytes.Clone(hashNode(n))
}

// FinalizeOption configures Finalize.
type FinalizeOption func(*finalizeConfig)

type finalizeConfig struct {
	fn      hasher.HashFn
	workers int
}

// WithHashFn finalizes through the given hash function instead of the
// accelerated default backend, on every path — batched flushes and the
// recursive hashing of trees below the batching threshold alike. An error
// from fn aborts finalization and is returned by Finalize.
func WithHashFn(fn hasher.HashFn) FinalizeOption {
	return func(c *finalizeConfig) {
		c.fn = fn
	}
}

// WithAsyncHashing spreads each finalization batch across the given number
// of goroutines. The hash function must be safe for concurrent use.
// workers <= 1 keeps finalization on the calling goroutine.
func WithAsyncHashing(workers int) FinalizeOption {
	return func(c *finalizeConfig) {
		c.workers = workers
	}
}

// Finalize computes and caches every node hash in the subtree, so afterwards
// the subtree is read-only and safe for concurrent Prove/ProveMulti/Hash/
// Value calls. Hash, Prove and ProveMulti finalize implicitly with default
// options; Finalize is the explicit entry point for configuring the hash
// function or parallel hashing.
//
// A hashing backend failure aborts finalization and is returned; the tree
// is then partially finalized — every cached hash is valid — and a later
// Finalize resumes from it. The default backend cannot fail in practice (it
// rejects only malformed buffer sizes, which finalization never produces),
// so without WithHashFn, Finalize errors only on a malformed tree.
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

// finalizePendingMark marks a branch that has joined a batch: a non-nil
// zero-length value nothing else produces. A walk arriving at a marked node
// is revisiting an aliased subtree.
var finalizePendingMark = make([]byte, 0)

// finalizeScratch holds a finalize pass's reusable buffers: the per-depth
// pending batches and the sequential gather buffer. Both are bounded by the
// flush size, not the tree size; computed values are allocated per flush,
// since the nodes retain them.
type finalizeScratch struct {
	pending [][]*Node
	input   []byte
}

var finalizeScratchPool = sync.Pool{
	New: func() any { return new(finalizeScratch) },
}

// release drops all node references (a pooled scratch must not pin a tree)
// and returns the scratch to the pool, keeping slice capacities.
func (s *finalizeScratch) release() {
	for i := range s.pending {
		clear(s.pending[i])
		s.pending[i] = s.pending[i][:0]
	}
	finalizeScratchPool.Put(s)
}

// finalize computes and caches the value of every unhashed branch node in
// the subtree rooted at n. Hashing streams through the post-order walk:
// complete branches join their depth's pending batch, full batches flush
// deepest-first so a batch's children are always hashed before its gather.
// Already-hashed subtrees are pruned. A malformed node (one nil child) and
// its ancestors are left unhashed and reported. An aliased subtree is
// batched once: further batching stops at the revisit, and the remaining
// parents finish on the recursive path.
//
// A backend failure aborts finalization and returns the error; every cached
// value is intact, and a later finalize resumes from it.
func (n *Node) finalize(cfg finalizeConfig) error {
	// A cached root value certifies the whole subtree is finalized.
	if n == nil || n.value != nil || (n.left == nil && n.right == nil) {
		return nil
	}
	fn := cfg.fn
	if fn == nil {
		fn = batchHashFn
	}

	// Parallel flushes grow with the worker count so each share stays near
	// one finalizeBatchPairs.
	flushPairs := finalizeBatchPairs
	if cfg.workers > 1 {
		flushPairs = finalizeBatchPairs * min(cfg.workers, 4)
	}

	scratch, _ := finalizeScratchPool.Get().(*finalizeScratch)
	defer scratch.release()
	if cap(scratch.input) < flushPairs*64 {
		scratch.input = make([]byte, flushPairs*64)
	}

	var pool *finalizeAsync

	total := 0
	aliased := false

	var hashErr error

	// flushFrom hashes every pending batch at depth d or deeper, deepest
	// first: a pending node's unhashed children sit one depth deeper, and
	// joining that child slot before gathering guarantees their values are
	// assigned. With a pool, each batch is dispatched fire-and-forget and
	// joined lazily — usually on a later sweep, with the walk continuing in
	// between.
	flushFrom := func(d int) {
		for dd := len(scratch.pending) - 1; dd >= d; dd-- {
			batch := scratch.pending[dd]
			if len(batch) == 0 {
				continue
			}
			if pool == nil {
				out := make([]byte, len(batch)*32)
				if err := hashBatch(batch, out, scratch.input[:len(batch)*64], fn); err != nil {
					hashErr = err
					return
				}
				clear(batch)
				scratch.pending[dd] = batch[:0]
				continue
			}
			if err := pool.join(dd + 1); err != nil {
				hashErr = err
				return
			}
			if err := pool.join(dd); err != nil {
				hashErr = err
				return
			}
			out := make([]byte, len(batch)*32)
			pool.dispatch(dd, batch, out, pool.getInput(len(batch)*64), fn)
			scratch.pending[dd] = pool.getSpare(flushPairs)
		}
	}

	// walk reports whether the subtree is fully hashed once pending batches
	// flush; only nodes whose children both are get batched. After a backend
	// failure the walk keeps traversing for that verdict but stops batching.
	var walk func(node *Node, depth int) bool
	walk = func(node *Node, depth int) bool {
		if node.left == nil && node.right == nil {
			return true
		}
		if node.value != nil {
			// A pending mark means an aliased subtree: it hashes once through
			// its existing batch, and batching stops so no parent gathers it
			// from a depth the flush ordering does not serve. Values grow to
			// full length only at a join, so a full-length value is final.
			if len(node.value) == 0 {
				aliased = true
			}
			return true
		}
		if node.left == nil || node.right == nil {
			return false
		}

		leftOk := walk(node.left, depth+1)
		rightOk := walk(node.right, depth+1)

		if !leftOk || !rightOk {
			return false
		}
		total++
		if hashErr != nil || aliased {
			return true
		}
		for len(scratch.pending) <= depth {
			scratch.pending = append(scratch.pending, nil)
		}
		if cap(scratch.pending[depth]) < flushPairs {
			scratch.pending[depth] = append(make([]*Node, 0, flushPairs), scratch.pending[depth]...)
		}
		node.value = finalizePendingMark
		scratch.pending[depth] = append(scratch.pending[depth], node)
		if len(scratch.pending[depth]) >= flushPairs {
			// The tree is large enough for mid-walk flushing; async hashing
			// spreads each flush across a worker pool.
			if pool == nil && cfg.workers > 1 {
				pool = &finalizeAsync{workers: cfg.workers}
			}
			flushFrom(depth)
		}

		return true
	}
	rootOk := walk(n, 0)

	if hashErr == nil && (total >= finalizeThreshold || aliased) {
		flushFrom(0)
	}
	if pool != nil {
		if err := pool.joinAll(); err != nil && hashErr == nil {
			hashErr = err
		}
	}
	// Batches that never flushed still carry pending marks; restore those
	// nodes to unhashed so the recursive path recomputes them.
	for _, batch := range scratch.pending {
		for _, node := range batch {
			if len(node.value) == 0 {
				node.value = nil
			}
		}
	}
	if hashErr != nil {
		return hashErr
	}
	// Small trees hash recursively (batch setup costs more than it saves),
	// as do the parents excluded after an aliasing stop.
	if (aliased || total < finalizeThreshold) && total > 0 && rootOk {
		if cfg.fn != nil {
			_, err := hashNodeFn(n, cfg.fn)
			return err
		}
		hashNode(n)
	}
	if !rootOk {
		return errors.New("tree is malformed: branch with a single nil child")
	}
	return nil
}

// hashNodeFn is the recursive finalization path for a caller-supplied hash
// function: it computes what hashNode computes, but through fn, and
// propagates fn's error instead of substituting the built-in hash. A value
// is only cached after a successful fn call, so an errored pass leaves a
// consistent, resumable tree.
func hashNodeFn(n *Node, fn hasher.HashFn) ([]byte, error) {
	if n == nil {
		return nil, errors.New("tree is malformed: nil node")
	}
	if n.left == nil && n.right == nil {
		return n.value, nil
	}
	if n.value != nil {
		return n.value, nil
	}
	if n.left == nil || n.right == nil {
		return nil, errors.New("tree is malformed: branch with a single nil child")
	}

	left, err := hashNodeFn(n.left, fn)
	if err != nil {
		return nil, err
	}
	right, err := hashNodeFn(n.right, fn)
	if err != nil {
		return nil, err
	}

	var input [64]byte
	copy(input[:32], left)
	copy(input[32:], right)
	out := make([]byte, 32)
	if err := fn(out, input[:]); err != nil {
		return nil, err
	}
	n.value = out
	return out, nil
}

// hashBatch gathers the sibling pairs of batch into input, compresses them
// in one batchHashFn call, and hands out the results as sub-slices of out.
// input must hold exactly len(batch) pairs; its prior contents are arbitrary
// (buffers are reused), so short values zero-extend their chunk explicitly,
// matching hashPair.
func hashBatch(batch []*Node, out, input []byte, fn hasher.HashFn) error {
	if err := hashBatchInto(batch, out, input, fn); err != nil {
		return err
	}
	for i, node := range batch {
		node.value = out[i*32 : (i+1)*32 : (i+1)*32]
	}
	return nil
}

// hashBatchInto is hashBatch without the value-header assignment, for
// worker-pool shares: the pool assigns every header on the calling
// goroutine once the whole batch hashed.
func hashBatchInto(batch []*Node, out, input []byte, fn hasher.HashFn) error {
	for i, node := range batch {
		off := i * 64
		n := copy(input[off:off+32], node.left.value)
		clear(input[off+n : off+32])
		n = copy(input[off+32:off+64], node.right.value)
		clear(input[off+32+n : off+64])
	}
	return fn(out[:len(batch)*32], input)
}

// finalizeAsync is one finalize call's async-flush state. Every flush is
// dispatched fire-and-forget as per-worker shares and parked in its depth's
// slot; joining a slot — always on the walking goroutine — collects the
// shares and assigns the nodes' value headers, so headers have a single
// writer and batched nodes keep their pending marks until their hash is
// final. The flush sweep joins a depth's child slot before gathering from
// it, which is the only ordering the batches need. Shares execute on the
// process-wide worker pool shared by all concurrent finalizations.
type finalizeAsync struct {
	workers int

	slots  []finalizeSlot
	inputs [][]byte  // gather-buffer freelist, recycled at join
	spares [][]*Node // batch-slice freelist, recycled at join
}

// finalizeHandoff passes flush shares to the persistent worker goroutines.
// It is never closed — finalize calls come and go while the workers
// persist, so concurrent calls share one pool instead of stacking
// goroutines.
var finalizeHandoff = make(chan finalizeShare, 64)

// finalizeWorkers counts the persistent worker goroutines and finalizeIdle
// the ones parked on the handoff channel.
var (
	finalizeWorkers atomic.Int64
	finalizeIdle    atomic.Int64
)

// ensureFinalizeWorker starts another persistent worker unless one is idle
// or limit is reached. The idle check is approximate: in the worst case a
// share waits for a busy worker, and the next dispatch tops the pool up.
func ensureFinalizeWorker(limit int) {
	if finalizeIdle.Load() > 0 {
		return
	}
	for {
		cur := finalizeWorkers.Load()
		if cur >= int64(limit) {
			return
		}
		if finalizeWorkers.CompareAndSwap(cur, cur+1) {
			go finalizeShareWorker()
			return
		}
	}
}

// finalizeShareWorker is a persistent worker goroutine: it gathers and
// hashes handed-off shares and reports each result on the share's channel.
func finalizeShareWorker() {
	for {
		finalizeIdle.Add(1)
		share := <-finalizeHandoff
		finalizeIdle.Add(-1)
		share.res <- hashBatchInto(share.batch, share.out, share.input, share.fn)
	}
}

// finalizeShare is one worker's slice of a flush: disjoint sub-ranges of
// the batch and its gather/output buffers.
type finalizeShare struct {
	batch      []*Node
	out, input []byte
	fn         hasher.HashFn
	res        chan<- error
}

// finalizeSlot parks one dispatched batch per depth until its join.
type finalizeSlot struct {
	batch  []*Node
	out    []byte
	input  []byte
	res    chan error
	shares int
}

// finalizeShareMinPairs is the smallest share worth handing to a worker;
// below it, the fan-out overhead outweighs the parallel hashing.
const finalizeShareMinPairs = 64

// getInput returns a gather buffer of at least size bytes.
func (p *finalizeAsync) getInput(size int) []byte {
	if n := len(p.inputs); n > 0 {
		buf := p.inputs[n-1]
		p.inputs = p.inputs[:n-1]
		if cap(buf) >= size {
			return buf[:size]
		}
	}
	return make([]byte, size)
}

// getSpare returns an empty batch slice to replace a dispatched one.
func (p *finalizeAsync) getSpare(capacity int) []*Node {
	if n := len(p.spares); n > 0 {
		batch := p.spares[n-1]
		p.spares = p.spares[:n-1]
		if cap(batch) >= capacity {
			return batch
		}
	}
	return make([]*Node, 0, capacity)
}

// dispatch fans batch out to the workers and parks it in depth's slot; the
// batch, out and input buffers stay owned by the shares until the join.
func (p *finalizeAsync) dispatch(depth int, batch []*Node, out, input []byte, fn hasher.HashFn) {
	for len(p.slots) <= depth {
		p.slots = append(p.slots, finalizeSlot{})
	}
	shares := max(min(p.workers, len(batch)/finalizeShareMinPairs), 1)
	span := (len(batch) + shares - 1) / shares
	res := make(chan error, shares)
	for s := 0; s < shares; s++ {
		lo := s * span
		hi := min(lo+span, len(batch))
		ensureFinalizeWorker(p.workers)
		finalizeHandoff <- finalizeShare{batch: batch[lo:hi], out: out[lo*32 : hi*32], input: input[lo*64 : hi*64], fn: fn, res: res}
	}
	p.slots[depth] = finalizeSlot{batch: batch, out: out, input: input, res: res, shares: shares}
}

// join completes the slot at depth: collect every share, then assign the
// value headers — or, when a share failed, clear the nodes' pending marks
// so they read as unhashed. Buffers recycle into the freelists.
func (p *finalizeAsync) join(depth int) error {
	if depth >= len(p.slots) || p.slots[depth].batch == nil {
		return nil
	}
	slot := p.slots[depth]
	p.slots[depth] = finalizeSlot{}
	var err error
	for range slot.shares {
		if shareErr := <-slot.res; shareErr != nil && err == nil {
			err = shareErr
		}
	}
	if err == nil {
		for i, node := range slot.batch {
			node.value = slot.out[i*32 : (i+1)*32 : (i+1)*32]
		}
	} else {
		for _, node := range slot.batch {
			node.value = nil
		}
	}
	clear(slot.batch)
	p.inputs = append(p.inputs, slot.input)
	p.spares = append(p.spares, slot.batch[:0])
	return err
}

// joinAll completes every outstanding slot and reports the first error.
func (p *finalizeAsync) joinAll() error {
	var err error
	for depth := len(p.slots) - 1; depth >= 0; depth-- {
		if joinErr := p.join(depth); joinErr != nil && err == nil {
			err = joinErr
		}
	}
	return err
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
			siblingHash = child.value
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
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint64(buf[:8], i)
	return newOwnedLeaf(buf)
}

// LeafFromUint32 creates a 32-byte leaf node from a uint32 value, encoded as
// little-endian in the first 4 bytes with the remaining 28 bytes zero-padded.
func LeafFromUint32(i uint32) *Node {
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint32(buf[:4], i)
	return newOwnedLeaf(buf)
}

// LeafFromUint16 creates a 32-byte leaf node from a uint16 value, encoded as
// little-endian in the first 2 bytes with the remaining 30 bytes zero-padded.
func LeafFromUint16(i uint16) *Node {
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint16(buf[:2], i)
	return newOwnedLeaf(buf)
}

// LeafFromUint8 creates a 32-byte leaf node from a uint8 value, stored in the
// first byte with the remaining 31 bytes zero-padded.
func LeafFromUint8(i uint8) *Node {
	buf := make([]byte, 32)
	buf[0] = i
	return newOwnedLeaf(buf)
}

// LeafFromBool creates a 32-byte leaf node from a boolean value, encoded as
// 0x01 (true) or 0x00 (false) in the first byte with 31 bytes zero-padded.
func LeafFromBool(b bool) *Node {
	buf := make([]byte, 32)
	if b {
		buf[0] = 1
	}
	return newOwnedLeaf(buf)
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
		// The three-index cap keeps the zero padding out of the caller's
		// backing array; input memory must never be mutated.
		return newOwnedLeaf(append(b[:l:l], sszutils.ZeroBytes()[:32-l]...))
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
			leaves[i] = newOwnedLeaf(chunk)
		}
	}

	// limit is a power of two, so TreeFromNodes never returns an error here.
	node, _ := TreeFromNodes(leaves, int(sszutils.NextPowerOfTwo(uint64(numChunks))))
	return node
}

// EmptyLeaf creates a leaf node containing 32 zero bytes, representing an
// empty or unset value in the Merkle tree.
func EmptyLeaf() *Node {
	return newOwnedLeaf(sszutils.ZeroBytes()[:32])
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
		leaves[i] = newOwnedLeaf(v)
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
