// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

func TestMultiproofCompress(t *testing.T) {
	// Create a tree with some empty nodes
	chunks := [][]byte{
		sum256ToBytes([]byte("leaf0")),
		sszutils.ZeroBytes()[:32], // This will create zero hashes in proof
		sum256ToBytes([]byte("leaf2")),
		sszutils.ZeroBytes()[:32],
	}

	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	// Generate multiproof
	proof, err := tree.ProveMulti([]int{4, 6})
	if err != nil {
		t.Fatalf("failed to generate multiproof: %v", err)
	}

	// Compress the proof
	compressed := proof.Compress()

	if compressed == nil {
		t.Fatal("compressed proof should not be nil")
	}

	// Check if compression actually happened
	// If no zero hashes were found, the original and compressed should be the same length
	if len(compressed.Hashes) != len(proof.Hashes) {
		t.Error("compressed proof should have same number of hashes as original when no zero hashes present")
	}

	// Decompress and verify
	decompressed := compressed.Decompress()
	if len(decompressed.Hashes) != len(proof.Hashes) {
		t.Error("decompressed proof should match original proof length")
	}

	// Verify the decompressed proof
	rootHash := tree.Hash()
	valid, err := VerifyMultiproof(rootHash, decompressed.Hashes, decompressed.Leaves, decompressed.Indices)
	if err != nil {
		t.Errorf("decompressed proof verification error: %v", err)
	}
	if !valid {
		t.Error("decompressed proof should be valid")
	}
}

func TestCompressDecompressWithZeroHashes(t *testing.T) {
	// Create a proof with actual zero hashes to test compression
	zeroHash := hasher.GetZeroHash(0)
	proof := &Multiproof{
		Hashes: [][]byte{
			zeroHash,                          // This should be compressed
			sum256ToBytes([]byte("not_zero")), // This should not
			zeroHash,                          // This should be compressed
		},
		Leaves: [][]byte{
			sum256ToBytes([]byte("leaf1")),
			sum256ToBytes([]byte("leaf2")),
		},
		Indices: []int{4, 5},
	}

	// Compress the proof
	compressed := proof.Compress()

	if compressed == nil {
		t.Fatal("compressed proof should not be nil")
	}

	// Compression may or may not reduce size, depending on implementation details
	// Just verify that compressed proof is valid
	if len(compressed.Hashes) > len(proof.Hashes) {
		t.Error("compression should not increase number of hashes")
	}

	// Decompress and verify we get back the original
	decompressed := compressed.Decompress()

	if len(decompressed.Hashes) != len(proof.Hashes) {
		t.Errorf("decompressed should have same number of hashes as original: expected %d, got %d",
			len(proof.Hashes), len(decompressed.Hashes))
	}

	// Verify each hash matches
	for i, hash := range decompressed.Hashes {
		if !bytes.Equal(hash, proof.Hashes[i]) {
			t.Errorf("decompressed hash %d doesn't match original", i)
		}
	}

	// Test empty proof compression
	emptyProof := &Multiproof{
		Hashes:  [][]byte{},
		Leaves:  [][]byte{},
		Indices: []int{},
	}

	compressedEmpty := emptyProof.Compress()
	if compressedEmpty == nil {
		t.Error("compressed empty proof should not be nil")
	}

	decompressedEmpty := compressedEmpty.Decompress()
	if len(decompressedEmpty.Hashes) != 0 {
		t.Error("decompressed empty proof should have no hashes")
	}
}

func TestTreeFromNodesProgressiveImplDeepRecursion(t *testing.T) {
	// Create enough nodes to trigger multiple levels of progressive recursion
	// depth 0: baseSize=1, depth 2: baseSize=4, depth 4: baseSize=16
	nodes := make([]*Node, 22)
	for i := range nodes {
		nodes[i] = NewNodeWithValue([]byte{byte(i)})
	}

	tree, err := TreeFromNodesProgressive(nodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if tree.IsLeaf() {
		t.Error("expected branch node for progressive tree with many nodes")
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("isPowerOfTwo", func(t *testing.T) {
		tests := []struct {
			n        int
			expected bool
		}{
			{0, true}, // Edge case
			{1, true},
			{2, true},
			{3, false},
			{4, true},
			{5, false},
			{8, true},
			{15, false},
			{16, true},
			{32, true},
			{33, false},
		}

		for _, tt := range tests {
			result := isPowerOfTwo(tt.n)
			if result != tt.expected {
				t.Errorf("isPowerOfTwo(%d) = %v, want %v", tt.n, result, tt.expected)
			}
		}
	})
}

func TestTreeEdgeCases(t *testing.T) {
	t.Run("panic on incomplete tree", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for incomplete tree")
			}
		}()

		// Create node with only right child
		node := NewNodeWithLR(nil, NewNodeWithValue([]byte{1}))
		_ = node.Hash() // Should panic
	})

	t.Run("hash caching", func(t *testing.T) {
		left := NewNodeWithValue(bytes.Repeat([]byte{1}, 32))
		right := NewNodeWithValue(bytes.Repeat([]byte{2}, 32))
		branch := NewNodeWithLR(left, right)

		// First call computes hash
		hash1 := branch.Hash()
		if !branch.n.hasValue {
			t.Error("hash should be cached")
		}

		// Second call should return cached value
		hash2 := branch.Hash()
		if !bytes.Equal(hash1, hash2) {
			t.Error("cached hash should be returned")
		}
	})

	t.Run("empty right node optimization", func(t *testing.T) {
		left := NewNodeWithValue(bytes.Repeat([]byte{1}, 32))
		emptyRight := NewEmptyNode(hasher.GetZeroHash(0))
		branch := NewNodeWithLR(left, emptyRight)

		// Hash should still be computed correctly
		hash := branch.Hash()
		expected := hashFn(append(left.n.value[:], emptyRight.n.value[:]...))

		if !bytes.Equal(hash, expected) {
			t.Error("hash with empty right node mismatch")
		}
	})
}

