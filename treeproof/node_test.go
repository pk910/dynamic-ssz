// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package treeproof

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// A Node made without a constructor is a leaf of zeros in a SHA256 tree.
func TestZeroValueNode(t *testing.T) {
	zero := &Node{}
	if !zero.IsLeaf() || zero.IsEmpty() || zero.Left() != nil || zero.Right() != nil {
		t.Error("a zero-value Node is not a plain leaf")
	}
	if !bytes.Equal(zero.Hash(), zeroChunk[:]) || !bytes.Equal(zero.Value(), zeroChunk[:]) {
		t.Errorf("a zero-value Node holds %x, want the zero chunk", zero.Hash())
	}
	if zero.Algorithm() != hasher.SHA256 {
		t.Error("a zero-value Node names another Algorithm than SHA256")
	}
	if err := zero.Finalize(); err != nil {
		t.Errorf("Finalize: %v", err)
	}
	zero.Show(0)

	leaf := LeafFromUint64(7)
	branch := NewNodeWithLR(zero, leaf)
	if want := hashPair(zeroChunk[:], leaf.Hash()); !bytes.Equal(branch.Hash(), want) {
		t.Errorf("a branch over a zero-value Node hashes to %x, want %x", branch.Hash(), want)
	}
	proof, err := branch.Prove(2)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	if ok, verifyErr := VerifyProof(branch.Hash(), proof); verifyErr != nil || !ok {
		t.Errorf("VerifyProof = %v, %v", ok, verifyErr)
	}
}

// Handles to one node are the same node, whichever call returned them.
func TestNodeSame(t *testing.T) {
	left, right := LeafFromUint64(1), LeafFromUint64(2)
	tree := NewNodeWithLR(left, right)

	root, err := tree.Get(1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if root == tree || !root.Same(tree) {
		t.Error("Get(1) is not a new handle to the root")
	}
	first, second := tree.Left(), tree.Left()
	if first == second || !first.Same(second) || !first.Same(left) {
		t.Error("two handles to the left child are not the same node")
	}
	if tree.Left().Same(tree.Right()) || tree.Same(left) {
		t.Error("handles to different nodes are the same node")
	}
	var missing *Node
	if tree.Same(missing) || missing.Same(tree) {
		t.Error("a handle is the same node as none")
	}
	if root, err = missing.Get(1); root != nil || err != nil {
		t.Errorf("Get(1) of no node = %v, %v, want none", root, err)
	}
}

func TestNodeGet(t *testing.T) {
	// Create a simple tree
	leaf1 := NewNodeWithValue([]byte{1})
	leaf2 := NewNodeWithValue([]byte{2})
	leaf3 := NewNodeWithValue([]byte{3})
	leaf4 := NewNodeWithValue([]byte{4})

	node1 := NewNodeWithLR(leaf1, leaf2)
	node2 := NewNodeWithLR(leaf3, leaf4)
	root := NewNodeWithLR(node1, node2)

	tests := []struct {
		name        string
		index       int
		expectError bool
		expectValue []byte
	}{
		{
			name:        "get root",
			index:       1,
			expectError: false,
		},
		{
			name:        "get left child",
			index:       2,
			expectError: false,
		},
		{
			name:        "get right child",
			index:       3,
			expectError: false,
		},
		{
			name:        "get leaf 1",
			index:       4,
			expectError: false,
			expectValue: []byte{1},
		},
		{
			name:        "get leaf 2",
			index:       5,
			expectError: false,
			expectValue: []byte{2},
		},
		{
			name:        "get leaf 3",
			index:       6,
			expectError: false,
			expectValue: []byte{3},
		},
		{
			name:        "get leaf 4",
			index:       7,
			expectError: false,
			expectValue: []byte{4},
		},
		{
			name:        "get non-existent node",
			index:       8,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, err := root.Get(tt.index)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if node == nil {
				t.Fatal("node should not be nil")
			}

			if tt.expectValue != nil && !bytes.Equal(node.n.value[:1], tt.expectValue) {
				t.Errorf("expected value %v, got %v", tt.expectValue, node.n.value[:1])
			}
		})
	}
}

