// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
//
// This file contains code derived from https://github.com/ferranbt/fastssz/blob/v1.0.0/wrapper.go
// Copyright (c) 2020 Ferran Borreguero
// Licensed under the MIT License
// The code has been modified for dynamic-ssz proof generation needs.

package treeproof

import (
	"fmt"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

var _ sszutils.HashWalker = (*Wrapper)(nil)

// Wrapper implements the sszutils.HashWalker interface to construct a complete
// Merkle tree instead of computing a single hash. This allows generating proofs
// for any field within an SSZ structure by building the tree during the same
// traversal that would normally produce only the hash tree root.
//
// Usage:
//
//	w := treeproof.NewWrapper()
//	// Use w as a HashWalker (e.g., via DynSsz.HashTreeRootWith or generated code)
//	myStruct.HashTreeRootWithDyn(specs, w)
//	tree := w.Node()
//	proof, _ := tree.Prove(generalizedIndex)
type Wrapper struct {
	nodes []*Node
	buf   []byte
	tmp   []byte
}

// NewWrapper creates a new Wrapper ready to construct a Merkle tree. The
// wrapper starts with an empty node list and pre-allocated scratch buffers.
func NewWrapper() *Wrapper {
	return &Wrapper{
		nodes: []*Node{},
		buf:   make([]byte, 0),
		tmp:   make([]byte, 64),
	}
}

// --- Wrapper implements the HashWalker interface ---

// WithTemp provides a temporary scratch buffer to the given function, allowing
// callers to perform intermediate computations without additional allocations.
func (w *Wrapper) WithTemp(fn func(tmp []byte) []byte) {
	w.tmp = fn(w.tmp)
}

// Index returns the current number of nodes in the wrapper's node list. This
// value is used as a checkpoint for subsequent Merkleize calls to identify
// which nodes should be combined into a subtree. Pending Append* bytes are
// flushed first so the checkpoint separates them from the new scope.
func (w *Wrapper) Index() int {
	w.flushBuffer()
	return len(w.nodes)
}

// CurrentIndex returns the current buffer index (debug only)
func (w *Wrapper) CurrentIndex() int {
	return len(w.buf)
}

// StartTree opens a new SSZ object scope. For the tree proof wrapper,
// this is identical to Index() since the wrapper does not support
// incremental hashing. Pending Append* bytes are flushed first so they
// become leaves of the enclosing scope, not of the new child scope.
func (w *Wrapper) StartTree(_ sszutils.TreeType) int {
	w.flushBuffer()
	return len(w.nodes)
}

// Collapse is a no-op for the tree proof wrapper.
func (w *Wrapper) Collapse() {}

// Append appends raw bytes to the internal buffer. These bytes are converted
// to leaf nodes when a Merkleize method is called.
func (w *Wrapper) Append(i []byte) {
	w.buf = append(w.buf, i...)
}

// AppendUint64 appends a little-endian uint64 (8 bytes) to the internal buffer.
func (w *Wrapper) AppendUint64(i uint64) {
	w.buf = sszutils.MarshalUint64(w.buf, i)
}

// AppendUint32 appends a little-endian uint32 (4 bytes) to the internal buffer.
func (w *Wrapper) AppendUint32(i uint32) {
	w.buf = sszutils.MarshalUint32(w.buf, i)
}

// AppendUint16 appends a little-endian uint16 (2 bytes) to the internal buffer.
func (w *Wrapper) AppendUint16(i uint16) {
	w.buf = sszutils.MarshalUint16(w.buf, i)
}

// AppendUint8 appends a single byte to the internal buffer.
func (w *Wrapper) AppendUint8(i uint8) {
	w.buf = sszutils.MarshalUint8(w.buf, i)
}

// AppendBool appends a single-byte boolean (0x00 or 0x01) to the internal buffer.
func (w *Wrapper) AppendBool(b bool) {
	w.buf = sszutils.MarshalBool(w.buf, b)
}

// AppendBytes32 appends the given bytes to the internal buffer and pads with
// zeros to the next 32-byte boundary via FillUpTo32.
func (w *Wrapper) AppendBytes32(b []byte) {
	w.buf = append(w.buf, b...)
	w.FillUpTo32()
}

// FillUpTo32 pads the internal buffer with zero bytes so its length is a
// multiple of 32. This ensures proper 32-byte chunk alignment for leaf nodes.
func (w *Wrapper) FillUpTo32() {
	// pad zero bytes to the left
	if rest := len(w.buf) % 32; rest != 0 {
		w.buf = sszutils.AppendZeroPadding(w.buf, 32-rest)
	}
}

// Merkleize flushes any buffered bytes as leaf nodes, then constructs a binary
// Merkle tree from all nodes added since the checkpoint index indx. The
// resulting subtree replaces those nodes as a single tree node.
func (w *Wrapper) Merkleize(indx int) {
	w.flushBuffer()
	w.commit(indx)
}

// MerkleizeWithMixin flushes buffered bytes, constructs a binary Merkle tree
// from nodes since indx, and mixes in the element count (num) as a right
// sibling. This implements SSZ list merkleization: hash(merkle_root || length).
func (w *Wrapper) MerkleizeWithMixin(indx int, num, limit uint64) {
	w.flushBuffer()
	w.commitWithMixin(indx, num, limit)
}

// MerkleizeProgressive flushes buffered bytes, then constructs a progressive
// Merkle tree from nodes since indx using the progressive split pattern
// (base sizes 1, 4, 16, 64...) instead of even binary splits.
func (w *Wrapper) MerkleizeProgressive(indx int) {
	w.flushBuffer()
	w.commitProgressive(indx)
}

// MerkleizeProgressiveWithMixin flushes buffered bytes, constructs a
// progressive Merkle tree from nodes since indx, and mixes in the element
// count (num) as a right sibling.
func (w *Wrapper) MerkleizeProgressiveWithMixin(indx int, num uint64) {
	w.flushBuffer()
	w.commitProgressiveWithMixin(indx, num)
}

// MerkleizeProgressiveWithActiveFields flushes buffered bytes, constructs a
// progressive Merkle tree from nodes since indx, and mixes in the active fields
// bitvector as a right sibling. This is used for stable/progressive containers.
func (w *Wrapper) MerkleizeProgressiveWithActiveFields(indx int, activeFields []byte) {
	w.flushBuffer()
	w.commitProgressiveWithActiveFields(indx, activeFields)
}

// PutBitlist parses a bitlist (with sentinel bit), creates leaf nodes from
// the data, and merkleizes them with a length mixin. The maxSize parameter
// determines the tree limit for proper padding.
func (w *Wrapper) PutBitlist(bb []byte, maxSize uint64) {
	b, size := hasher.ParseBitlist(w.tmp[:0], bb)
	w.tmp = b

	indx := w.Index()
	w.appendBytesAsNodes(b)

	limit := sszutils.CalculateBitlistLimit(maxSize)
	w.commitWithMixin(indx, size, limit)
}

// PutProgressiveBitlist parses a bitlist (with sentinel bit), creates leaf
// nodes from the data, and merkleizes them using the progressive tree algorithm
// with a length mixin.
func (w *Wrapper) PutProgressiveBitlist(bb []byte) {
	// Use the progressive parse so all-zero top chunks are preserved: the chunk
	// count defines the progressive tree shape. See hasher.ParseProgressiveBitlist.
	b, size := hasher.ParseProgressiveBitlist(w.tmp[:0], bb)
	w.tmp = b

	indx := w.Index()
	w.appendBytesAsNodes(b)

	w.commitProgressiveWithMixin(indx, size)
}

// flushBuffer converts any bytes buffered by Append* calls into leaf nodes.
// It must run before a node is added directly (Put*/Add*) and before a
// checkpoint is captured (Index/StartTree), so buffered fields keep their
// position in the leaf order — mirroring the single ordered byte buffer of
// hasher.Hasher.
func (w *Wrapper) flushBuffer() {
	if len(w.buf) != 0 {
		w.appendBytesAsNodes(w.buf)
		w.buf = w.buf[:0]
	}
}

func (w *Wrapper) appendBytesAsNodes(b []byte) {
	// Empty input contributes no leaf (matching hasher.Hasher.AppendBytes32); a
	// phantom zero leaf would shift sibling positions and give progressive
	// bitlists a spec-incorrect root.
	// Pad the final chunk to 32 bytes if the input is not chunk-aligned. The
	// three-index cap keeps the zero bytes out of the caller's backing array
	// (input memory must never be mutated), matching hasher.Hasher.PutBytes
	// which copies before padding.
	if rest := len(b) % 32; rest != 0 {
		b = sszutils.AppendZeroPadding(b[:len(b):len(b)], 32-rest)
	}
	for i := 0; i < len(b); i += 32 {
		val := append([]byte{}, b[i:min(len(b), i+32)]...)
		w.nodes = append(w.nodes, LeafFromBytes(val))
	}
}

// PutBool adds a boolean value as a single 32-byte leaf node.
func (w *Wrapper) PutBool(b bool) {
	w.AddNode(LeafFromBool(b))
}

// PutBytes adds a byte slice as one or more 32-byte leaf nodes. If the slice
// exceeds 32 bytes, it is split into chunks and merkleized into a subtree.
func (w *Wrapper) PutBytes(b []byte) {
	w.AddBytes(b)
}

// PutUint16 adds a uint16 value as a single 32-byte leaf node.
func (w *Wrapper) PutUint16(i uint16) {
	w.AddUint16(i)
}

// PutUint64 adds a uint64 value as a single 32-byte leaf node.
func (w *Wrapper) PutUint64(i uint64) {
	w.AddUint64(i)
}

// PutUint8 adds a uint8 value as a single 32-byte leaf node.
func (w *Wrapper) PutUint8(i uint8) {
	w.AddUint8(i)
}

// PutUint32 adds a uint32 value as a single 32-byte leaf node.
func (w *Wrapper) PutUint32(i uint32) {
	w.AddUint32(i)
}

// PutUint64Array appends all uint64 values as buffered bytes, pads to 32-byte
// alignment, and merkleizes. If maxCapacity is provided, the result is
// merkleized with a length mixin (list semantics); otherwise it uses fixed-size
// merkleization (vector semantics).
func (w *Wrapper) PutUint64Array(b []uint64, maxCapacity ...uint64) {
	indx := w.Index()
	for _, i := range b {
		w.AppendUint64(i)
	}

	// pad zero bytes to the left
	w.FillUpTo32()

	if len(maxCapacity) == 0 {
		// Array with fixed size
		w.Merkleize(indx)
	} else {
		numItems := uint64(len(b))
		limit := sszutils.CalculateLimit(maxCapacity[0], numItems, 8)

		w.MerkleizeWithMixin(indx, numItems, limit)
	}
}

// --- Legacy convenience methods ---

// AddBytes adds a byte slice as a leaf node (<=32 bytes) or as a merkleized
// subtree of 32-byte chunks (>32 bytes).
func (w *Wrapper) AddBytes(b []byte) {
	if len(b) == 0 {
		// Empty input contributes no leaf, matching hasher.Hasher.
		return
	}
	if len(b) <= 32 {
		w.AddNode(LeafFromBytes(b))
	} else {
		indx := w.Index()
		w.appendBytesAsNodes(b)
		w.commit(indx)
	}
}

// AddUint64 adds a uint64 value as a single leaf node.
func (w *Wrapper) AddUint64(i uint64) {
	w.AddNode(LeafFromUint64(i))
}

// AddUint32 adds a uint32 value as a single leaf node.
func (w *Wrapper) AddUint32(i uint32) {
	w.AddNode(LeafFromUint32(i))
}

// AddUint16 adds a uint16 value as a single leaf node.
func (w *Wrapper) AddUint16(i uint16) {
	w.AddNode(LeafFromUint16(i))
}

// AddUint8 adds a uint8 value as a single leaf node.
func (w *Wrapper) AddUint8(i uint8) {
	w.AddNode(LeafFromUint8(i))
}

// AddNode appends a pre-constructed tree node to the wrapper's node list.
// Bytes buffered by Append* calls are flushed to leaf nodes first so they
// keep their position before this node.
func (w *Wrapper) AddNode(n *Node) {
	w.flushBuffer()

	if w.nodes == nil {
		w.nodes = []*Node{}
	}

	w.nodes = append(w.nodes, n)
}

// Node returns the single root node of the constructed Merkle tree. Bytes still
// buffered by Append* calls (e.g. a top-level packed uint256/uint128 that is
// never merkleized into a container) are flushed to a leaf first. Panics if the
// wrapper does not then hold exactly one node, which indicates incomplete
// merkleization.
func (w *Wrapper) Node() *Node {
	w.flushBuffer()
	if len(w.nodes) != 1 {
		panic(fmt.Sprintf("incomplete merkleization: wrapper holds %d nodes, want 1", len(w.nodes)))
	}
	return w.nodes[0]
}

// Hash returns the most recent data of the wrapper's state without mutating
// it: the trailing bytes of the pending Append* buffer when present (up to 32
// bytes — fewer when the buffer is not chunk-aligned, like
// hasher.Hasher.Hash()), otherwise the last committed node's hash. An entirely
// empty wrapper yields a 32-byte zero chunk. Used for verbose/debug logging
// between subvalues, so it tolerates both an unflushed buffer and an empty
// node list.
func (w *Wrapper) Hash() []byte {
	if n := len(w.buf); n > 0 {
		start := 0
		if n > 32 {
			start = n - 32
		}
		return w.buf[start:]
	}
	if len(w.nodes) == 0 {
		return make([]byte, 32)
	}
	return w.nodes[len(w.nodes)-1].Hash()
}

// clampIndex bounds a caller-supplied scope index to the node list so an
// out-of-range value cannot trigger a slice-bounds panic (matching
// hasher.Hasher, which the Wrapper is interchangeable with).
func (w *Wrapper) clampIndex(i int) int {
	if i < 0 {
		return 0
	}
	if i > len(w.nodes) {
		return len(w.nodes)
	}
	return i
}

// Commit constructs a binary Merkle tree from all nodes added since index i,
// replaces those nodes with the resulting subtree root, and adds it back to the
// node list.
func (w *Wrapper) commit(i int) {
	i = w.clampIndex(i)
	// A bare scope pads to the next power of two of the node count.
	res, err := treeFromNodesToDepthFn(w.nodes[i:], chunkLimitDepth(uint64(len(w.nodes[i:]))))
	if err != nil {
		panic(err)
	}
	// remove the old nodes
	w.nodes = w.nodes[:i]
	// add the new node
	w.AddNode(res)
}

// commitWithMixin constructs a binary Merkle tree from nodes since index i with
// a length mixin, used for SSZ list merkleization. num and limit are uint64 so a
// list capacity above the platform int range (e.g. a practically-unbounded
// ssz-max) hashes correctly instead of panicking.
func (w *Wrapper) commitWithMixin(i int, num, limit uint64) {
	i = w.clampIndex(i)
	// create tree from nodes
	res, err := TreeFromNodesWithMixin64(w.nodes[i:], num, limit)
	if err != nil {
		panic(err)
	}
	// remove the old nodes
	w.nodes = w.nodes[:i]

	// add the new node
	w.AddNode(res)
}

// commitProgressive creates a progressive tree from nodes
func (w *Wrapper) commitProgressive(i int) {
	i = w.clampIndex(i)
	// create progressive tree from nodes
	res, err := TreeFromNodesProgressive(w.nodes[i:])
	if err != nil {
		panic(err)
	}
	// remove the old nodes
	w.nodes = w.nodes[:i]
	// add the new node
	w.AddNode(res)
}

// commitProgressiveWithMixin creates a progressive tree with length mixin
func (w *Wrapper) commitProgressiveWithMixin(i int, num uint64) {
	i = w.clampIndex(i)
	// create progressive tree from nodes
	res, err := TreeFromNodesProgressiveWithMixin64(w.nodes[i:], num)
	if err != nil {
		panic(err)
	}
	// remove the old nodes
	w.nodes = w.nodes[:i]
	// add the new node
	w.AddNode(res)
}

// commitProgressiveWithActiveFields creates a progressive tree with active fields bitvector
func (w *Wrapper) commitProgressiveWithActiveFields(i int, activeFields []byte) {
	i = w.clampIndex(i)
	// create progressive tree from nodes
	res, err := TreeFromNodesProgressiveWithActiveFields(w.nodes[i:], activeFields)
	if err != nil {
		panic(err)
	}
	// remove the old nodes
	w.nodes = w.nodes[:i]
	// add the new node
	w.AddNode(res)
}

// addEmpty adds an empty (all-zeros) leaf node to the wrapper's node list.
func (w *Wrapper) addEmpty() {
	w.AddNode(EmptyLeaf())
}

// The methods below preserve the exported v1.x Wrapper API (Commit*/AddEmpty)
// as thin aliases over the current Merkleize* methods, so consumers built
// against earlier v1 releases keep compiling. New code should use Merkleize*.

// Commit merkleizes the nodes added since index i into a binary subtree.
//
// Deprecated: use Merkleize.
func (w *Wrapper) Commit(i int) { w.Merkleize(i) }

// CommitWithMixin merkleizes nodes since index i with a length mixin.
//
// Deprecated: use MerkleizeWithMixin.
func (w *Wrapper) CommitWithMixin(i, num, limit int) {
	w.MerkleizeWithMixin(i, uint64(num), uint64(limit))
}

// CommitProgressive merkleizes nodes since index i into a progressive subtree.
//
// Deprecated: use MerkleizeProgressive.
func (w *Wrapper) CommitProgressive(i int) { w.MerkleizeProgressive(i) }

// CommitProgressiveWithMixin merkleizes nodes since index i as a progressive
// subtree with a length mixin.
//
// Deprecated: use MerkleizeProgressiveWithMixin.
func (w *Wrapper) CommitProgressiveWithMixin(i, num int) {
	w.MerkleizeProgressiveWithMixin(i, uint64(num))
}

// CommitProgressiveWithActiveFields merkleizes nodes since index i as a
// progressive subtree with an active-fields bitvector mixin.
//
// Deprecated: use MerkleizeProgressiveWithActiveFields.
func (w *Wrapper) CommitProgressiveWithActiveFields(i int, activeFields []byte) {
	w.MerkleizeProgressiveWithActiveFields(i, activeFields)
}

// AddEmpty adds an empty (all-zeros) leaf node to the wrapper's node list.
//
// Deprecated: retained for v1 API compatibility.
func (w *Wrapper) AddEmpty() { w.addEmpty() }

// HashRoot returns the 32-byte hash tree root of the constructed tree. This
// implements the HashWalker interface's final step, producing the same root
// hash that a regular Hasher would compute.
func (w *Wrapper) HashRoot() ([32]byte, error) {
	// Flush any buffered bytes so a top-level packed value (never merkleized into
	// a container) still yields its single-leaf root instead of an empty tree.
	w.flushBuffer()
	if len(w.nodes) == 0 {
		return [32]byte{}, fmt.Errorf("expected 32 byte size")
	}
	root := w.nodes[len(w.nodes)-1].Hash()
	if len(root) != 32 {
		return [32]byte{}, fmt.Errorf("expected 32 byte size")
	}
	return [32]byte(root), nil
}
