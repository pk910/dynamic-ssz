// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package treeproof

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// This file holds the exported API the library does not use itself: building
// trees by hand from nodes, the node-adding and Commit* methods of the
// Wrapper, and the hash function options. It exists for code written against
// earlier releases.

// joins reports whether the node can become part of a tree of alg: a subtree
// with branches has to be built for a compatible Algorithm.
func (n *Node) joins(alg *hasher.Algorithm) error {
	if !n.IsLeaf() && !n.Algorithm().Compatible(alg) {
		return sszutils.NewSszError(sszutils.ErrHashAlgorithmMismatch, "the subtree was built for another hash algorithm than the tree it is added to")
	}
	return nil
}

// inner returns the nodes of the given handles for a tree of alg. A nil
// handle stays a nil node, which the tree builders report.
func inner(alg *hasher.Algorithm, handles []*Node) ([]*node, error) {
	nodes := make([]*node, len(handles))
	for i, handle := range handles {
		if handle == nil {
			continue
		}
		if err := handle.joins(alg); err != nil {
			return nil, err
		}
		nodes[i] = handle.node()
	}
	return nodes, nil
}

// build runs a tree builder over the given handles and wraps its result. A
// tree that is one of the given nodes itself is returned as that handle,
// where the handle names the Algorithm of the tree.
func build(opts []Option, leaves []*Node, builder func(leaves []*node) (*node, error)) (*Node, error) {
	alg := algorithmOf(opts)
	nodes, err := inner(alg, leaves)
	if err != nil {
		return nil, err
	}
	root, err := builder(nodes)
	if err != nil {
		return nil, err
	}
	if len(leaves) == 1 && leaves[0] != nil && leaves[0].node() == root && leaves[0].Algorithm() == alg {
		return leaves[0], nil
	}
	return &Node{n: root, alg: alg}, nil
}

// NewNodeWithValue initializes a leaf node holding a copy of value, zero
// padded to the full 32-byte chunk. Copying is what makes the node
// independent of the caller's buffer: retaining a reference would let a
// later mutation of a reused scratch buffer silently change every tree and
// root built from it. A value longer than 32 bytes panics — a leaf cannot
// hold it, and truncating would silently corrupt every root built from the
// node; use LeafFromBytes to merkleize longer input into a subtree.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func NewNodeWithValue(value []byte, opts ...Option) *Node {
	if len(value) > 32 {
		panic(fmt.Sprintf("NewNodeWithValue: value length %d exceeds the 32-byte leaf chunk", len(value)))
	}
	return &Node{n: newLeaf(value), alg: algorithmOf(opts)}
}

// NewEmptyNode creates an empty (zero-padding) tree node from the root of a
// zero subtree of the tree's Algorithm. Empty nodes represent unused
// positions in the binary tree. A root that is no zero subtree root of the
// Algorithm makes an empty node that keeps the root and has nothing below it.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func NewEmptyNode(zeroOrderHash []byte, opts ...Option) *Node {
	alg := algorithmOf(opts)
	if depth, ok := alg.ZeroHashLevel(zeroOrderHash); ok {
		return &Node{n: emptyNode(depth), alg: alg}
	}
	n := &node{hasValue: true, isEmpty: true}
	copy(n.value[:], zeroOrderHash)
	return &Node{n: n, alg: alg}
}

// NewNodeWithLR initializes a branch node. It panics when a child is a
// subtree built for another hash than the node's Algorithm, which cannot be
// part of it.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func NewNodeWithLR(left, right *Node, opts ...Option) *Node {
	alg := algorithmOf(opts)
	children, err := inner(alg, []*Node{left, right})
	if err != nil {
		panic("NewNodeWithLR: " + err.Error())
	}
	return &Node{n: newBranch(children[0], children[1]), alg: alg}
}

// TreeFromChunks constructs a tree from leaf values.
// The number of leaves should be a power of 2, and every chunk must be exactly
// 32 bytes: a shorter chunk would be zero-extended and a longer one
// truncated. Either way distinct inputs would collide.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func TreeFromChunks(chunks [][]byte, opts ...Option) (*Node, error) {
	root, err := treeFromChunks(chunks)
	if err != nil {
		return nil, err
	}
	return &Node{n: root, alg: algorithmOf(opts)}, nil
}

// TreeFromNodes constructs a tree from leaf nodes.
// This is useful for merging subtrees.
// The limit should be a power of 2.
// Adjacent sibling nodes will be filled with zero subtrees according to the
// tree depth.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func TreeFromNodes(leaves []*Node, limit int, opts ...Option) (*Node, error) {
	return build(opts, leaves, func(nodes []*node) (*node, error) {
		return treeFromNodes(nodes, limit)
	})
}