func TestNodeHash(t *testing.T) {
	// Test leaf node hash
	leafValue := bytes.Repeat([]byte{42}, 32)
	leaf := NewNodeWithValue(leafValue)

	hash := leaf.Hash()
	if !bytes.Equal(hash, leafValue) {
		t.Error("leaf node hash should be its value")
	}

	// Test branch node hash
	left := NewNodeWithValue(bytes.Repeat([]byte{1}, 32))
	right := NewNodeWithValue(bytes.Repeat([]byte{2}, 32))
	branch := NewNodeWithLR(left, right)

	expectedHash := sha256.Sum256(append(left.n.value[:], right.n.value[:]...))
	branchHash := branch.Hash()

	if !bytes.Equal(branchHash, expectedHash[:]) {
		t.Error("branch node hash mismatch")
	}

	// Test that hash is cached
	if !branch.n.hasValue {
		t.Error("hash should be cached in node value")
	}

	// Call hash again to ensure it returns cached value
	branchHash2 := branch.Hash()
	if !bytes.Equal(branchHash, branchHash2) {
		t.Error("cached hash should be returned")
	}
}

func TestNodeHashReturnsCopy(t *testing.T) {
	// Empty nodes alias the process-wide zero-hash table; Hash must hand out
	// a copy so a caller's mutation cannot corrupt it (and with it the roots
	// of every other tree sharing the cached empty nodes).
	const depth = 3
	empty := &Node{n: emptyNode(depth), alg: hasher.SHA256}
	zeroBefore := bytes.Clone(hasher.GetZeroHash(depth))

	h := empty.Hash()
	if !bytes.Equal(h, zeroBefore) {
		t.Fatalf("empty node hash mismatch: %x != %x", h, zeroBefore)
	}

	h[0] ^= 0xff
	if !bytes.Equal(hasher.GetZeroHash(depth), zeroBefore) {
		t.Fatal("mutating Hash() result corrupted the global zero-hash table")
	}
	if !bytes.Equal(empty.Hash(), zeroBefore) {
		t.Fatal("mutating Hash() result changed the node's stored hash")
	}

	// The same guarantee holds for regular leaf and cached branch values.
	leaf := NewNodeWithValue(bytes.Repeat([]byte{7}, 32))
	leafHash := leaf.Hash()
	leafHash[0] ^= 0xff
	if !bytes.Equal(leaf.Hash(), bytes.Repeat([]byte{7}, 32)) {
		t.Fatal("mutating Hash() result changed the leaf value")
	}
}

func TestNodeProve(t *testing.T) {
	// Create a simple tree with 4 leaves
	chunks := [][]byte{
		sum256ToBytes([]byte("leaf0")),
		sum256ToBytes([]byte("leaf1")),
		sum256ToBytes([]byte("leaf2")),
		sum256ToBytes([]byte("leaf3")),
	}

	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	tests := []struct {
		name        string
		index       int
		expectError bool
	}{
		{
			name:        "prove leaf 0",
			index:       4,
			expectError: false,
		},
		{
			name:        "prove leaf 3",
			index:       7,
			expectError: false,
		},
		{
			name:        "prove intermediate node",
			index:       2,
			expectError: false,
		},
		{
			name:        "prove root",
			index:       1,
			expectError: false,
		},
		{
			name:        "prove non-existent node",
			index:       10,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proof, err := tree.Prove(tt.index)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if proof == nil {
				t.Fatal("proof should not be nil")
			}

			if proof.Index != tt.index {
				t.Errorf("proof index mismatch: expected %d, got %d", tt.index, proof.Index)
			}

			// Verify proof structure
			expectedPathLen := getPathLength(tt.index)
			if len(proof.Hashes) != expectedPathLen {
				t.Errorf("expected %d hashes, got %d", expectedPathLen, len(proof.Hashes))
			}

			// Verify the proof
			rootHash := tree.Hash()
			valid, err := VerifyProof(rootHash, proof)
			if err != nil {
				t.Errorf("proof verification error: %v", err)
			}
			if !valid {
				t.Error("proof should be valid")
			}
		})
	}
}

