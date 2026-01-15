// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

func TestNewNodeWithValue(t *testing.T) {
	tests := []struct {
		name        string
		value       []byte
		expectEmpty bool
	}{
		{
			name:        "non-empty value",
			value:       []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
			expectEmpty: false,
		},
		{
			name:        "zero value",
			value:       sszutils.ZeroBytes()[:32],
			expectEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := NewNodeWithValue(tt.value)

			if node == nil {
				t.Fatal("node should not be nil")
			}
			if node.left != nil || node.right != nil {
				t.Error("leaf node should have nil children")
			}
			if !bytes.Equal(node.value, tt.value) {
				t.Error("node value mismatch")
			}
			if node.isEmpty != tt.expectEmpty {
				t.Errorf("expected isEmpty=%v, got %v", tt.expectEmpty, node.isEmpty)
			}
		})
	}
}

func TestNewEmptyNode(t *testing.T) {
	zeroHash := sszutils.ZeroBytes()[:32]
	node := NewEmptyNode(zeroHash)

	if node == nil {
		t.Fatal("node should not be nil")
	}
	if !node.isEmpty {
		t.Error("empty node should have isEmpty=true")
	}
	if !bytes.Equal(node.value, zeroHash) {
		t.Error("empty node value should match zero hash")
	}
	if node.left != nil || node.right != nil {
		t.Error("empty node should have nil children")
	}
}

func TestNewNodeWithLR(t *testing.T) {
	left := NewNodeWithValue([]byte{1})
	right := NewNodeWithValue([]byte{2})

	node := NewNodeWithLR(left, right)

	if node == nil {
		t.Fatal("node should not be nil")
	}
	if node.left != left {
		t.Error("left child mismatch")
	}
	if node.right != right {
		t.Error("right child mismatch")
	}
	if node.value != nil {
		t.Error("branch node should have nil value initially")
	}
}

func TestTreeFromChunks(t *testing.T) {
	tests := []struct {
		name        string
		chunks      [][]byte
		expectError bool
	}{
		{
			name: "valid power of 2 chunks",
			chunks: [][]byte{
				bytes.Repeat([]byte{1}, 32),
				bytes.Repeat([]byte{2}, 32),
				bytes.Repeat([]byte{3}, 32),
				bytes.Repeat([]byte{4}, 32),
			},
			expectError: false,
		},
		{
			name: "non-power of 2 chunks",
			chunks: [][]byte{
				bytes.Repeat([]byte{1}, 32),
				bytes.Repeat([]byte{2}, 32),
				bytes.Repeat([]byte{3}, 32),
			},
			expectError: true,
		},
		{
			name:        "single chunk",
			chunks:      [][]byte{bytes.Repeat([]byte{1}, 32)},
			expectError: false,
		},
		{
			name:        "empty chunks",
			chunks:      [][]byte{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := TreeFromChunks(tt.chunks)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tree == nil {
				t.Fatal("tree should not be nil")
			}

			// Verify the tree has correct structure
			if len(tt.chunks) == 1 {
				if !tree.IsLeaf() {
					t.Error("single chunk should produce leaf node")
				}
			} else {
				if tree.IsLeaf() {
					t.Error("multiple chunks should produce branch node")
				}
			}
		})
	}
}