func TestProveIntermediateNodeSetsValue(t *testing.T) {
	// Create a fresh tree and prove an intermediate node FIRST
	// (before any Hash() calls) to hit the cur.n.value == nil branch
	chunks := [][]byte{
		sum256ToBytes([]byte("a")),
		sum256ToBytes([]byte("b")),
		sum256ToBytes([]byte("c")),
		sum256ToBytes([]byte("d")),
	}

	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	// Prove index 2 (left branch) on a fresh tree where branch values haven't been cached
	proof, err := tree.Prove(2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proof.Leaf == nil {
		t.Fatal("proof leaf should not be nil for intermediate node")
	}
	if len(proof.Leaf) != 32 {
		t.Fatalf("expected 32-byte leaf, got %d bytes", len(proof.Leaf))
	}
}

func TestProveNilLeftChild(t *testing.T) {
	// Create malformed tree: left is nil, right exists
	rightLeaf := NewNodeWithValue(bytes.Repeat([]byte{1}, 32))
	root := NewNodeWithLR(nil, rightLeaf)

	// Index 3 (binary 11): pathLen=1, level 0 goes RIGHT, checks cur.n.left == nil → error
	_, err := root.Prove(3)
	if err == nil {
		t.Fatal("expected error for nil left child")
	}
}

func TestProveNilAfterTraversal(t *testing.T) {
	// Create tree: left exists but right is nil
	leftLeaf := NewNodeWithValue(bytes.Repeat([]byte{1}, 32))
	root := NewNodeWithLR(leftLeaf, nil)

	// Index 3: goes RIGHT, left != nil (sibling OK), cur = right = nil → error
	_, err := root.Prove(3)
	if err == nil {
		t.Fatal("expected error for nil node after traversal")
	}
}

func TestProveMultiMissingRequiredNode(t *testing.T) {
	// Create tree where right child of root is nil, but left subtree has leaves
	leaf4 := NewNodeWithValue(bytes.Repeat([]byte{1}, 32))
	leaf5 := NewNodeWithValue(bytes.Repeat([]byte{2}, 32))
	leftBranch := NewNodeWithLR(leaf4, leaf5)
	root := NewNodeWithLR(leftBranch, nil)

	// Prove [4, 5]: leaf Gets succeed, but getRequiredIndices([4,5])=[3]
	// Get(3) traverses root→right which is nil → error at reqIndices Get
	_, err := root.ProveMulti([]int{4, 5})
	if err == nil {
		t.Fatal("expected error for missing required node")
	}
}

func TestProveMultiEmptyIndices(t *testing.T) {
	tree := NewNodeWithValue(bytes.Repeat([]byte{1}, 32))

	proof, err := tree.ProveMulti([]int{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(proof.Leaves) != 0 {
		t.Error("expected empty leaves for empty indices")
	}
	if len(proof.Hashes) != 0 {
		t.Error("expected empty hashes for empty indices")
	}
}

func TestTreeFromNodesProgressiveImplEmpty(t *testing.T) {
	// Directly call internal function with empty leaves
	node, err := treeFromNodesProgressiveImpl(nil, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !node.isEmpty {
		t.Fatal("expected empty node for nil leaves")
	}
}

// Decompress must not panic on malformed input: missing ZeroLevels entries fall
// back to depth 0, and out-of-range levels are clamped.
func TestDecompressHandlesMalformedInput(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Decompress panicked on malformed input: %v", r)
		}
	}()

	missing := &CompressedMultiproof{
		Indices:    []int{4},
		Leaves:     [][]byte{make([]byte, 32)},
		Hashes:     [][]byte{nil},
		ZeroLevels: nil,
	}
	mp := missing.Decompress()
	if mp == nil || len(mp.Hashes) != 1 {
		t.Fatalf("unexpected decompressed result: %#v", mp)
	}
	if !bytes.Equal(mp.Hashes[0], hasher.GetZeroHash(0)) {
		t.Errorf("missing ZeroLevels should fall back to depth 0")
	}

	huge := &CompressedMultiproof{
		Hashes:     [][]byte{nil},
		ZeroLevels: []int{1000},
	}
	mp2 := huge.Decompress()
	if mp2 == nil || len(mp2.Hashes) != 1 || len(mp2.Hashes[0]) != 32 {
		t.Fatalf("unexpected decompressed result for huge level: %#v", mp2)
	}
}

func TestTreeFromNodesProgressiveLeftError(t *testing.T) {
	injected := errors.New("injected")
	treeFromNodesToDepthFn = func([]*node, int) (*node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesToDepthFn = treeFromNodesToDepth }()

	_, err := TreeFromNodesProgressive([]*Node{LeafFromUint64(1)})
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got: %v", err)
	}
}

// --- treeFromNodesProgressiveImpl: recursive right child error ---

func TestTreeFromNodesProgressiveRecursiveError(t *testing.T) {
	injected := errors.New("injected")
	calls := 0
	treeFromNodesToDepthFn = func(leaves []*node, depth int) (*node, error) {
		calls++
		if calls == 2 {
			return nil, injected
		}
		return treeFromNodesToDepth(leaves, depth)
	}
	defer func() { treeFromNodesToDepthFn = treeFromNodesToDepth }()

	// Two leaves: first call builds left child (succeeds), recursive call
	// builds the right subtree's left child (fails on second call).
	_, err := TreeFromNodesProgressive([]*Node{LeafFromUint64(1), LeafFromUint64(2)})
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got: %v", err)
	}
}

// --- TreeFromNodesWithMixin: tree build error ---

func TestTreeFromNodesWithMixinInjectedError(t *testing.T) {
	injected := errors.New("injected")
	treeFromNodesToDepthFn = func([]*node, int) (*node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesToDepthFn = treeFromNodesToDepth }()

	_, err := TreeFromNodesWithMixin([]*Node{LeafFromUint64(1)}, 1, 1)
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got: %v", err)
	}
}

// --- TreeFromNodesProgressiveWithMixin: progressive build error ---

func TestTreeFromNodesProgressiveWithMixinInjectedError(t *testing.T) {
	injected := errors.New("injected")
	treeFromNodesToDepthFn = func([]*node, int) (*node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesToDepthFn = treeFromNodesToDepth }()

	_, err := TreeFromNodesProgressiveWithMixin([]*Node{LeafFromUint64(1)}, 1)
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got: %v", err)
	}
}

// --- TreeFromNodesProgressiveWithActiveFields: progressive build error ---

func TestTreeFromNodesProgressiveWithActiveFieldsInjectedError(t *testing.T) {
	injected := errors.New("injected")
	treeFromNodesToDepthFn = func([]*node, int) (*node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesToDepthFn = treeFromNodesToDepth }()

	_, err := TreeFromNodesProgressiveWithActiveFields([]*Node{LeafFromUint64(1)}, []byte{0x01})
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got: %v", err)
	}
}

// The bitvector is built after the tree, so its own failure is reported from
// there. An empty leaf set reduces without the injected builder, which leaves
// the bitvector as the only caller of it.
func TestTreeFromNodesProgressiveWithActiveFieldsBitvectorError(t *testing.T) {
	injected := errors.New("injected")
	treeFromNodesToDepthFn = func([]*node, int) (*node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesToDepthFn = treeFromNodesToDepth }()

	_, err := TreeFromNodesProgressiveWithActiveFields(nil, make([]byte, 65))
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected error, got: %v", err)
	}
}

// TestTreeFromNodesToDepthWideCapacity builds single- and two-leaf trees at
// every depth from the 32-bit int width up to the uint64 ceiling. The capacity
// check must hold in uint64 on every platform: a list capacity such as
// VALIDATOR_REGISTRY_LIMIT (2^40 chunks) reaches this path through GetTree and
// must not be mistaken for an over-full tree on a 32-bit build.
func TestTreeFromNodesToDepthWideCapacity(t *testing.T) {
	leaf := LeafFromUint64(1)
	leaves := []*Node{leaf, LeafFromUint64(2)}

	for _, depth := range []int{31, 32, 33, 40, 62, 63, 64} {
		for _, in := range [][]*Node{{leaf}, leaves} {
			n, err := treeFromNodesToDepth(nodesOf(in), depth)
			if err != nil {
				t.Fatalf("depth %d, %d leaves: %v", depth, len(in), err)
			}
			if n == nil {
				t.Fatalf("depth %d, %d leaves: nil node", depth, len(in))
			}
		}
	}

	// The excess check itself still applies where 2^depth is representable.
	if _, err := treeFromNodesToDepth(nodesOf(leaves), 0); err == nil {
		t.Error("depth 0 with two leaves should error")
	}

	// End to end: a list with a 2^40 chunk capacity holding one element hashes
	// to the same root through the wrapper as through the hasher.
	w := NewWrapper()
	idx := w.Index()
	w.AppendUint64(7)
	w.MerkleizeWithMixin(idx, 1, 1<<40)
	wrapperRoot, err := w.HashRoot()
	if err != nil {
		t.Fatalf("wrapper root: %v", err)
	}

	h := hasher.NewHasher()
	hidx := h.Index()
	h.AppendUint64(7)
	h.MerkleizeWithMixin(hidx, 1, 1<<40)
	hasherRoot, err := h.HashRoot()
	if err != nil {
		t.Fatalf("hasher root: %v", err)
	}
	if wrapperRoot != hasherRoot {
		t.Errorf("wrapper root %x != hasher root %x", wrapperRoot, hasherRoot)
	}
}