func TestNodeProveMulti(t *testing.T) {
	// Create a tree with 8 leaves
	chunks := make([][]byte, 8)
	for i := 0; i < 8; i++ {
		hash := sha256.Sum256([]byte{byte(i)})
		chunks[i] = hash[:]
	}

	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatalf("failed to create tree: %v", err)
	}

	tests := []struct {
		name        string
		indices     []int
		expectError bool
	}{
		{
			name:        "prove single leaf",
			indices:     []int{8},
			expectError: false,
		},
		{
			name:        "prove two leaves",
			indices:     []int{8, 11},
			expectError: false,
		},
		{
			name:        "prove adjacent leaves",
			indices:     []int{8, 9},
			expectError: false,
		},
		{
			name:        "prove all leaves",
			indices:     []int{8, 9, 10, 11, 12, 13, 14, 15},
			expectError: false,
		},
		{
			name:        "prove with invalid index",
			indices:     []int{8, 20},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proof, err := tree.ProveMulti(tt.indices)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if proof == nil {
				t.Fatal("proof should not be nil")
			}

			// Verify proof structure
			if len(proof.Indices) != len(tt.indices) {
				t.Error("indices count mismatch")
			}
			if len(proof.Leaves) != len(tt.indices) {
				t.Error("leaves count mismatch")
			}

			// Verify the multiproof
			rootHash := tree.Hash()
			valid, err := VerifyMultiproof(rootHash, proof.Hashes, proof.Leaves, proof.Indices)
			if err != nil {
				t.Errorf("multiproof verification error: %v", err)
			}
			if !valid {
				t.Error("multiproof should be valid")
			}
		})
	}
}

func TestNodeGetters(t *testing.T) {
	// Create test nodes
	leafValue := []byte{42}
	leaf := NewNodeWithValue(leafValue)

	leftChild := NewNodeWithValue([]byte{1})
	rightChild := NewNodeWithValue([]byte{2})
	branch := NewNodeWithLR(leftChild, rightChild)

	emptyNode := NewEmptyNode(sszutils.ZeroBytes()[:32])

	t.Run("Left", func(t *testing.T) {
		if leaf.Left() != nil {
			t.Error("leaf.Left() should be nil")
		}
		if branch.Left().n != leftChild.n {
			t.Error("branch.Left() mismatch")
		}
	})

	t.Run("Right", func(t *testing.T) {
		if leaf.Right() != nil {
			t.Error("leaf.Right() should be nil")
		}
		if branch.Right().n != rightChild.n {
			t.Error("branch.Right() mismatch")
		}
	})

	t.Run("IsLeaf", func(t *testing.T) {
		if !leaf.IsLeaf() {
			t.Error("leaf.IsLeaf() should be true")
		}
		if branch.IsLeaf() {
			t.Error("branch.IsLeaf() should be false")
		}
		if !emptyNode.IsLeaf() {
			t.Error("emptyNode.IsLeaf() should be true")
		}
	})

	t.Run("IsEmpty", func(t *testing.T) {
		if leaf.IsEmpty() {
			t.Error("leaf.IsEmpty() should be false")
		}
		if branch.IsEmpty() {
			t.Error("branch.IsEmpty() should be false")
		}
		if !emptyNode.IsEmpty() {
			t.Error("emptyNode.IsEmpty() should be true")
		}
	})

	t.Run("Value", func(t *testing.T) {
		if !bytes.Equal(leaf.Value()[:1], leafValue) {
			t.Error("leaf.Value() mismatch")
		}
		if branch.Value() != nil {
			t.Error("branch.Value() should be nil before hashing")
		}
		if !bytes.Equal(emptyNode.Value(), sszutils.ZeroBytes()[:32]) {
			t.Error("emptyNode.Value() mismatch")
		}
	})
}

func TestNodeShow(t *testing.T) {
	// Create a simple tree for testing Show functionality
	// We'll create a tree with leaves and branches to test the show output
	leaf1 := NewNodeWithValue([]byte{1, 2, 3, 4})
	leaf2 := NewNodeWithValue([]byte{5, 6, 7, 8})
	leaf3 := NewNodeWithValue([]byte{9, 10, 11, 12})
	leaf4 := NewNodeWithValue([]byte{13, 14, 15, 16})

	branch1 := NewNodeWithLR(leaf1, leaf2)
	branch2 := NewNodeWithLR(leaf3, leaf4)
	root := NewNodeWithLR(branch1, branch2)

	t.Run("Show with depth 0", func(t *testing.T) {
		// Just test that Show doesn't panic - it prints to stdout
		// so we can't easily capture the output in a unit test
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Show should not panic: %v", r)
			}
		}()

		root.Show(0)
	})

	t.Run("Show with depth 2", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Show should not panic: %v", r)
			}
		}()

		root.Show(2)
	})

	t.Run("Show with depth 10", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Show should not panic: %v", r)
			}
		}()

		root.Show(10)
	})

	t.Run("Show on leaf node", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Show on leaf should not panic: %v", r)
			}
		}()

		leaf1.Show(5)
	})

	t.Run("Show on empty node", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Show on empty node should not panic: %v", r)
			}
		}()

		emptyNode := NewEmptyNode(sszutils.ZeroBytes()[:32])
		emptyNode.Show(3)
	})
}

