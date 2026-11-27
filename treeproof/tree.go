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
// See `CompressedMultiproof` for more info. The zero hashes are those of the
// Algorithm named with WithAlgorithm, which Decompress has to be given too.
func (p *Multiproof) Compress(opts ...Option) *CompressedMultiproof {
	alg := algorithmOf(opts)
	// The compressed proof owns its data, so editing one proof never edits
	// the other.
	compressed := &CompressedMultiproof{
		Indices:    slices.Clone(p.Indices),
		Leaves:     cloneChunks(p.Leaves),
		Hashes:     make([][]byte, 0, len(p.Hashes)),
		ZeroLevels: make([]int, 0, len(p.Hashes)),
	}

	for _, h := range p.Hashes {
		if l, ok := alg.ZeroHashLevel(h); ok {
			compressed.ZeroLevels = append(compressed.ZeroLevels, l)
			compressed.Hashes = append(compressed.Hashes, nil)
		} else {
			compressed.Hashes = append(compressed.Hashes, bytes.Clone(h))
		}
	}

	return compressed
}

// cloneChunks copies a slice of byte slices and every byte slice in it.
func cloneChunks(src [][]byte) [][]byte {
	if src == nil {
		return nil
	}
	out := make([][]byte, len(src))
	for i, chunk := range src {
		out[i] = bytes.Clone(chunk)
	}
	return out
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
func (c *CompressedMultiproof) Decompress(opts ...Option) *Multiproof {
	alg := algorithmOf(opts)
	// The decompressed proof owns its data, so editing one proof never edits
	// the other.
	p := &Multiproof{
		Indices: slices.Clone(c.Indices),
		Leaves:  cloneChunks(c.Leaves),
		Hashes:  make([][]byte, len(c.Hashes)),
	}

	zc := 0
	for i, h := range c.Hashes {
		if h == nil {
			level := 0
			if zc < len(c.ZeroLevels) {
				level = c.ZeroLevels[zc]
			}
			p.Hashes[i] = alg.ZeroHash(level)
			zc++
		} else {
			p.Hashes[i] = bytes.Clone(c.Hashes[i])
		}
	}

	return p
}

// node is a node of a Merkle tree: a leaf holding 32 bytes of data, a branch
// whose value is the hash of its children once the tree is hashed, or a
// zero-padding node standing for a zero subtree of some depth.
//
// A node knows nothing about the hash of its tree. The value of a branch is
// valid only while hasValue is set, and belongs to the Algorithm the tree was
// hashed with; a zero-padding node holds no value at all, since its root
// depends on the hash. Whatever hashes a tree or reads a root from it brings
// the Algorithm along (see hashing).
//
// isVisited is scheduling state owned by the finalization walk: set while the
// node waits in a pending hash batch, cleared when its value is written.
type node struct {
	left      *node    // Left child node (nil for leaves)
	right     *node    // Right child node (nil for leaves)
	value     [32]byte // Leaf data or cached branch hash, valid iff hasValue
	hasValue  bool     // True once value holds the leaf data or branch hash
	isEmpty   bool     // True if this is a zero-padding node
	isVisited bool     // True while pending in a finalization batch
	depth     uint8    // Depth of the zero subtree a zero-padding node stands for
}

// emptyNodes are the zero-padding nodes, one per depth. They carry no value,
// so every tree of every hash shares them.
var emptyNodes = func() (nodes [65]node) {
	for depth := range nodes {
		nodes[depth] = node{isEmpty: true, depth: uint8(depth)}
	}
	return nodes
}()

// emptyNode returns the zero-padding node of the given depth.
func emptyNode(depth int) *node {
	return &emptyNodes[depth]
}

// hashing is what one operation hashes a tree with, or reads its roots with:
// the function and the zero subtree roots of the tree's Algorithm. It is
// resolved once per operation and is the only way a hash or a zero subtree
// root enters this package.
type hashing struct {
	fn   hasher.HashFn
	zero *[65][32]byte
}

// hashingOf returns the hashing of the given Algorithm.
func hashingOf(alg *hasher.Algorithm) hashing {
	return hashing{fn: alg.HashFn(), zero: alg.ZeroHashTable()}
}

// value returns the value of a node that holds one or needs none computed: a
// leaf, a hashed branch, or a zero-padding node, whose value is the zero
// subtree root of its depth.
func (h *hashing) value(n *node) *[32]byte {
	if n.isEmpty && !n.hasValue {
		return &h.zero[n.depth]
	}
	return &n.value
}

// errMalformedTree is the finalization failure a tree's own shape causes: a
// nil node, or a branch holding one child. It is what Hash reports through a
// panic, so it is told apart from a hash backend's refusal.
var errMalformedTree = errors.New("tree is malformed")

// show prints the subtree in the format Node.Show documents.
func (n *node) show(h *hashing, depth, maxDepth, index int) {
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
		printNode("HASH: " + hex.EncodeToString(h.value(n)[:]) + "\n")
	} else {
		// Leaf node - show value only (no hash for leaves)
		printNode("VALUE: " + hex.EncodeToString(h.value(n)[:]) + "\n")
	}

	if n.isEmpty {
		printNode("EMPTY: true (depth: " + strconv.Itoa(int(n.depth)) + ")\n")
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
		n.left.show(h, depth+1, maxDepth, index*2) // Left child index = parent * 2
	}
	if n.right != nil {
		printNode("RIGHT: \n")
		n.right.show(h, depth+1, maxDepth, index*2+1) // Right child index = parent * 2 + 1
	}
}