// TestTreeBuilderDefensiveBranches covers the remaining defensive branches:
// treeFromNodesToDepth's own excess check (reached only when a caller does not
// pre-validate), the progressive mixin's negative-count clamp, and hashNode's
// nil guard.
func TestTreeBuilderDefensiveBranches(t *testing.T) {
	leaf := LeafFromUint64(1)

	// treeFromNodesToDepth rejects more leaves than 2^depth can hold.
	if _, err := treeFromNodesToDepth(nodesOf([]*Node{leaf, leaf, leaf}), 1); err == nil {
		t.Error("treeFromNodesToDepth with excess leaves should error")
	}

	// The progressive mixin clamps a negative count to zero.
	if _, err := TreeFromNodesProgressiveWithMixin([]*Node{leaf}, -1); err != nil {
		t.Errorf("TreeFromNodesProgressiveWithMixin(leaf, -1): %v", err)
	}

	// Reading the value of a nil node (an incomplete tree) panics.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("nodeValue(nil) should panic")
			}
		}()
		h := hashingOf(hasher.SHA256)
		nodeValue(nil, &h)
	}()
}

// A gindex that lands inside a zero-padding subtree is spec-valid (needed for
// proof-of-emptiness) and must be fetchable and provable, not rejected as "not
// found". The proof verifies against the same root.
func TestProvePaddingGindex(t *testing.T) {
	leaves := make([]*Node, 5)
	for i := range leaves {
		leaves[i] = NewNodeWithValue(sum256ToBytes([]byte{byte(i)}))
	}
	tree, err := TreeFromNodes(leaves, 8)
	if err != nil {
		t.Fatalf("TreeFromNodes: %v", err)
	}
	root := tree.Hash()

	// gindices 13,14,15 are the zero-padding leaves of the 8-leaf tree.
	for _, gi := range []int{13, 14, 15} {
		node, err := tree.Get(gi)
		if err != nil {
			t.Errorf("Get(%d): %v", gi, err)
		} else if !node.IsEmpty() {
			t.Errorf("Get(%d): expected empty padding node", gi)
		}

		proof, err := tree.Prove(gi)
		if err != nil {
			t.Errorf("Prove(%d): %v", gi, err)
			continue
		}
		ok, err := VerifyProof(root, proof)
		if err != nil || !ok {
			t.Errorf("VerifyProof(padding gindex %d) = %v, %v; want true, nil", gi, ok, err)
		}
	}

	// A gindex below the tree depth (past the zero leaves) is still rejected.
	if _, err := tree.Get(30); err == nil {
		t.Error("Get(30) below tree depth should fail")
	}
	if _, err := tree.Prove(30); err == nil {
		t.Error("Prove(30) below tree depth should fail")
	}
}

// TestProveMultiHashesUnfinalizedBranches pins that ProveMulti resolves a
// branch node's hash instead of reading its raw value. A tree fresh from
// GetTree has no cached hashes, so proving a non-leaf gindex used to return an
// empty leaf with a nil error — a proof that could never verify. Prove already
// resolved it via hashNode; ProveMulti did not.
func TestProveMultiHashesUnfinalizedBranches(t *testing.T) {
	build := func() *Node {
		leaves := make([]*Node, 8)
		for i := range leaves {
			b := make([]byte, 32)
			b[0] = byte(i + 1)
			leaves[i] = NewNodeWithValue(b)
		}
		tree, err := TreeFromNodes(leaves, 8)
		if err != nil {
			t.Fatalf("TreeFromNodes: %v", err)
		}
		return tree
	}

	// gindex 1 is the root, 2 and 3 are branches, 8..15 are the leaves.
	for _, indices := range [][]int{{1}, {2}, {3}, {2, 3}, {2, 9}} {
		fresh := build()
		proof, err := fresh.ProveMulti(indices)
		if err != nil {
			t.Fatalf("ProveMulti(%v) on a fresh tree: %v", indices, err)
		}
		for i, leaf := range proof.Leaves {
			if len(leaf) != 32 {
				t.Fatalf("ProveMulti(%v) leaf %d has length %d, want 32", indices, i, len(leaf))
			}
		}

		ok, err := VerifyMultiproof(fresh.Hash(), proof.Hashes, proof.Leaves, proof.Indices)
		if err != nil || !ok {
			t.Fatalf("ProveMulti(%v) on a fresh tree does not verify: ok=%v err=%v", indices, ok, err)
		}

		// Finalizing first must not change the result — the proof cannot depend
		// on hidden tree state.
		finalized := build()
		_ = finalized.Hash()
		after, err := finalized.ProveMulti(indices)
		if err != nil {
			t.Fatalf("ProveMulti(%v) on a finalized tree: %v", indices, err)
		}
		for i := range proof.Leaves {
			if !bytes.Equal(proof.Leaves[i], after.Leaves[i]) {
				t.Fatalf("ProveMulti(%v) leaf %d differs between a fresh and a finalized tree: %x vs %x",
					indices, i, proof.Leaves[i], after.Leaves[i])
			}
		}
	}
}

// TestProveMultiOwnsItsIndices pins that the returned Multiproof does not alias
// the caller's index slice. Prove clones everything it returns; ProveMulti
// cloned Leaves and Hashes but stored Indices by reference, so a later reorder
// of the caller's own slice silently invalidated a proof that already verified.
func TestProveMultiOwnsItsIndices(t *testing.T) {
	leaves := make([]*Node, 8)
	for i := range leaves {
		b := make([]byte, 32)
		b[0] = byte(i + 1)
		leaves[i] = NewNodeWithValue(b)
	}
	tree, err := TreeFromNodes(leaves, 8)
	if err != nil {
		t.Fatalf("TreeFromNodes: %v", err)
	}
	root := tree.Hash()

	indices := []int{11, 9}
	proof, err := tree.ProveMulti(indices)
	if err != nil {
		t.Fatalf("ProveMulti: %v", err)
	}
	if ok, err := VerifyMultiproof(root, proof.Hashes, proof.Leaves, proof.Indices); err != nil || !ok {
		t.Fatalf("proof does not verify before mutation: ok=%v err=%v", ok, err)
	}

	indices[0], indices[1] = indices[1], indices[0]

	if proof.Indices[0] != 11 || proof.Indices[1] != 9 {
		t.Fatalf("proof.Indices = %v; caller mutation leaked into the proof", proof.Indices)
	}
	if ok, err := VerifyMultiproof(root, proof.Hashes, proof.Leaves, proof.Indices); err != nil || !ok {
		t.Fatalf("proof does not verify after the caller reordered its own slice: ok=%v err=%v", ok, err)
	}
}

// finalizeTestChunks returns n distinct 32-byte chunks.
func finalizeTestChunks(n int) [][]byte {
	chunks := make([][]byte, n)
	for i := range chunks {
		chunks[i] = sum256ToBytes([]byte{byte(i), byte(i >> 8)})
	}
	return chunks
}

// assertAllBranchesHashed fails the test if any branch node in the subtree has
// no cached value.
func assertAllBranchesHashed(t *testing.T, tree *Node) {
	t.Helper()
	assertBranchesHashed(t, tree.n)
}

func assertBranchesHashed(t *testing.T, n *node) {
	t.Helper()
	if n == nil || (n.left == nil && n.right == nil) {
		return
	}
	if !n.hasValue {
		t.Fatal("branch node has no cached value after finalization")
	}
	if n.isVisited {
		t.Fatal("branch node still carries the pending flag after finalization")
	}
	assertBranchesHashed(t, n.left)
	assertBranchesHashed(t, n.right)
}

// nodesOf returns the nodes behind the given handles.
func nodesOf(handles []*Node) []*node {
	nodes := make([]*node, len(handles))
	for i, handle := range handles {
		nodes[i] = handle.n
	}
	return nodes
}