// Get must reject non-positive generalized indices instead of returning the
// root or producing an out-of-range access.
func TestGetRejectsNonPositiveIndex(t *testing.T) {
	node, err := TreeFromNodes([]*Node{LeafFromUint64(1), LeafFromUint64(2)}, 2)
	if err != nil {
		t.Fatalf("TreeFromNodes: %v", err)
	}

	for _, idx := range []int{0, -1, -100} {
		got, err := node.Get(idx)
		if err == nil {
			t.Errorf("Get(%d): expected error", idx)
		}
		if got != nil {
			t.Errorf("Get(%d): expected nil node", idx)
		}
	}
}

// Prove must reject non-positive generalized indices without panicking on a
// negative make capacity.
func TestProveRejectsNonPositiveIndex(t *testing.T) {
	node, err := TreeFromNodes([]*Node{LeafFromUint64(1), LeafFromUint64(2)}, 2)
	if err != nil {
		t.Fatalf("TreeFromNodes: %v", err)
	}

	for _, idx := range []int{0, -1, -100} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Prove(%d) panicked: %v", idx, r)
				}
			}()
			proof, err := node.Prove(idx)
			if err == nil {
				t.Errorf("Prove(%d): expected error", idx)
			}
			if proof != nil {
				t.Errorf("Prove(%d): expected nil proof", idx)
			}
		}()
	}
}

// ProveMulti must reject any non-positive generalized index, including when it
// is mixed with valid indices.
func TestProveMultiRejectsNonPositiveIndex(t *testing.T) {
	node, err := TreeFromNodes([]*Node{LeafFromUint64(1), LeafFromUint64(2)}, 2)
	if err != nil {
		t.Fatalf("TreeFromNodes: %v", err)
	}

	for _, indices := range [][]int{{0}, {4, 0}, {-1}} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ProveMulti(%v) panicked: %v", indices, r)
				}
			}()
			proof, err := node.ProveMulti(indices)
			if err == nil {
				t.Errorf("ProveMulti(%v): expected error", indices)
			}
			if proof != nil {
				t.Errorf("ProveMulti(%v): expected nil proof", indices)
			}
		}()
	}
}

// Hashing a branch node with a missing child must produce a clear panic rather
// than a raw nil pointer dereference.
func TestHashIncompleteBranchPanicsCleanly(t *testing.T) {
	leaf := LeafFromUint64(1)

	for _, n := range []*Node{NewNodeWithLR(leaf, nil), NewNodeWithLR(nil, leaf)} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Error("expected panic for incomplete branch")
					return
				}
				if msg, ok := r.(string); !ok || msg != "Tree incomplete" {
					t.Errorf("expected 'Tree incomplete' panic, got %v", r)
				}
			}()
			_ = n.Hash()
		}()
	}
}

// A nil node has nothing to finalize, and a tree that cannot be hashed shows
// why instead of its nodes.
func TestNodeFinalizeNilAndShowMalformed(t *testing.T) {
	var missing *Node
	if err := missing.Finalize(); err != nil {
		t.Errorf("Finalize of a nil node: %v", err)
	}

	malformed := NewNodeWithLR(LeafFromUint64(1), nil)
	malformed.Show(0)
	if err := malformed.Finalize(); !errors.Is(err, errMalformedTree) {
		t.Errorf("Finalize of a malformed tree: err = %v", err)
	}
}

// No option, the zero option and a nil Algorithm all name SHA256.
func TestOptionsDefaultToSHA256(t *testing.T) {
	tree, root := algorithmTree(t, hasher.SHA256, paddedListWalk)
	proof, err := tree.Prove(130)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	for name, opts := range map[string][]Option{
		"none":          nil,
		"zero option":   {{}},
		"nil algorithm": {WithAlgorithm(nil)},
	} {
		ok, verifyErr := VerifyProof(root, proof, opts...)
		if verifyErr != nil || !ok {
			t.Errorf("%s: VerifyProof = %v, %v", name, ok, verifyErr)
		}
		if w := NewWrapper(opts...); w.Algorithm() != hasher.SHA256 {
			t.Errorf("%s: NewWrapper is not the plain SHA256 wrapper", name)
		}
	}
}