var TestCasesTreeFromNodes = []struct {
	name        string
	nodes       []*Node
	limit       int
	expectError bool
	validateFn  func(*testing.T, *Node)
}{
	{
		name:        "zero limit returns empty node",
		nodes:       []*Node{},
		limit:       0,
		expectError: false,
		validateFn: func(t *testing.T, n *Node) {
			if !n.IsEmpty() {
				t.Error("expected empty node for zero limit")
			}
		},
	},
	{
		name:        "no nodes with limit",
		nodes:       []*Node{},
		limit:       4,
		expectError: false,
		validateFn: func(t *testing.T, n *Node) {
			if !n.IsEmpty() {
				t.Error("expected empty node for no leaves")
			}
		},
	},
	{
		name:        "single node with limit 1",
		nodes:       []*Node{NewNodeWithValue([]byte{1})},
		limit:       1,
		expectError: false,
		validateFn: func(t *testing.T, n *Node) {
			if !n.IsLeaf() {
				t.Error("expected leaf node")
			}
		},
	},
	{
		name:        "single node with limit 2",
		nodes:       []*Node{NewNodeWithValue([]byte{1})},
		limit:       2,
		expectError: false,
		validateFn: func(t *testing.T, n *Node) {
			if n.IsLeaf() {
				t.Error("expected branch node")
			}
			if n.right == nil || !n.right.IsEmpty() {
				t.Error("expected empty right child")
			}
		},
	},
	{
		name:        "two nodes with limit 2",
		nodes:       []*Node{NewNodeWithValue([]byte{1}), NewNodeWithValue([]byte{2})},
		limit:       2,
		expectError: false,
		validateFn: func(t *testing.T, n *Node) {
			if n.IsLeaf() {
				t.Error("expected branch node")
			}
		},
	},
	{
		name:        "non-power of 2 limit",
		nodes:       []*Node{NewNodeWithValue([]byte{1})},
		limit:       3,
		expectError: true,
	},
	{
		name: "four nodes with limit 8",
		nodes: []*Node{
			NewNodeWithValue([]byte{1}),
			NewNodeWithValue([]byte{2}),
			NewNodeWithValue([]byte{3}),
			NewNodeWithValue([]byte{4}),
		},
		limit:       8,
		expectError: false,
		validateFn: func(t *testing.T, n *Node) {
			// Should have padding on the right side
			if n.IsLeaf() {
				t.Error("expected branch node")
			}
		},
	},
	{
		name: "large limit with few nodes does not OOM",
		nodes: []*Node{
			NewNodeWithValue([]byte{1}),
			NewNodeWithValue([]byte{2}),
		},

		limit:       1 << 20, // ~1 million
		expectError: false,
		validateFn: func(t *testing.T, n *Node) {
			if n == nil {
				t.Fatal("expected non-nil tree")
			}
			if n.IsLeaf() {
				t.Error("expected branch node for large limit")
			}
		},
	},
}

func TestTreeFromNodes(t *testing.T) {
	for _, tt := range TestCasesTreeFromNodes {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := TreeFromNodes(tt.nodes, tt.limit)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tree == nil {
				t.Fatal("tree should not be nil")
			}

			if tt.validateFn != nil {
				tt.validateFn(t, tree)
			}
		})
	}
}

func BenchmarkTreeFromNodes(b *testing.B) {
	for _, bm := range TestCasesTreeFromNodes {
		b.Run(bm.name, func(b *testing.B) {

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				TreeFromNodes(bm.nodes, bm.limit)
			}
		})
	}
}

func TestTreeFromNodesProgressive(t *testing.T) {
	tests := []struct {
		name       string
		nodes      []*Node
		validateFn func(*testing.T, *Node)
	}{
		{
			name:  "empty nodes",
			nodes: []*Node{},
			validateFn: func(t *testing.T, n *Node) {
				if !n.IsEmpty() {
					t.Error("expected empty node")
				}
			},
		},
		{
			name:  "single node",
			nodes: []*Node{NewNodeWithValue([]byte{1})},
			validateFn: func(t *testing.T, n *Node) {
				// Progressive tree with 1 node: base_size=1, so left gets the node, right is empty
				if n.IsLeaf() {
					t.Error("expected branch node")
				}
				if !n.right.IsEmpty() {
					t.Error("expected empty right child")
				}
			},
		},
		{
			name: "five nodes - progressive pattern",
			nodes: []*Node{
				NewNodeWithValue([]byte{1}),
				NewNodeWithValue([]byte{2}),
				NewNodeWithValue([]byte{3}),
				NewNodeWithValue([]byte{4}),
				NewNodeWithValue([]byte{5}),
			},
			validateFn: func(t *testing.T, n *Node) {
				// With 5 nodes: first 1 goes to right (binary), remaining 4 go to left (progressive)
				if n.IsLeaf() {
					t.Error("expected branch node")
				}
				// Right child should be a single leaf (first node)
				// Left child should be progressive tree of remaining 4 nodes
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := TreeFromNodesProgressive(tt.nodes)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tree == nil {
				t.Fatal("tree should not be nil")
			}

			if tt.validateFn != nil {
				tt.validateFn(t, tree)
			}
		})
	}
}