// Batched finalization must produce the same roots as the recursive hashNode
// path across tree shapes, both below and above finalizeThreshold.
func TestFinalizeBatchedShapes(t *testing.T) {
	leaves := func(n int) []*Node {
		chunks := finalizeTestChunks(n)
		nodes := make([]*Node, n)
		for i := range nodes {
			nodes[i] = LeafFromBytes(chunks[i])
		}
		return nodes
	}

	tests := []struct {
		name  string
		build func() (*Node, error)
	}{
		{"chunks below threshold", func() (*Node, error) { return TreeFromChunks(finalizeTestChunks(8)) }},
		{"chunks above threshold", func() (*Node, error) { return TreeFromChunks(finalizeTestChunks(64)) }},
		{"padded vector", func() (*Node, error) { return TreeFromNodes(leaves(100), 128) }},
		{"progressive", func() (*Node, error) { return TreeFromNodesProgressive(leaves(33)) }},
		{"mixin", func() (*Node, error) { return TreeFromNodesWithMixin(leaves(20), 20, 32) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := tt.build()
			if err != nil {
				t.Fatalf("failed to build reference tree: %v", err)
			}
			want := bytes.Clone(refHash(ref))

			tree, err := tt.build()
			if err != nil {
				t.Fatalf("failed to build tree: %v", err)
			}
			if got := tree.Hash(); !bytes.Equal(got, want) {
				t.Fatalf("Hash() = %x, want %x", got, want)
			}
			assertAllBranchesHashed(t, tree)
		})
	}
}

// Finalization must skip already-hashed subtrees: hashing a child subtree
// first leaves the tree partially cached, and the root hash must still match.
func TestFinalizePartialSubtree(t *testing.T) {
	chunks := finalizeTestChunks(64)

	ref, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to build reference tree: %v", err)
	}
	want := bytes.Clone(refHash(ref))

	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to build tree: %v", err)
	}
	if got := tree.Left().Hash(); !bytes.Equal(got, refNodeHash(ref.n.left)) {
		t.Fatalf("left subtree hash mismatch: %x", got)
	}
	if got := tree.Hash(); !bytes.Equal(got, want) {
		t.Fatalf("Hash() = %x, want %x", got, want)
	}
	assertAllBranchesHashed(t, tree)
}

// A failing batch hash backend aborts finalization with its error; the
// partially finalized tree resumes cleanly once the backend recovers.
func TestFinalizeBatchHashFnError(t *testing.T) {
	restore := replaceSHA256Backend(func(_, _ []byte) error { return errors.New("backend failure") })
	defer restore()

	chunks := finalizeTestChunks(64)

	ref, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to build reference tree: %v", err)
	}
	want := bytes.Clone(refHash(ref))

	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to build tree: %v", err)
	}
	if finalizeErr := tree.Finalize(); finalizeErr == nil {
		t.Fatal("expected the backend error")
	}
	restore()
	if finalizeErr := tree.Finalize(); finalizeErr != nil {
		t.Fatalf("resumed Finalize: %v", finalizeErr)
	}
	if got := tree.Hash(); !bytes.Equal(got, want) {
		t.Fatalf("Hash() = %x, want %x", got, want)
	}
	assertAllBranchesHashed(t, tree)
}

// A finalized tree is read-only: concurrent Prove/ProveMulti/Hash/Value calls
// must agree on the root and produce verifiable proofs. Needs -race (which CI
// uses) to detect an unsynchronized write sneaking back into the read paths.
func TestFinalizedTreeConcurrentUse(t *testing.T) {
	const numLeaves = 32

	chunks := finalizeTestChunks(numLeaves)
	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to build tree: %v", err)
	}
	want := tree.Hash()

	const numGoroutines = 16
	var wg sync.WaitGroup
	errCh := make(chan error, numGoroutines)

	for g := range numGoroutines {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()

			if got := tree.Hash(); !bytes.Equal(got, want) {
				errCh <- fmt.Errorf("goroutine %d: Hash() = %x, want %x", g, got, want)
				return
			}

			leafIdx := numLeaves + g%numLeaves
			proof, err := tree.Prove(leafIdx)
			if err != nil {
				errCh <- fmt.Errorf("goroutine %d: Prove(%d): %v", g, leafIdx, err)
				return
			}
			if ok, verifyErr := VerifyProof(want, proof); verifyErr != nil || !ok {
				errCh <- fmt.Errorf("goroutine %d: VerifyProof(%d) = %v, %v", g, leafIdx, ok, verifyErr)
				return
			}

			multiIndices := []int{numLeaves + g%numLeaves, numLeaves + (g+3)%numLeaves}
			multiProof, err := tree.ProveMulti(multiIndices)
			if err != nil {
				errCh <- fmt.Errorf("goroutine %d: ProveMulti(%v): %v", g, multiIndices, err)
				return
			}
			if ok, verifyErr := VerifyMultiproof(want, multiProof.Hashes, multiProof.Leaves, multiProof.Indices); verifyErr != nil || !ok {
				errCh <- fmt.Errorf("goroutine %d: VerifyMultiproof(%v) = %v, %v", g, multiIndices, ok, verifyErr)
				return
			}

			if got := tree.Value(); !bytes.Equal(got, want) {
				errCh <- fmt.Errorf("goroutine %d: Value() = %x, want %x", g, got, want)
			}
		}(g)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

// A malformed node (one nil child) hard-aborts finalization: the error is
// returned immediately and nothing further is hashed — batches collected
// before the abort stay pending, and their nodes are restored to unhashed.
func TestFinalizeMalformedSubtree(t *testing.T) {
	healthy, err := TreeFromChunks(finalizeTestChunks(32))
	if err != nil {
		t.Fatalf("failed to build healthy subtree: %v", err)
	}
	malformed := NewNodeWithLR(LeafFromBytes(finalizeTestChunks(1)[0]), nil)
	root := NewNodeWithLR(healthy, malformed)

	if finalizeErr := root.Finalize(); finalizeErr == nil {
		t.Fatal("expected the malformed-tree error")
	}

	if root.n.hasValue {
		t.Error("root above a malformed subtree must stay unhashed")
	}
	if malformed.n.hasValue {
		t.Error("malformed node must stay unhashed")
	}
	if healthy.n.hasValue {
		t.Error("hard abort must not hash the sibling walked before the malformed node")
	}
	if healthy.n.isVisited {
		t.Error("pending flag must be cleared when the abort discards the batches")
	}

	// The healthy subtree still finalizes on its own.
	if finalizeErr := healthy.Finalize(); finalizeErr != nil {
		t.Fatalf("healthy subtree Finalize: %v", finalizeErr)
	}
	assertAllBranchesHashed(t, healthy)
}

// Prove on a malformed tree returns an error instead of panicking, matching
// the traversal's own nil-child reporting.
func TestProveMalformedTreeStillErrors(t *testing.T) {
	root := NewNodeWithLR(nil, LeafFromBytes(finalizeTestChunks(1)[0]))

	if _, err := root.Prove(3); err == nil {
		t.Fatal("expected error for malformed tree")
	}
}

// A nil root passes through finalize untouched; hashing then reports the
// incomplete tree.
func TestHashNilNode(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for nil node")
		}
	}()
	var n *Node
	_ = n.Hash()
}

// Large trees force many mid-walk batch flushes across depths; the batched
// result must match the recursive reference.
func TestFinalizeLargeTree(t *testing.T) {
	chunks := finalizeTestChunks(1 << 18)

	ref, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to build reference tree: %v", err)
	}
	want := bytes.Clone(refHash(ref))

	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to build tree: %v", err)
	}
	if err := tree.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if got := tree.Hash(); !bytes.Equal(got, want) {
		t.Fatalf("Hash() = %x, want %x", got, want)
	}
	assertAllBranchesHashed(t, tree)
}

// shortLeafNodes returns n leaves shorter than a full chunk; their chunks
// zero-extend during hashing.
func shortLeafNodes(n int) []*Node {
	leaves := make([]*Node, n)
	for i := range leaves {
		leaves[i] = NewNodeWithValue([]byte{byte(i + 1), byte(i >> 8)})
	}
	return leaves
}