// TreeFromNodes64 is the uint64 form of TreeFromNodes, for a limit an int
// cannot hold on a 32-bit target.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func TreeFromNodes64(leaves []*Node, limit uint64, opts ...Option) (*Node, error) {
	return build(opts, leaves, func(nodes []*node) (*node, error) {
		return treeFromNodes64(nodes, limit)
	})
}

// TreeFromNodesProgressive constructs a progressive tree from leaf nodes.
// This implements the progressive merkleization algorithm where chunks are
// split into groups with increasing base sizes (1, 4, 16, 64, ...).
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func TreeFromNodesProgressive(leaves []*Node, opts ...Option) (*Node, error) {
	return build(opts, leaves, treeFromNodesProgressive)
}

// TreeFromNodesWithMixin constructs a Merkle tree from leaves and mixes in the
// provided num value as the right child of the root. A zero limit takes the
// number of leaves as the limit.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func TreeFromNodesWithMixin(leaves []*Node, num, limit int, opts ...Option) (*Node, error) {
	if limit < 0 {
		limit = 0
	}
	if num < 0 {
		num = 0
	}
	return TreeFromNodesWithMixin64(leaves, uint64(num), uint64(limit), opts...)
}

// TreeFromNodesWithMixin64 is the uint64 form of TreeFromNodesWithMixin, for
// a count and a limit an int cannot hold on a 32-bit target. More leaves than
// a non-zero limit holds fail with sszutils.ErrChunkLimitExceeded.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func TreeFromNodesWithMixin64(leaves []*Node, num, limit uint64, opts ...Option) (*Node, error) {
	return build(opts, leaves, func(nodes []*node) (*node, error) {
		return treeFromNodesWithMixinChecked64(nodes, num, limit)
	})
}

// TreeFromNodesProgressiveWithMixin constructs a progressive tree with length mixin.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func TreeFromNodesProgressiveWithMixin(leaves []*Node, num int, opts ...Option) (*Node, error) {
	if num < 0 {
		num = 0
	}
	return TreeFromNodesProgressiveWithMixin64(leaves, uint64(num), opts...)
}

// TreeFromNodesProgressiveWithMixin64 is the uint64 form of
// TreeFromNodesProgressiveWithMixin.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func TreeFromNodesProgressiveWithMixin64(leaves []*Node, num uint64, opts ...Option) (*Node, error) {
	return build(opts, leaves, func(nodes []*node) (*node, error) {
		return treeFromNodesProgressiveWithMixin64(nodes, num)
	})
}

// TreeFromNodesProgressiveWithActiveFields constructs a progressive tree with active fields bitvector.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func TreeFromNodesProgressiveWithActiveFields(leaves []*Node, activeFields []byte, opts ...Option) (*Node, error) {
	return build(opts, leaves, func(nodes []*node) (*node, error) {
		return treeFromNodesProgressiveWithActiveFields(nodes, activeFields)
	})
}

// LeafFromUint64 creates a 32-byte leaf node from a uint64 value, encoded as
// little-endian in the first 8 bytes with the remaining 24 bytes zero-padded.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func LeafFromUint64(i uint64, opts ...Option) *Node {
	return &Node{n: leafFromUint64(i), alg: algorithmOf(opts)}
}

// LeafFromUint32 creates a 32-byte leaf node from a uint32 value, encoded as
// little-endian in the first 4 bytes with the remaining 28 bytes zero-padded.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func LeafFromUint32(i uint32, opts ...Option) *Node {
	return LeafFromUint64(uint64(i), opts...)
}

// LeafFromUint16 creates a 32-byte leaf node from a uint16 value, encoded as
// little-endian in the first 2 bytes with the remaining 30 bytes zero-padded.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func LeafFromUint16(i uint16, opts ...Option) *Node {
	return LeafFromUint64(uint64(i), opts...)
}

// LeafFromUint8 creates a 32-byte leaf node from a uint8 value, stored in the
// first byte with the remaining 31 bytes zero-padded.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func LeafFromUint8(i uint8, opts ...Option) *Node {
	return LeafFromUint64(uint64(i), opts...)
}

// LeafFromBool creates a 32-byte leaf node from a boolean value, encoded as
// 0x01 (true) or 0x00 (false) in the first byte with 31 bytes zero-padded.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func LeafFromBool(b bool, opts ...Option) *Node {
	if b {
		return LeafFromUint64(1, opts...)
	}
	return LeafFromUint64(0, opts...)
}