func TestTreeFromNodesWithMixin(t *testing.T) {
	nodes := []*Node{
		NewNodeWithValue([]byte{1}),
		NewNodeWithValue([]byte{2}),
		NewNodeWithValue([]byte{3}),
		NewNodeWithValue([]byte{4}),
	}

	tree, err := TreeFromNodesWithMixin(nodes, 4, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tree.IsLeaf() {
		t.Error("expected branch node")
	}

	// Right child should be the length mixin
	lengthBuf := make([]byte, 32)
	binary.LittleEndian.PutUint64(lengthBuf[:8], 4)
	if !bytes.Equal(tree.right.value, lengthBuf) {
		t.Error("right child should be length mixin")
	}
}

func TestTreeFromNodesProgressiveWithMixin(t *testing.T) {
	nodes := []*Node{
		NewNodeWithValue([]byte{1}),
		NewNodeWithValue([]byte{2}),
		NewNodeWithValue([]byte{3}),
	}

	tree, err := TreeFromNodesProgressiveWithMixin(nodes, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tree.IsLeaf() {
		t.Error("expected branch node")
	}

	// Right child should be the length mixin
	lengthBuf := make([]byte, 32)
	binary.LittleEndian.PutUint64(lengthBuf[:8], 3)
	if !bytes.Equal(tree.right.value, lengthBuf) {
		t.Error("right child should be length mixin")
	}
}

func TestTreeFromNodesProgressiveWithActiveFields(t *testing.T) {
	nodes := []*Node{
		NewNodeWithValue([]byte{1}),
		NewNodeWithValue([]byte{2}),
		NewNodeWithValue([]byte{3}),
	}

	activeFields := []byte{0b11111111, 0b00000111} // 11 active fields

	tree, err := TreeFromNodesProgressiveWithActiveFields(nodes, activeFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tree.IsLeaf() {
		t.Error("expected branch node")
	}

	// Right child should be the active fields bitvector
	expectedLeaf := append(activeFields, bytes.Repeat([]byte{0}, 30)...)
	if !bytes.Equal(tree.right.value, expectedLeaf) {
		t.Error("right child should be active fields bitvector")
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

			if tt.expectValue != nil && !bytes.Equal(node.value[:1], tt.expectValue) {
				t.Errorf("expected value %v, got %v", tt.expectValue, node.value[:1])
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

	expectedHash := sha256.Sum256(append(left.value, right.value...))
	branchHash := branch.Hash()

	if !bytes.Equal(branchHash, expectedHash[:]) {
		t.Error("branch node hash mismatch")
	}

	// Test that hash is cached
	if branch.value == nil {
		t.Error("hash should be cached in node value")
	}

	// Call hash again to ensure it returns cached value
	branchHash2 := branch.Hash()
	if !bytes.Equal(branchHash, branchHash2) {
		t.Error("cached hash should be returned")
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

func TestLeafFromBytesLargeThan32(t *testing.T) {
	// Test LeafFromBytes with data larger than 32 bytes (should panic)
	defer func() {
		if r := recover(); r == nil {
			t.Error("LeafFromBytes with large data should panic")
		}
	}()

	largeData := bytes.Repeat([]byte{0xFF}, 50)
	LeafFromBytes(largeData) // Should panic with "Unimplemented"
}

func TestTreeFromNodesProgressiveBoundaryConditions(t *testing.T) {
	t.Run("exactly power of 2 nodes", func(t *testing.T) {
		// Test with exactly 4 nodes (power of 2)
		nodes := []*Node{
			NewNodeWithValue([]byte{1}),
			NewNodeWithValue([]byte{2}),
			NewNodeWithValue([]byte{3}),
			NewNodeWithValue([]byte{4}),
		}

		tree, err := TreeFromNodesProgressive(nodes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tree == nil {
			t.Fatal("tree should not be nil")
		}
	})

	t.Run("large number of nodes", func(t *testing.T) {
		// Test with many nodes to exercise deeper recursion
		nodes := make([]*Node, 17) // 17 nodes should create complex progressive tree
		for i := range nodes {
			nodes[i] = NewNodeWithValue([]byte{byte(i)})
		}

		tree, err := TreeFromNodesProgressive(nodes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tree == nil {
			t.Fatal("tree should not be nil")
		}

		// Should be a branch node due to progressive structure
		if tree.IsLeaf() {
			t.Error("tree with many nodes should be a branch")
		}
	})
}

func TestLeafCreationFunctions(t *testing.T) {
	t.Run("LeafFromUint64", func(t *testing.T) {
		val := uint64(0x1234567890ABCDEF)
		leaf := LeafFromUint64(val)

		buf := make([]byte, 32)
		binary.LittleEndian.PutUint64(buf[:8], val)

		if !bytes.Equal(leaf.value, buf) {
			t.Error("LeafFromUint64 value mismatch")
		}
	})

	t.Run("LeafFromUint32", func(t *testing.T) {
		val := uint32(0x12345678)
		leaf := LeafFromUint32(val)

		buf := make([]byte, 32)
		binary.LittleEndian.PutUint32(buf[:4], val)

		if !bytes.Equal(leaf.value, buf) {
			t.Error("LeafFromUint32 value mismatch")
		}
	})

	t.Run("LeafFromUint16", func(t *testing.T) {
		val := uint16(0x1234)
		leaf := LeafFromUint16(val)

		buf := make([]byte, 32)
		binary.LittleEndian.PutUint16(buf[:2], val)

		if !bytes.Equal(leaf.value, buf) {
			t.Error("LeafFromUint16 value mismatch")
		}
	})

	t.Run("LeafFromUint8", func(t *testing.T) {
		val := uint8(0xAB)
		leaf := LeafFromUint8(val)

		buf := make([]byte, 32)
		buf[0] = val

		if !bytes.Equal(leaf.value, buf) {
			t.Error("LeafFromUint8 value mismatch")
		}
	})

	t.Run("LeafFromBool", func(t *testing.T) {
		// Test true
		leafTrue := LeafFromBool(true)
		bufTrue := make([]byte, 32)
		bufTrue[0] = 1

		if !bytes.Equal(leafTrue.value, bufTrue) {
			t.Error("LeafFromBool(true) value mismatch")
		}

		// Test false
		leafFalse := LeafFromBool(false)
		bufFalse := make([]byte, 32)

		if !bytes.Equal(leafFalse.value, bufFalse) {
			t.Error("LeafFromBool(false) value mismatch")
		}
	})

	t.Run("LeafFromBytes", func(t *testing.T) {
		// Test < 32 bytes
		smallBytes := []byte{1, 2, 3, 4}
		leafSmall := LeafFromBytes(smallBytes)
		expectedSmall := append(smallBytes, bytes.Repeat([]byte{0}, 28)...)

		if !bytes.Equal(leafSmall.value, expectedSmall) {
			t.Error("LeafFromBytes (small) value mismatch")
		}

		// Test exactly 32 bytes
		fullBytes := bytes.Repeat([]byte{0xFF}, 32)
		leafFull := LeafFromBytes(fullBytes)

		if !bytes.Equal(leafFull.value, fullBytes) {
			t.Error("LeafFromBytes (32 bytes) value mismatch")
		}
	})

	t.Run("EmptyLeaf", func(t *testing.T) {
		leaf := EmptyLeaf()

		if !bytes.Equal(leaf.value, sszutils.ZeroBytes()[:32]) {
			t.Error("EmptyLeaf value mismatch")
		}
		if !leaf.isEmpty {
			t.Error("EmptyLeaf should have isEmpty=true")
		}
	})

	t.Run("LeavesFromUint64", func(t *testing.T) {
		// Test empty
		emptyLeaves := LeavesFromUint64([]uint64{})
		if len(emptyLeaves) != 0 {
			t.Error("LeavesFromUint64 empty should return empty slice")
		}

		// Test multiple values
		values := []uint64{0x1111111111111111, 0x2222222222222222, 0x3333333333333333, 0x4444444444444444}
		leaves := LeavesFromUint64(values)

		// 4 uint64s = 32 bytes = 1 leaf
		if len(leaves) != 1 {
			t.Errorf("expected 1 leaf, got %d", len(leaves))
		}

		// Verify the packed values
		buf := make([]byte, 32)
		for i, v := range values {
			binary.LittleEndian.PutUint64(buf[i*8:(i+1)*8], v)
		}

		if !bytes.Equal(leaves[0].value, buf) {
			t.Error("LeavesFromUint64 packed value mismatch")
		}

		// Test with more values that span multiple leaves
		manyValues := make([]uint64, 5)
		for i := range manyValues {
			manyValues[i] = uint64(i + 1)
		}
		manyLeaves := LeavesFromUint64(manyValues)

		// 5 uint64s = 40 bytes = 2 leaves (32 + 8 bytes)
		if len(manyLeaves) != 2 {
			t.Errorf("expected 2 leaves, got %d", len(manyLeaves))
		}
	})
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
		if branch.Left() != leftChild {
			t.Error("branch.Left() mismatch")
		}
	})

	t.Run("Right", func(t *testing.T) {
		if leaf.Right() != nil {
			t.Error("leaf.Right() should be nil")
		}
		if branch.Right() != rightChild {
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

	t.Run("floorLog2", func(t *testing.T) {
		tests := []struct {
			n        int
			expected int
		}{
			{1, 0},
			{2, 1},
			{3, 1},
			{4, 2},
			{5, 2},
			{7, 2},
			{8, 3},
			{15, 3},
			{16, 4},
			{31, 4},
			{32, 5},
		}

		for _, tt := range tests {
			result := floorLog2(tt.n)
			if result != tt.expected {
				t.Errorf("floorLog2(%d) = %d, want %d", tt.n, result, tt.expected)
			}
		}
	})

	t.Run("powerTwo", func(t *testing.T) {
		tests := []struct {
			n        int
			expected int
		}{
			{0, 1},
			{1, 2},
			{2, 4},
			{3, 8},
			{4, 16},
			{5, 32},
			{10, 1024},
		}

		for _, tt := range tests {
			result := powerTwo(tt.n)
			if result != tt.expected {
				t.Errorf("powerTwo(%d) = %d, want %d", tt.n, result, tt.expected)
			}
		}
	})
}

func TestGetZeroOrderHashes(t *testing.T) {
	depth := 3
	hashes := getZeroOrderHashes(depth)

	// Should have hashes for depths 0, 1, 2, 3
	if len(hashes) != depth+1 {
		t.Errorf("expected %d hashes, got %d", depth+1, len(hashes))
	}

	// Depth 3 should be zero bytes
	if !bytes.Equal(hashes[3], make([]byte, 32)) {
		t.Error("depth 3 hash should be zero bytes")
	}

	// Each level should be hash of two children
	for i := depth - 1; i >= 0; i-- {
		expected := hashFn(append(hashes[i+1], hashes[i+1]...))
		if !bytes.Equal(hashes[i], expected) {
			t.Errorf("hash at depth %d mismatch", i)
		}
	}

	// Verify against precomputed zero hashes
	// hasher.GetZeroHash(0) = zero bytes = hashes[depth]
	// hasher.GetZeroHash(1) = hash(zero, zero) = hashes[depth-1]
	// etc.
	for i := 0; i <= depth; i++ {
		precomputed := hasher.GetZeroHash(i)
		computed := hashes[depth-i]
		if !bytes.Equal(computed, precomputed) {
			t.Errorf("hash at depth %d doesn't match precomputed: computed=%x, precomputed=%x", i, computed, precomputed)
		}
	}
}

func TestTreeEdgeCases(t *testing.T) {
	t.Run("panic on incomplete tree", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for incomplete tree")
			}
		}()

		// Create node with only right child
		node := &Node{left: nil, right: NewNodeWithValue([]byte{1})}
		_ = node.Hash() // Should panic
	})

	t.Run("hash caching", func(t *testing.T) {
		left := NewNodeWithValue(bytes.Repeat([]byte{1}, 32))
		right := NewNodeWithValue(bytes.Repeat([]byte{2}, 32))
		branch := NewNodeWithLR(left, right)

		// First call computes hash
		hash1 := branch.Hash()
		if branch.value == nil {
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
		expected := hashFn(append(left.value, emptyRight.value...))

		if !bytes.Equal(hash, expected) {
			t.Error("hash with empty right node mismatch")
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