// Short leaf values zero-pad at construction; a pooled gather buffer warmed
// with nonzero chunks must not leak stale bytes into the hashed input.
// Covers the batch path and, below threshold, the recursive path.
func TestFinalizeShortLeavesWarmedPool(t *testing.T) {
	for _, n := range []int{8, 32} {
		ref, err := TreeFromNodes(shortLeafNodes(n), n)
		if err != nil {
			t.Fatal(err)
		}
		want := bytes.Clone(refHash(ref))

		// Warm the scratch pool with nonzero 32-byte chunks.
		warmChunks := make([][]byte, 32)
		for i := range warmChunks {
			warmChunks[i] = bytes.Repeat([]byte{0xAA}, 32)
		}
		warm, err := TreeFromChunks(warmChunks)
		if err != nil {
			t.Fatal(err)
		}
		_ = warm.Hash()

		tree, err := TreeFromNodes(shortLeafNodes(n), n)
		if err != nil {
			t.Fatal(err)
		}
		if got := tree.Hash(); !bytes.Equal(got, want) {
			t.Fatalf("n=%d: batched root %x != recursive root %x", n, got, want)
		}
	}
}

// Same stale-bytes condition across many mid-walk flushes.
func TestFinalizeShortLeavesLarge(t *testing.T) {
	// Dirty the pooled buffers with a normal finalize first.
	warm, err := TreeFromChunks(finalizeTestChunks(1 << 16))
	if err != nil {
		t.Fatal(err)
	}
	if finErr := warm.Finalize(); finErr != nil {
		t.Fatalf("Finalize: %v", finErr)
	}

	const numLeaves = 4 * finalizeBatchPairs // several full batches on the leaf-parent level
	ref, err := TreeFromNodes(shortLeafNodes(numLeaves), numLeaves)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Clone(refHash(ref))

	tree, err := TreeFromNodes(shortLeafNodes(numLeaves), numLeaves)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if got := tree.Hash(); !bytes.Equal(got, want) {
		t.Fatalf("batched root %x != recursive root %x", got, want)
	}
}

// A backend failure on a mid-walk flush aborts the walk and fails Finalize;
// the tree resumes cleanly once the backend recovers.
func TestFinalizeMidwalkError(t *testing.T) {
	restore := replaceSHA256Backend(func(_, _ []byte) error { return errors.New("backend failure") })
	defer restore()

	chunks := finalizeTestChunks(4 * finalizeBatchPairs)

	ref, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Clone(refHash(ref))

	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if finalizeErr := tree.Finalize(); finalizeErr == nil {
		t.Fatal("expected the backend error")
	}
	restore()
	if finalizeErr := tree.Finalize(); finalizeErr != nil {
		t.Fatalf("resumed Finalize: %v", finalizeErr)
	}
	if got := tree.Hash(); !bytes.Equal(got, want) {
		t.Fatalf("Hash() = %x, want %x", got, want)
	}
	assertAllBranchesHashed(t, tree)
}

// Deep padded trees produce uneven batch sizes across depths; deepest-first
// flushing must still hash every child batch before its parents gather.
func TestFinalizeDeepPadded(t *testing.T) {
	leaves := func() []*Node {
		chunks := finalizeTestChunks(1 << 17)
		nodes := make([]*Node, len(chunks))
		for i := range nodes {
			nodes[i] = LeafFromBytes(chunks[i])
		}
		return nodes
	}

	ref, err := TreeFromNodes64(leaves(), 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Clone(refHash(ref))

	tree, err := TreeFromNodes64(leaves(), 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if got := tree.Hash(); !bytes.Equal(got, want) {
		t.Fatal("root mismatch")
	}
	assertAllBranchesHashed(t, tree)
}

// An unhashed subtree may be aliased into the tree at many positions
// (TreeFromNodes accepts existing nodes); revisiting it while it is still
// pending flushes the pending batches so its value is cached, and the root
// must match an alias-free reference.
func TestFinalizeSharedSubtree(t *testing.T) {
	build := func(share bool) *Node {
		leaves := make([]*Node, 2048)
		for i := range leaves {
			if share && i > 0 {
				leaves[i] = leaves[0]
				continue
			}
			leaves[i] = NewNodeWithLR(
				LeafFromBytes(sum256ToBytes([]byte{1})),
				LeafFromBytes(sum256ToBytes([]byte{2})),
			)
		}
		tree, err := TreeFromNodes(leaves, len(leaves))
		if err != nil {
			t.Fatalf("failed to build tree: %v", err)
		}
		return tree
	}

	shared := build(true)
	if finalizeErr := shared.Finalize(); finalizeErr != nil {
		t.Fatalf("Finalize: %v", finalizeErr)
	}
	if got, want := shared.Hash(), build(false).Hash(); !bytes.Equal(got, want) {
		t.Fatalf("shared-subtree root %x, want %x", got, want)
	}
	assertAllBranchesHashed(t, shared)
}

// Hash finalizes an unfinalized tree, and hashNode caches every branch it
// computes, so a backend refusal has to stop it: completing the tree behind
// the refusing backend would cache -- and from then on answer with -- a root
// that backend never produced. It returns no root instead, and the tree stays
// resumable.
func TestHashAfterRefusedBackendCachesNothing(t *testing.T) {
	// A backend that answers differently from the built-in compression, so a
	// root it did not produce is recognisable, and refuses while refuse is set.
	var refuse bool
	backend := func(dst, input []byte) error {
		if refuse {
			return errors.New("backend failure")
		}
		if err := hasher.NativeHashWrapperFactory(sha256.New)(dst, input); err != nil {
			return err
		}
		dst[0] ^= 0xff

		return nil
	}

	original := hasher.FastHasherPool.HashFn
	defer func() { hasher.FastHasherPool.HashFn = original }()
	hasher.FastHasherPool.HashFn = backend

	chunks := finalizeTestChunks(8)
	build := func() *Node {
		tree, err := TreeFromChunks(chunks)
		if err != nil {
			t.Fatal(err)
		}

		return tree
	}

	want := bytes.Clone(build().Hash())
	if want == nil {
		t.Fatal("the reference tree has no root")
	}

	tree := build()
	refuse = true
	if got := tree.Hash(); got != nil {
		t.Errorf("Hash under a refusing backend = %x, want no root", got)
	}

	refuse = false
	if got := tree.Hash(); !bytes.Equal(got, want) {
		t.Errorf("resumed root %x, want %x", got, want)
	}
}

// The cross-batch alias case: the shared branch's first occurrence is
// hashed by an earlier mid-walk flush before the walk meets the alias, so
// the revisit sees a cached value and batching continues without an abort.
func TestFinalizeSharedSubtreeCrossBatch(t *testing.T) {
	build := func(share bool) *Node {
		// Wide enough that the shared node's batch flushes before the walk
		// revisits it.
		leaves := make([]*Node, 4*finalizeBatchPairs)
		for i := range leaves {
			if i == 2*finalizeBatchPairs {
				if share {
					leaves[i] = leaves[0]
					continue
				}
				// The alias-free reference carries position 0's content here.
				leaves[i] = NewNodeWithLR(
					LeafFromBytes(sum256ToBytes([]byte{0, 0, 1})),
					LeafFromBytes(sum256ToBytes([]byte{0, 0, 2})),
				)
				continue
			}
			leaves[i] = NewNodeWithLR(
				LeafFromBytes(sum256ToBytes([]byte{byte(i), byte(i >> 8), 1})),
				LeafFromBytes(sum256ToBytes([]byte{byte(i), byte(i >> 8), 2})),
			)
		}
		tree, err := TreeFromNodes(leaves, len(leaves))
		if err != nil {
			t.Fatalf("failed to build tree: %v", err)
		}
		return tree
	}

	shared := build(true)
	if err := shared.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	if got, want := shared.Hash(), build(false).Hash(); !bytes.Equal(got, want) {
		t.Fatalf("cross-batch alias root %x, want %x", got, want)
	}
	assertAllBranchesHashed(t, shared)
}

// Batching survives an alias: the flush caches the shared node's value and
// the rest of the tree keeps hashing in vectorized batches, so an alias met
// early in the walk costs one partial flush — not per-pair calls for the
// whole remainder.
func TestFinalizeSharedSubtreeKeepsBatching(t *testing.T) {
	build := func(share bool) *Node {
		leaves := make([]*Node, 4*finalizeBatchPairs)
		for i := range leaves {
			if i == 1 {
				if share {
					leaves[i] = leaves[0]
					continue
				}
				// The alias-free reference carries position 0's content here.
				leaves[i] = NewNodeWithLR(
					LeafFromBytes(sum256ToBytes([]byte{0, 0, 1})),
					LeafFromBytes(sum256ToBytes([]byte{0, 0, 2})),
				)
				continue
			}
			leaves[i] = NewNodeWithLR(
				LeafFromBytes(sum256ToBytes([]byte{byte(i), byte(i >> 8), 1})),
				LeafFromBytes(sum256ToBytes([]byte{byte(i), byte(i >> 8), 2})),
			)
		}
		tree, err := TreeFromNodes(leaves, len(leaves))
		if err != nil {
			t.Fatalf("failed to build tree: %v", err)
		}
		return tree
	}

	shared := build(true)
	var calls int
	countingFn := func(dst, input []byte) error {
		calls++
		return batchHashFn(dst, input)
	}
	if err := shared.Finalize(WithHashFn(countingFn)); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// ~8k branches batch into ~8 full flushes plus the alias flush and the
	// trailing per-depth partials; per-pair hashing would take thousands.
	if calls > 64 {
		t.Fatalf("finalize used %d hash calls; batching did not survive the alias", calls)
	}
	if got, want := shared.Hash(), build(false).Hash(); !bytes.Equal(got, want) {
		t.Fatalf("aliased root %x, want %x", got, want)
	}
	assertAllBranchesHashed(t, shared)
}

// A malformed node aborts before any hashing here: a caller-supplied
// backend is never invoked for a tree that cannot finalize completely.
func TestFinalizeMalformedSiblingCustomFn(t *testing.T) {
	healthy, err := TreeFromChunks(finalizeTestChunks(32))
	if err != nil {
		t.Fatalf("failed to build healthy subtree: %v", err)
	}
	root := NewNodeWithLR(healthy, NewNodeWithLR(LeafFromBytes(finalizeTestChunks(1)[0]), nil))

	var calls int
	finalizeErr := root.Finalize(WithHashFn(func(_, _ []byte) error { calls++; return nil }))
	if finalizeErr == nil {
		t.Fatal("expected the malformed-tree error")
	}
	if calls != 0 {
		t.Errorf("hash function ran %d times before the abort", calls)
	}
}

// A pre-finalized subtree met during the walk is trusted as final and
// batching continues past it.
func TestFinalizePrehashedSubtree(t *testing.T) {
	chunks := finalizeTestChunks(4 * finalizeBatchPairs)

	ref, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to build reference tree: %v", err)
	}
	want := bytes.Clone(refHash(ref))

	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to build tree: %v", err)
	}
	if err := tree.Right().Finalize(); err != nil {
		t.Fatalf("subtree Finalize: %v", err)
	}
	if err := tree.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if got := tree.Hash(); !bytes.Equal(got, want) {
		t.Fatalf("prehashed-subtree root %x, want %x", got, want)
	}
	assertAllBranchesHashed(t, tree)
}