// LeafFromBytes creates a tree node from a byte slice. A slice of up to 32
// bytes becomes a single leaf, right-padded with zeros, and a longer slice is
// split into 32-byte chunks that are merkleized into a subtree so the node
// still hashes to a single 32-byte root.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func LeafFromBytes(b []byte, opts ...Option) *Node {
	return &Node{n: leafFromBytes(b), alg: algorithmOf(opts)}
}

// EmptyLeaf creates a leaf node containing 32 zero bytes, representing an
// empty or unset value in the Merkle tree.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func EmptyLeaf(opts ...Option) *Node {
	return &Node{n: emptyNode(0), alg: algorithmOf(opts)}
}

// LeavesFromUint64 packs a slice of uint64 values into leaf nodes, with 4
// values per 32-byte leaf (8 bytes each, little-endian). The final leaf is
// zero-padded if the number of items is not a multiple of 4.
//
// Deprecated: build the tree with DynSsz.GetTree or by walking the value
// into a Wrapper.
func LeavesFromUint64(items []uint64, opts ...Option) []*Node {
	if len(items) == 0 {
		return []*Node{}
	}
	alg := algorithmOf(opts)

	numLeaves := (len(items)*8 + 31) / 32
	buf := make([]byte, numLeaves*32)
	for i, v := range items {
		binary.LittleEndian.PutUint64(buf[i*8:(i+1)*8], v)
	}

	leaves := make([]*Node, numLeaves)
	for i := 0; i < numLeaves; i++ {
		leaves[i] = &Node{n: newLeaf(buf[i*32 : (i+1)*32]), alg: alg}
	}

	return leaves
}

// WithHashFn finalizes through the given hash function instead of the
// accelerated default backend, on every path — batched flushes and the
// recursive hashing of trees below the batching threshold alike. An error
// from fn aborts finalization and is returned by Finalize. fn must support
// in-place hashing: dst aliases the front of input.
//
// Deprecated: fn has to compute the hash of the tree's Algorithm. Build the
// tree for the Algorithm of the function instead (WithAlgorithm,
// hasher.AlgorithmFor).
func WithHashFn(fn hasher.HashFn) FinalizeOption {
	return func(c *finalizeConfig) {
		c.fn = fn
	}
}

// NewWrapperWithHashFn returns a Wrapper that compresses with fn, so a tree
// answers with the same root the hasher gives for the same value: a caller
// that installs a backend installs it for both walkers or for neither.
//
// The tree is built for the Algorithm derived from fn, as
// hasher.NewHasherWithHashFn derives it: fn is called once to find the hash
// it computes.
//
// Deprecated: use NewWrapper with the Algorithm of the function
// (WithAlgorithm, hasher.AlgorithmFor), which is derived once.
func NewWrapperWithHashFn(fn hasher.HashFn) *Wrapper {
	return NewWrapper(WithAlgorithm((&hasher.HasherPool{HashFn: fn}).Algorithm()))
}

// alignForNode pads to the next chunk boundary of the enclosing scope, so a
// node added through the legacy Add* API occupies a chunk of its own and keeps
// its identity as a leaf. hasher.Hasher has no counterpart to these methods,
// so nothing constrains this to the byte layout of a value write.
func (w *Wrapper) alignForNode() {
	off := 0
	if n := len(w.scopes); n > 0 {
		off = w.scopes[n-1].off
	}
	w.fillRegionUpTo32(off)
}

// AddNode places a node at the next chunk boundary, where it occupies one
// chunk. A subtree built for a hash the wrapper's Algorithm is not compatible
// with is refused: HashErr reports sszutils.ErrHashAlgorithmMismatch.
//
// Deprecated: write the value with the Put and Append methods.
func (w *Wrapper) AddNode(n *Node) {
	if n == nil {
		// The tree builder refuses a missing node when the region is reduced.
		w.addNode(nil)
		return
	}
	if err := n.joins(w.Algorithm()); err != nil {
		// A zero chunk takes the place of the refused subtree, so the
		// wrapper never hashes a node of another tree.
		w.setWalkErr(err)
		w.addNode(emptyNode(0))
		return
	}
	w.addNode(n.node())
}

// addNode places a node at the next chunk boundary.
func (w *Wrapper) addNode(n *node) {
	w.alignForNode()
	off := len(w.buf)
	w.buf = append(w.buf, zeroChunk[:]...)
	w.nodes = append(w.nodes, nodeRef{off: off, node: n})
}