// newLeaf initializes a leaf node from up to 32 bytes of value, zero padded.
// A leaf built from a value is data, never padding, whatever the value.
func newLeaf(value []byte) *node {
	n := new(node)
	initLeaf(n, value)
	return n
}

// initLeaf turns the zero node n into a leaf of value, as newLeaf does.
func initLeaf(n *node, value []byte) {
	n.hasValue = true
	copy(n.value[:], value)
}

// newBranch initializes a branch node.
func newBranch(left, right *node) *node {
	return &node{left: left, right: right}
}

// treeFromNodesToDepthFn is the injectable core tree builder. Internal callers
// route binary-tree construction through it so tests can replace it to exercise
// otherwise-unreachable defensive error paths.
var treeFromNodesToDepthFn = treeFromNodesToDepth

// treeFromNodes constructs a tree from leaf nodes, padded with zero subtrees
// up to limit, which has to be a power of two.
func treeFromNodes(leaves []*node, limit int) (*node, error) {
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
		return emptyNode(0), nil
	}
	if len(leaves) > limit {
		return nil, fmt.Errorf("number of leaves %d exceeds limit %d", len(leaves), limit)
	}
	return treeFromNodes64(leaves, uint64(limit))
}

// treeFromNodes64 is treeFromNodes with a limit an int cannot hold on a
// 32-bit target.
func treeFromNodes64(leaves []*node, limit uint64) (*node, error) {
	if limit == 0 {
		// A zero capacity cannot hold any leaf; accepting one would silently drop
		// it and produce a valid-looking root for a different tree.
		if len(leaves) > 0 {
			return nil, fmt.Errorf("number of leaves %d exceeds limit %d", len(leaves), 0)
		}
		return emptyNode(0), nil
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
func treeFromNodesToDepth(leaves []*node, depth int) (*node, error) {
	numLeaves := len(leaves)

	// Reject excess leaves (silently dropped otherwise) when 2^depth is
	// representable. The capacity is compared in uint64 so that depths at or
	// above the platform int width (31 on 32-bit targets) keep their real
	// capacity instead of wrapping to zero; for depth >= 64 the capacity dwarfs
	// any real leaf count.
	if depth >= 0 && depth < 64 && uint64(numLeaves) > (uint64(1)<<uint(depth)) {
		return nil, fmt.Errorf("number of leaves %d exceeds limit %d", numLeaves, uint64(1)<<uint(depth))
	}

	// there are no leaves, return a zero order hash node
	if numLeaves == 0 {
		if depth < 0 {
			depth = 0
		}
		return emptyNode(depth), nil
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
	branchNodes := make([]node, totalBranches)
	current := make([]*node, firstLevelCount)
	next := make([]*node, firstLevelCount)
	branchPos := 0

	for i := range firstLevelCount {
		leftIdx := i * 2
		rightIdx := i*2 + 1

		var left, right *node
		left = leaves[leftIdx]
		if rightIdx < numLeaves {
			right = leaves[rightIdx]
		} else {
			right = emptyNode(0)
		}
		branchNodes[branchPos] = node{left: left, right: right}
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

			var left, right *node
			left = current[leftIdx]
			if rightIdx < activeCount {
				right = current[rightIdx]
			} else {
				right = emptyNode(depth - d)
			}
			branchNodes[branchPos] = node{left: left, right: right}
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

// treeFromNodesProgressive constructs a progressive tree from leaf nodes:
// subtrees of 1, 4, 16, ... leaves, each the left child of the node holding
// the rest.
func treeFromNodesProgressive(leaves []*node) (*node, error) {
	if len(leaves) == 0 {
		return emptyNode(0), nil
	}

	for i := range leaves {
		if leaves[i] == nil {
			return nil, fmt.Errorf("leaf at index %d is nil", i)
		}
	}

	return treeFromNodesProgressiveImpl(leaves, 0)
}

// treeFromNodesProgressiveImpl implements the recursive progressive tree construction
func treeFromNodesProgressiveImpl(leaves []*node, depth int) (*node, error) {
	if len(leaves) == 0 {
		return emptyNode(0), nil
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
	var rightChild *node
	if len(rightNodes) == 0 {
		rightChild = emptyNode(0)
	} else {
		rightChild, err = treeFromNodesProgressiveImpl(rightNodes, depth+2)
		if err != nil {
			return nil, err
		}
	}

	// Return PairNode(left, right)
	return newBranch(leftChild, rightChild), nil
}

// treeFromNodesWithMixin64 builds the tree for a leaf count the limit does not
// hold by taking the depth the leaves need, as hasher.Hasher takes it, so every
// leaf reaches the root and no two values that differ share one. The walkers
// reduce through this so a refused scope still produces the root they agree on;
// the exported form above refuses the limit instead.
func treeFromNodesWithMixin64(leaves []*node, num, limit uint64) (*node, error) {
	count := uint64(len(leaves))
	if limit == 0 {
		// No limit: the tree is exactly as deep as the leaves require.
		limit = count
	}
	if count > limit {
		limit = count
	}

	depth := chunkLimitDepth(limit)

	mainTree, err := treeFromNodesToDepthFn(leaves, depth)
	if err != nil {
		return nil, err
	}

	// Mixin len
	countLeaf := leafFromUint64(num)
	node := newBranch(mainTree, countLeaf)
	return node, nil
}

// treeFromNodesProgressiveWithMixin64 constructs a progressive tree from
// leaf nodes and mixes in num.
func treeFromNodesProgressiveWithMixin64(leaves []*node, num uint64) (*node, error) {
	mainTree, err := treeFromNodesProgressive(leaves)
	if err != nil {
		return nil, err
	}

	countLeaf := leafFromUint64(num)
	node := newBranch(mainTree, countLeaf)
	return node, nil
}

// treeFromNodesProgressiveWithActiveFields constructs a progressive tree from
// leaf nodes and mixes in an active-fields bitvector.
func treeFromNodesProgressiveWithActiveFields(leaves []*node, activeFields []byte) (*node, error) {
	mainTree, err := treeFromNodesProgressive(leaves)
	if err != nil {
		return nil, err
	}

	activeFieldsLeaf, err := activeFieldsNode(activeFields)
	if err != nil {
		return nil, err
	}

	node := newBranch(mainTree, activeFieldsLeaf)
	return node, nil
}

// activeFieldsNode turns an active-fields bitvector into the single node that is
// mixed in beside the tree, as hasher.Hasher does: up to 256 bits are one chunk,
// and a wider bitvector is merkleized to its root first so the fields past the
// first chunk still reach the tree.
func activeFieldsNode(activeFields []byte) (*node, error) {
	if len(activeFields) <= 32 {
		return leafFromBytes(activeFields), nil
	}

	chunks := make([]*node, 0, (len(activeFields)+31)/32)
	for off := 0; off < len(activeFields); off += 32 {
		end := off + 32
		if end > len(activeFields) {
			end = len(activeFields)
		}
		chunks = append(chunks, leafFromBytes(activeFields[off:end]))
	}

	return treeFromNodesToDepthFn(chunks, chunkLimitDepth(uint64(len(chunks))))
}

// emptyChild returns the zero-padding child one level below an empty node.
// Both children of an empty node are the same zero subtree, so direction does
// not matter. It reports false for the zero chunk, which has nothing below
// it, and for an empty node that carries a root of its own instead of a
// depth.
func emptyChild(n *node) (*node, bool) {
	if n.hasValue || n.depth == 0 {
		return nil, false
	}
	return emptyNode(int(n.depth) - 1), true
}

// get fetches the node with the given generalized index.
func (n *node) get(index int) (*node, error) {
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
	pending [][]*node
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
func (n *node) finalize(h *hashing) error {
	// A cached root value certifies the whole subtree is finalized.
	if n == nil || n.hasValue || (n.left == nil && n.right == nil) {
		return nil
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
			if err := hashBatch(batch, scratch.buf, h); err != nil {
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
	var walk func(node *node, depth int)
	walk = func(node *node, depth int) {
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
		return fmt.Errorf("%w: branch with a single nil child", errMalformedTree)
	case total < finalizeThreshold:
		// Small trees hash recursively: batch setup costs more than the
		// vectorized backend saves.
		return hashNodeFn(n, h)
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
func hashNodeFn(n *node, h *hashing) error {
	if n == nil {
		return fmt.Errorf("%w: nil node", errMalformedTree)
	}
	if n.hasValue || (n.left == nil && n.right == nil) {
		return nil
	}
	if n.left == nil || n.right == nil {
		return fmt.Errorf("%w: branch with a single nil child", errMalformedTree)
	}

	if err := hashNodeFn(n.left, h); err != nil {
		return err
	}
	if err := hashNodeFn(n.right, h); err != nil {
		return err
	}

	var pair [64]byte
	copy(pair[:32], h.value(n.left)[:])
	copy(pair[32:], h.value(n.right)[:])
	if err := h.fn(pair[:32], pair[:]); err != nil {
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
func hashBatch(batch []*node, buf []byte, h *hashing) error {
	for i, node := range batch {
		off := i * 64
		copy(buf[off:off+32], h.value(node.left)[:])
		copy(buf[off+32:off+64], h.value(node.right)[:])
	}
	if err := h.fn(buf[:len(batch)*32], buf[:len(batch)*64]); err != nil {
		return err
	}
	for i, node := range batch {
		copy(node.value[:], buf[i*32:(i+1)*32])
		node.hasValue = true
		node.isVisited = false
	}
	return nil
}

// nodeValue returns the value of a node of a finalized tree: leaf data, a
// branch hash, or the zero subtree root of a zero-padding node. It panics on
// an incomplete tree (a nil node, or a branch that could not be hashed
// because a child is missing); the error-reporting paths run through
// finalize instead.
func nodeValue(n *node, h *hashing) []byte {
	if n == nil || (!n.hasValue && (n.left != nil || n.right != nil)) {
		panic("Tree incomplete")
	}
	return h.value(n)[:]
}

// prove returns the leaf and the sibling hashes for the given generalized
// index, finalizing the tree first.
func (n *node) prove(h *hashing, index int) (*Proof, error) {
	if index < 1 {
		return nil, fmt.Errorf("invalid generalized index %d (must be >= 1)", index)
	}
	if err := n.finalize(h); err != nil {
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
			siblingHash = h.value(child)[:]
			cur = child
		case getPosAtLevel(index, i):
			if cur.left == nil {
				return nil, errors.New("Node not found in tree")
			}
			siblingHash = nodeValue(cur.left, h)
			cur = cur.right
		default:
			if cur.right == nil {
				return nil, errors.New("Node not found in tree")
			}
			siblingHash = nodeValue(cur.right, h)
			cur = cur.left
		}
		// Copy the sibling hash: empty siblings alias the zero subtree roots
		// of the Algorithm, and the caller must be free to mutate the
		// returned proof.
		hashes = append(hashes, bytes.Clone(siblingHash))
		if cur == nil {
			return nil, errors.New("Node not found in tree")
		}
	}

	for i, j := 0, len(hashes)-1; i < j; i, j = i+1, j-1 {
		hashes[i], hashes[j] = hashes[j], hashes[i]
	}

	proof.Hashes = hashes
	proof.Leaf = bytes.Clone(nodeValue(cur, h))

	return proof, nil
}

// proveMulti returns the leaves at the given generalized indices and the
// hashes needed to reconstruct the root from them, finalizing the tree first.
func (n *node) proveMulti(h *hashing, indices []int) (*Multiproof, error) {
	for _, gi := range indices {
		if gi < 1 {
			return nil, fmt.Errorf("invalid generalized index %d (must be >= 1)", gi)
		}
	}
	if err := n.finalize(h); err != nil {
		return nil, err
	}
	reqIndices := getRequiredIndices(indices)
	// Indices is cloned like Leaves and Hashes: storing the caller's slice by
	// reference lets a later reorder or reuse of it silently invalidate a proof
	// that already verified.
	proof := &Multiproof{Indices: slices.Clone(indices), Leaves: make([][]byte, len(indices)), Hashes: make([][]byte, len(reqIndices))}

	// Copy leaf and hash values: empty nodes alias the zero subtree roots of
	// the Algorithm, so the returned proof must own its bytes to stay
	// mutation-safe.
	for i, gi := range indices {
		leaf, err := n.get(gi)
		if err != nil {
			return nil, err
		}
		proof.Leaves[i] = bytes.Clone(nodeValue(leaf, h))
	}

	for i, gi := range reqIndices {
		cur, err := n.get(gi)
		if err != nil {
			return nil, err
		}
		proof.Hashes[i] = bytes.Clone(nodeValue(cur, h))
	}

	return proof, nil
}

// leafFromUint64 creates a leaf from a uint64 value, encoded as little-endian
// in the first 8 bytes.
func leafFromUint64(i uint64) *node {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], i)
	return newLeaf(buf[:])
}

// leafFromBytes creates a node from a byte slice: a single leaf for up to 32
// bytes, and a subtree over its 32-byte chunks for a longer slice.
func leafFromBytes(b []byte) *node {
	l := len(b)
	if l <= 32 {
		return newLeaf(b)
	}

	numChunks := (l + 31) / 32
	leaves := make([]*node, numChunks)
	for i := range leaves {
		start := i * 32
		leaves[i] = newLeaf(b[start:min(start+32, l)])
	}

	// limit is a power of two, so treeFromNodes never returns an error here.
	root, _ := treeFromNodes(leaves, int(sszutils.NextPowerOfTwo(uint64(numChunks))))
	return root
}

func isPowerOfTwo(n int) bool {
	return (n & (n - 1)) == 0
}