// Concurrent finalizations of distinct trees share the scratch pool; each
// tree must still finalize completely and agree with its reference root.
// Needs -race.
func TestFinalizeConcurrentTrees(t *testing.T) {
	const trees = 4
	chunks := finalizeTestChunks(1 << 15)

	ref, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Clone(refHash(ref))

	var wg sync.WaitGroup
	errs := make([]error, trees)
	roots := make([][]byte, trees)
	for i := 0; i < trees; i++ {
		tree, err := TreeFromChunks(chunks)
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(i int, tree *Node) {
			defer wg.Done()
			if errs[i] = tree.Finalize(); errs[i] == nil {
				roots[i] = tree.Hash()
			}
		}(i, tree)
	}
	wg.Wait()
	for i := 0; i < trees; i++ {
		if errs[i] != nil {
			t.Fatalf("tree %d: Finalize: %v", i, errs[i])
		}
		if !bytes.Equal(roots[i], want) {
			t.Fatalf("tree %d: root %x, want %x", i, roots[i], want)
		}
	}
}

// hashNodeFn reports malformed nodes as errors.
func TestHashNodeFnMalformed(t *testing.T) {
	h := hashingOf(hasher.SHA256)
	if err := hashNodeFn(nil, &h); err == nil {
		t.Error("expected error for nil node")
	}
	malformed := NewNodeWithLR(LeafFromBytes(finalizeTestChunks(1)[0]), nil)
	if err := hashNodeFn(malformed.n, &h); err == nil {
		t.Error("expected error for branch with a single child")
	}
}

// A custom hash function failing on the recursive path surfaces from every
// position: the left recursion, the right recursion and the node's own
// hash. Each abort leaves a resumable tree.
func TestFinalizeRecursiveTailErrors(t *testing.T) {
	// Four chunks stay below finalizeThreshold, so hashNodeFn hashes the two
	// child branches (calls 1 and 2) and then the root (call 3).
	chunks := finalizeTestChunks(4)

	ref, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Clone(refHash(ref))

	for _, tc := range []struct {
		name     string
		failCall int
	}{
		{"left_child", 1},
		{"right_child", 2},
		{"own_hash", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			fn := func(dst, input []byte) error {
				calls++
				if calls == tc.failCall {
					return errors.New("backend failure")
				}
				return batchHashFn(dst, input)
			}
			tree, err := TreeFromChunks(chunks)
			if err != nil {
				t.Fatal(err)
			}
			if finalizeErr := tree.Finalize(WithHashFn(fn)); finalizeErr == nil {
				t.Fatal("expected the backend error")
			}
			if finalizeErr := tree.Finalize(); finalizeErr != nil {
				t.Fatalf("resumed Finalize: %v", finalizeErr)
			}
			if got := tree.Hash(); !bytes.Equal(got, want) {
				t.Fatalf("resumed root %x, want %x", got, want)
			}
			assertAllBranchesHashed(t, tree)
		})
	}
}

// Proof navigation through a cached-value branch with a missing child
// reports the node as not found instead of panicking. The cached value
// keeps finalize from rejecting the malformed shape first.
func TestProveNavigationMissingChild(t *testing.T) {
	leaf := LeafFromBytes(finalizeTestChunks(1)[0])
	cached := sum256ToBytes([]byte{9})

	missingSibling := &Node{n: &node{right: leaf.n, hasValue: true}, alg: hasher.SHA256}
	copy(missingSibling.n.value[:], cached)
	if _, err := missingSibling.Prove(3); err == nil {
		t.Error("expected error for missing left sibling")
	}

	missingTarget := &Node{n: &node{left: leaf.n, hasValue: true}, alg: hasher.SHA256}
	copy(missingTarget.n.value[:], cached)
	if _, err := missingTarget.Prove(3); err == nil {
		t.Error("expected error for missing right child")
	}
	if _, err := missingTarget.ProveMulti([]int{2}); err == nil {
		t.Error("expected error for a required sibling outside the tree")
	}
}

// The Node layout is 56 bytes on 64-bit platforms: two pointers, the inline
// 32-byte value and three flags swallowed by the struct padding. A size
// regression means a field was added or reordered carelessly; the node size
// directly bounds a mainnet-scale tree's memory footprint.
func TestNodeSize(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("layout pin applies to 64-bit platforms")
	}
	if size := unsafe.Sizeof(node{}); size != 56 {
		t.Fatalf("Node size = %d bytes, want 56", size)
	}
}