// AddUint64 adds a uint64 as a leaf node.
//
// Deprecated: write the value with the Put and Append methods.
func (w *Wrapper) AddUint64(i uint64) {
	w.addNode(leafFromUint64(i))
}

// AddUint32 adds a uint32 as a leaf node.
//
// Deprecated: write the value with the Put and Append methods.
func (w *Wrapper) AddUint32(i uint32) {
	w.addNode(leafFromUint64(uint64(i)))
}

// AddUint16 adds a uint16 as a leaf node.
//
// Deprecated: write the value with the Put and Append methods.
func (w *Wrapper) AddUint16(i uint16) {
	w.addNode(leafFromUint64(uint64(i)))
}

// AddUint8 adds a uint8 as a leaf node.
//
// Deprecated: write the value with the Put and Append methods.
func (w *Wrapper) AddUint8(i uint8) {
	w.addNode(leafFromUint64(uint64(i)))
}

// AddBytes adds a byte slice as a leaf node (<=32 bytes) or as a subtree of
// its chunks. An empty value is a leaf of zeros, the meaning this method has
// carried since v1: it builds a tree from values, so every value it is given
// takes a place in the leaf order. PutBytes, the walker operation, is the one
// that contributes nothing for an empty value, because there the value writes
// bytes into a region.
//
// Deprecated: write the value with the Put and Append methods.
func (w *Wrapper) AddBytes(b []byte) {
	if len(b) <= 32 {
		w.addNode(leafFromBytes(b))
		return
	}
	w.alignForNode()
	indx := len(w.buf)
	w.AppendBytes32(b)
	w.reduceBinary(indx)
}

// addEmpty adds an empty (all-zeros) leaf node.
func (w *Wrapper) addEmpty() {
	w.addNode(emptyNode(0))
}

// Commit reduces the region that began at index i into a binary subtree.
//
// Deprecated: use Merkleize.
func (w *Wrapper) Commit(i int) { w.Merkleize(i) }

// CommitWithMixin reduces the region that began at index i with a length mixin.
//
// Deprecated: use MerkleizeWithMixin.
func (w *Wrapper) CommitWithMixin(i, num, limit int) {
	w.MerkleizeWithMixin(i, uint64(num), uint64(limit))
}

// CommitProgressive reduces the region that began at index i progressively.
//
// Deprecated: use MerkleizeProgressive.
func (w *Wrapper) CommitProgressive(i int) { w.MerkleizeProgressive(i) }

// CommitProgressiveWithMixin reduces the region that began at index i
// progressively, with a length mixin.
//
// Deprecated: use MerkleizeProgressiveWithMixin.
func (w *Wrapper) CommitProgressiveWithMixin(i, num int) {
	w.MerkleizeProgressiveWithMixin(i, uint64(num))
}

// CommitProgressiveWithActiveFields reduces the region that began at index i
// progressively, with an active-fields bitvector mixin.
//
// Deprecated: use MerkleizeProgressiveWithActiveFields.
func (w *Wrapper) CommitProgressiveWithActiveFields(i int, activeFields []byte) {
	w.MerkleizeProgressiveWithActiveFields(i, activeFields)
}

// AddEmpty adds an empty (all-zeros) leaf node.
//
// Deprecated: retained for v1 API compatibility.
func (w *Wrapper) AddEmpty() { w.addEmpty() }

// treeFromChunks constructs a tree from leaf values, which have to be a power
// of two many chunks of exactly 32 bytes.
func treeFromChunks(chunks [][]byte) (*node, error) {
	numLeaves := len(chunks)
	if numLeaves == 0 {
		return nil, errors.New("cannot create tree from empty chunks")
	}
	if !isPowerOfTwo(numLeaves) {
		return nil, errors.New("number of leaves should be a power of 2")
	}

	leaves := make([]*node, numLeaves)
	for i, c := range chunks {
		if len(c) != 32 {
			return nil, fmt.Errorf("chunk %d has length %d, want 32", i, len(c))
		}
		leaves[i] = newLeaf(c)
	}
	return treeFromNodes(leaves, numLeaves)
}

// treeFromNodesWithMixinChecked64 constructs a tree from leaf nodes and mixes
// in num, refusing more leaves than a non-zero limit holds.
func treeFromNodesWithMixinChecked64(leaves []*node, num, limit uint64) (*node, error) {
	if count := uint64(len(leaves)); limit > 0 && count > limit {
		return nil, sszutils.ErrChunkLimitFn(count, limit)
	}

	return treeFromNodesWithMixin64(leaves, num, limit)
}