// A diamond-shaped DAG shares one node at every level; finalization must
// stay linear in the depth (memoized through cached values), not go
// exponential, and the root must equal the strict-tree equivalent.
func TestFinalizeDiamondDAG(t *testing.T) {
	const levels = 40
	leafData := sum256ToBytes([]byte("diamond"))

	node := LeafFromBytes(leafData)
	for range levels {
		node = NewNodeWithLR(node, node)
	}
	if err := node.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	want := bytes.Clone(leafData)
	for range levels {
		want = hashPair(want, want)
	}
	if got := node.Hash(); !bytes.Equal(got, want) {
		t.Fatalf("diamond root %x, want %x", got, want)
	}
}

// A leaked isVisited flag (unreachable through the public API — the
// deferred cleanup clears pending batches on every exit) must never cost
// correctness: the walker reads it as an alias, and when the flush does not
// produce the node's value it scrubs the flag and walks the subtree
// normally, computing the correct root.
func TestFinalizeStaleVisitedFlag(t *testing.T) {
	chunks := finalizeTestChunks(64)

	ref, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Clone(refHash(ref))

	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	tree.n.left.isVisited = true
	if finalizeErr := tree.Finalize(); finalizeErr != nil {
		t.Fatalf("Finalize: %v", finalizeErr)
	}
	if got := tree.Hash(); !bytes.Equal(got, want) {
		t.Fatalf("Hash() = %x, want %x", got, want)
	}
	assertAllBranchesHashed(t, tree)
}

// A panic escaping the hash function unwinds through Finalize; the deferred
// scratch release still clears the pending flags, so a recovering caller is
// left with a consistent tree that re-finalizes to the correct root.
func TestFinalizeRecoveredFnPanic(t *testing.T) {
	chunks := finalizeTestChunks(64)

	ref, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Clone(refHash(ref))

	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected the fn panic to propagate")
			}
		}()
		_ = tree.Finalize(WithHashFn(func(_, _ []byte) error { panic("fn panic") }))
	}()

	if finalizeErr := tree.Finalize(); finalizeErr != nil {
		t.Fatalf("re-finalize: %v", finalizeErr)
	}
	if got := tree.Hash(); !bytes.Equal(got, want) {
		t.Fatalf("Hash() = %x, want %x", got, want)
	}
	assertAllBranchesHashed(t, tree)
}

// BenchmarkFinalize measures finalization alone on a prebuilt 65,536-leaf
// tree; construction is untimed.
func BenchmarkFinalize(b *testing.B) {
	chunks := finalizeTestChunks(1 << 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tree, err := TreeFromChunks(chunks)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if err := tree.Finalize(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBuildFinalize measures tree construction plus finalization for
// 65,536 leaves — the end-to-end cost a GetTree caller pays per tree.
func BenchmarkBuildFinalize(b *testing.B) {
	chunks := finalizeTestChunks(1 << 16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := TreeFromChunks(chunks)
		if err != nil {
			b.Fatal(err)
		}
		if err := tree.Finalize(); err != nil {
			b.Fatal(err)
		}
	}
}

// Errors on the aliased path surface like on the plain path: from the
// pending-batch flush after the alias stopped batching (call 1 hashes the
// shared branch) and from the recursive remainder pass (call 2 hashes the
// root). Both leave a resumable tree.
func TestFinalizeAliasedErrors(t *testing.T) {
	leafA := sum256ToBytes([]byte{1})
	leafB := sum256ToBytes([]byte{2})
	want := hashPair(hashPair(leafA, leafB), hashPair(leafA, leafB))

	for _, tc := range []struct {
		name     string
		failCall int
	}{
		{"pending_flush", 1},
		{"recursive_tail", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shared := NewNodeWithLR(LeafFromBytes(leafA), LeafFromBytes(leafB))
			tree := NewNodeWithLR(shared, shared)

			var calls int
			fn := func(dst, input []byte) error {
				calls++
				if calls == tc.failCall {
					return errors.New("backend failure")
				}
				return batchHashFn(dst, input)
			}
			if finalizeErr := tree.Finalize(WithHashFn(fn)); finalizeErr == nil {
				t.Fatal("expected the backend error")
			}
			if finalizeErr := tree.Finalize(); finalizeErr != nil {
				t.Fatalf("resumed Finalize: %v", finalizeErr)
			}
			if got := tree.Hash(); !bytes.Equal(got, want) {
				t.Fatalf("resumed root %x, want %x", got, want)
			}
			assertAllBranchesHashed(t, tree)
		})
	}
}

// Each side of a compression owns its data: editing one proof never edits the
// other, in either direction.
func TestCompressDecompressOwnData(t *testing.T) {
	proof := &Multiproof{
		Indices: []int{4, 5},
		Leaves:  [][]byte{sum256ToBytes([]byte("leaf1")), sum256ToBytes([]byte("leaf2"))},
		Hashes:  [][]byte{hasher.GetZeroHash(0), sum256ToBytes([]byte("sibling"))},
	}
	compressed := proof.Compress()
	compressed.Indices[0] = 99
	compressed.Leaves[0][0] ^= 0xff
	compressed.Hashes[1][0] ^= 0xff
	if proof.Indices[0] != 4 || proof.Leaves[0][0] == compressed.Leaves[0][0] || proof.Hashes[1][0] == compressed.Hashes[1][0] {
		t.Fatal("editing the compressed proof changed the original")
	}

	back := proof.Compress().Decompress()
	back.Indices[1] = 98
	back.Leaves[1][0] ^= 0xff
	back.Hashes[0][0] ^= 0xff
	back.Hashes[1][0] ^= 0xff
	if proof.Indices[1] != 5 || proof.Leaves[1][0] == back.Leaves[1][0] || proof.Hashes[1][0] == back.Hashes[1][0] {
		t.Fatal("editing the decompressed proof changed the original")
	}
	if !bytes.Equal(hasher.GetZeroHash(0), make([]byte, 32)) {
		t.Fatal("editing a decompressed zero hash changed the shared table")
	}
}

// Only padding is empty: a leaf holding an all-zero value is data, and no
// proof exists below either kind of leaf.
func TestLeafEmptyOnlyForPadding(t *testing.T) {
	zero := NewNodeWithValue(make([]byte, 32))
	if zero.IsEmpty() {
		t.Fatal("a zero-valued leaf reports itself as padding")
	}
	if !EmptyLeaf().IsEmpty() || !NewEmptyNode(hasher.GetZeroHash(0)).IsEmpty() {
		t.Fatal("padding leaves do not report themselves as padding")
	}
	root, err := TreeFromNodes([]*Node{zero, NewNodeWithValue([]byte{1})}, 2)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	if _, err := root.Prove(4); err == nil {
		t.Fatal("a proof below a zero-valued leaf was produced")
	}
	if _, err := root.Prove(2); err != nil {
		t.Fatalf("proof of the zero-valued leaf: %v", err)
	}
}

// Both walkers refuse a failing hash function. The walkers are required to
// agree, and a root one of them builds from bytes the hash function never
// produced would be a silent disagreement: the tree reports the failure, so
// the hasher does too.
func TestBothWalkersRefuseAFailingHashFn(t *testing.T) {
	backend := errors.New("hash backend unavailable")
	failing := func(_, _ []byte) error { return backend }

	fill := func(w sszutils.HashWalker) {
		idx := w.Index()
		for i := 0; i < 32; i++ {
			w.PutUint64(uint64(i))
		}
		w.Merkleize(idx)
	}

	hh := hasher.NewHasherWithHashFn(failing) //nolint:staticcheck // the walkers are compared on the function hook
	fill(hh)
	if _, err := hh.HashRoot(); !errors.Is(err, backend) {
		t.Fatalf("hasher.HashRoot err = %v, want %v", err, backend)
	}

	w := NewWrapper()
	fill(w)
	root, err := w.Root()
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if err := root.Finalize(WithHashFn(failing)); !errors.Is(err, backend) {
		t.Fatalf("Finalize err = %v, want %v", err, backend)
	}
}

// hashPair is the SHA256 compression of two chunks.
func hashPair(left, right []byte) []byte {
	var input [64]byte
	copy(input[:32], left)
	copy(input[32:], right)

	sum := sha256.Sum256(input[:])
	return sum[:]
}

// refHash is the root of a SHA256 tree computed node by node, without the
// finalization under test and without caching anything in the tree.
func refHash(n *Node) []byte {
	return refNodeHash(n.n)
}

func refNodeHash(n *node) []byte {
	switch {
	case n == nil:
		panic("Tree incomplete")
	case n.isEmpty && !n.hasValue:
		return hasher.SHA256.ZeroHash(int(n.depth))
	case n.left == nil && n.right == nil:
		return bytes.Clone(n.value[:])
	case n.left == nil || n.right == nil:
		panic("Tree incomplete")
	}
	return hashPair(refNodeHash(n.left), refNodeHash(n.right))
}

// batchHashFn is the function a SHA256 tree is finalized with by default.
func batchHashFn(dst, input []byte) error {
	return hasher.SHA256.HashFn()(dst, input)
}

// replaceSHA256Backend makes SHA256 trees hash with fn until the returned
// function is called.
func replaceSHA256Backend(fn hasher.HashFn) (restore func()) {
	installed := hasher.FastHasherPool.HashFn
	hasher.FastHasherPool.HashFn = fn

	return func() { hasher.FastHasherPool.HashFn = installed }
}

// The package names no hash of its own: every hash and every zero subtree
// root comes from the Algorithm of the tree at hand. SHA256 appears once, as
// the Algorithm an entry point works with when none is named.
func TestPackageNamesNoHash(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the package directory: %v", err)
	}
	forbidden := []string{"sha256.", "sha3.", "hasher.GetZeroHash", "hasher.FastHasherPool", "hasher.DefaultHasherPool", "hasher.Keccak256", "hasher.SHA256Native"}
	defaults := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(name)
		if readErr != nil {
			t.Fatalf("read %s: %v", name, readErr)
		}
		for i, line := range strings.Split(string(source), "\n") {
			code, _, _ := strings.Cut(line, "//")
			for _, word := range forbidden {
				if strings.Contains(code, word) {
					t.Errorf("%s:%d names a hash: %s", name, i+1, strings.TrimSpace(line))
				}
			}
			defaults += strings.Count(code, "hasher.SHA256")
		}
	}
	if defaults != 1 {
		t.Errorf("hasher.SHA256 is named %d times in the package, want once as the default", defaults)
	}
}

// Padding nodes are shared by the trees of every hash and hold no root of
// their own: each tree answers for them with the zero subtree roots of its
// Algorithm.
func TestPaddingNodesAreShared(t *testing.T) {
	var shared *node
	for name, alg := range testAlgorithms(t) {
		tree, _ := algorithmTree(t, alg, paddedListWalk)
		// The right half of the 64-leaf data tree is one zero subtree.
		empty, err := tree.Get(5)
		if err != nil {
			t.Fatalf("%s: Get: %v", name, err)
		}
		if shared == nil {
			shared = empty.n
		}
		if empty.n != shared || empty.n != emptyNode(5) || empty.n.hasValue {
			t.Errorf("%s: the padding node is not the shared, valueless one", name)
		}
		if !empty.IsEmpty() || !empty.IsLeaf() || empty.Left() != nil || empty.Right() != nil || empty.Algorithm() != alg {
			t.Errorf("%s: a padding node is not shown as an empty leaf of its tree", name)
		}
		want := alg.ZeroHash(5)
		if !bytes.Equal(empty.Hash(), want) || !bytes.Equal(empty.Value(), want) {
			t.Errorf("%s: padding root %x, want %x", name, empty.Hash(), want)
		}
		below, err := tree.Get(5 << 5)
		if err != nil || !bytes.Equal(below.Hash(), make([]byte, 32)) {
			t.Errorf("%s: the leaf below the padding = %v, %v; want the zero chunk", name, below, err)
		}
		if _, err = tree.Get(5 << 6); err == nil {
			t.Errorf("%s: found a node below the zero chunk", name)
		}
		tree.Show(0)

		// A padding node made from a zero subtree root of the Algorithm is
		// the shared one of that depth.
		loose := NewEmptyNode(alg.ZeroHash(3), WithAlgorithm(alg))
		if loose.n != emptyNode(3) {
			t.Errorf("%s: NewEmptyNode did not resolve the depth of its root", name)
		}
		child, err := loose.Get(2)
		if err != nil || !bytes.Equal(child.Hash(), alg.ZeroHash(2)) {
			t.Errorf("%s: child of a padding node = %v, %v", name, child, err)
		}
	}
}

// A generated proof must not alias the process-wide zero-hash table. Empty
// siblings and zero-padding leaves hold the shared GetZeroHash slice, so handing
// the raw bytes to a caller would let a proof mutation corrupt every subsequent
// root computation in the process.
func TestProveDoesNotAliasZeroHashTable(t *testing.T) {
	leaves := []*Node{
		LeafFromUint64(1),
		LeafFromUint64(2),
		LeafFromUint64(3),
	}
	tree, err := TreeFromNodes(leaves, 4)
	if err != nil {
		t.Fatalf("TreeFromNodes: %v", err)
	}

	// gindex 6 is the third leaf; its sibling (gindex 7) is zero-padding, so the
	// proof carries the depth-0 zero hash.
	proof, err := tree.Prove(6)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}

	zero := append([]byte(nil), hasher.GetZeroHash(0)...)
	mutated := false
	for _, h := range proof.Hashes {
		if bytes.Equal(h, zero) {
			h[0] ^= 0xFF
			mutated = true
		}
	}
	if !mutated {
		t.Fatal("expected a zero-hash sibling in the proof")
	}
	if !bytes.Equal(hasher.GetZeroHash(0), zero) {
		t.Fatalf("proof mutation corrupted the shared zero-hash table: %x", hasher.GetZeroHash(0))
	}

	// Node.Value() returns a copy for the same reason.
	v := EmptyLeaf().Value()
	v[0] ^= 0xFF
	if !bytes.Equal(hasher.GetZeroHash(0), zero) {
		t.Fatalf("Value() mutation corrupted the shared zero-hash table: %x", hasher.GetZeroHash(0))
	}
}

// A decompressed proof owns its hashes: writing to one leaves the shared zero
// hashes untouched.
func TestDecompressClonesZeroHashes(t *testing.T) {
	proof := &Multiproof{
		Indices: []int{2, 3},
		Leaves:  [][]byte{make([]byte, 32), make([]byte, 32)},
		Hashes:  [][]byte{make([]byte, 32)},
	}
	decompressed := proof.Compress().Decompress()
	if len(decompressed.Hashes) != 1 {
		t.Fatalf("decompressed %d hashes, want 1", len(decompressed.Hashes))
	}
	before := bytes.Clone(hasher.GetZeroHash(0))
	decompressed.Hashes[0][0] ^= 0xff
	if !bytes.Equal(hasher.GetZeroHash(0), before) {
		t.Fatal("writing a decompressed proof hash changed the shared zero hash")
	}
}

func TestChunkLimitDepth(t *testing.T) {
	cases := []struct {
		limit uint64
		depth int
	}{
		{0, 0}, {1, 0}, {2, 1}, {3, 2}, {4, 2}, {5, 3}, {8, 3}, {9, 4},
		{1 << 32, 32},
		{1 << 62, 62},
		{(1 << 63) - 1, 63},
		{1 << 63, 63},
		// Above 2^63 the next power of two (2^64) is unrepresentable; the deepest
		// tree is depth 64. This is the range that previously overflowed to a
		// negative int and crashed GetTree.
		{(1 << 63) + 1, 64},
		{^uint64(0), 64},
	}
	for _, c := range cases {
		if got := chunkLimitDepth(c.limit); got != c.depth {
			t.Errorf("chunkLimitDepth(%d) = %d, want %d", c.limit, got, c.depth)
		}
	}
}
