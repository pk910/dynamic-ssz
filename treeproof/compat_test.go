// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0

package treeproof

import (
	"bytes"
	"encoding/binary"
	"errors"
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
			// A zero value is data, not padding.
			name:        "zero value",
			value:       sszutils.ZeroBytes()[:32],
			expectEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := NewNodeWithValue(tt.value)

			if node == nil {
				t.Fatal("node should not be nil")
			}
			if node.n.left != nil || node.n.right != nil {
				t.Error("leaf node should have nil children")
			}
			if !bytes.Equal(node.n.value[:], tt.value) {
				t.Error("node value mismatch")
			}
			if node.n.isEmpty != tt.expectEmpty {
				t.Errorf("expected isEmpty=%v, got %v", tt.expectEmpty, node.n.isEmpty)
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
	if !node.n.isEmpty {
		t.Error("empty node should have isEmpty=true")
	}
	if !bytes.Equal(node.n.value[:], zeroHash) {
		t.Error("empty node value should match zero hash")
	}
	if node.n.left != nil || node.n.right != nil {
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
	if node.n.left != left.n {
		t.Error("left child mismatch")
	}
	if node.n.right != right.n {
		t.Error("right child mismatch")
	}
	if node.n.hasValue {
		t.Error("branch node should have no value initially")
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
			t.Helper()
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
			t.Helper()
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
			t.Helper()
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
			t.Helper()
			if n.IsLeaf() {
				t.Error("expected branch node")
			}
			if n.n.right == nil || !n.n.right.isEmpty {
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
			t.Helper()
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
			t.Helper()
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
			t.Helper()
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

			for i := 0; i < b.N; i++ {
				_, _ = TreeFromNodes(bm.nodes, bm.limit)
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
				t.Helper()
				if !n.IsEmpty() {
					t.Error("expected empty node")
				}
			},
		},
		{
			name:  "single node",
			nodes: []*Node{NewNodeWithValue([]byte{1})},
			validateFn: func(t *testing.T, n *Node) {
				t.Helper()
				// Progressive tree with 1 node: base_size=1, so left gets the node, right is empty
				if n.IsLeaf() {
					t.Error("expected branch node")
				}
				if !n.n.right.isEmpty {
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
				t.Helper()
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
	if !bytes.Equal(tree.n.right.value[:], lengthBuf) {
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
	if !bytes.Equal(tree.n.right.value[:], lengthBuf) {
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
	expectedLeaf := make([]byte, 0, 32)
	expectedLeaf = append(expectedLeaf, activeFields...)
	expectedLeaf = append(expectedLeaf, make([]byte, 30)...)
	if !bytes.Equal(tree.n.right.value[:], expectedLeaf) {
		t.Error("right child should be active fields bitvector")
	}
}

func TestLeafFromBytesLargeThan32(t *testing.T) {
	// Oversized input is merkleized into a subtree instead of panicking, and the
	// chunk structure is recorded as real child nodes.
	largeData := bytes.Repeat([]byte{0xFF}, 50)

	var node *Node
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("LeafFromBytes panicked on oversized input: %v", r)
			}
		}()
		node = LeafFromBytes(largeData)
	}()

	if node == nil {
		t.Fatal("expected non-nil node")
	}
	if node.Left() == nil || node.Right() == nil {
		t.Fatal("oversized input must be recorded as a branch node with children")
	}

	chunk0 := make([]byte, 32)
	copy(chunk0, largeData[:32])
	chunk1 := make([]byte, 32)
	copy(chunk1, largeData[32:])

	if !bytes.Equal(node.Left().Value(), chunk0) {
		t.Errorf("left chunk mismatch: got %x want %x", node.Left().Value(), chunk0)
	}
	if !bytes.Equal(node.Right().Value(), chunk1) {
		t.Errorf("right chunk mismatch: got %x want %x", node.Right().Value(), chunk1)
	}

	root := node.Hash()
	if len(root) != 32 {
		t.Fatalf("root must be 32 bytes, got %d", len(root))
	}
	if want := hashPair(chunk0, chunk1); !bytes.Equal(root, want) {
		t.Errorf("merkle root mismatch: got %x want %x", root, want)
	}
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

		if !bytes.Equal(leaf.n.value[:], buf) {
			t.Error("LeafFromUint64 value mismatch")
		}
	})

	t.Run("LeafFromUint32", func(t *testing.T) {
		val := uint32(0x12345678)
		leaf := LeafFromUint32(val)

		buf := make([]byte, 32)
		binary.LittleEndian.PutUint32(buf[:4], val)

		if !bytes.Equal(leaf.n.value[:], buf) {
			t.Error("LeafFromUint32 value mismatch")
		}
	})

	t.Run("LeafFromUint16", func(t *testing.T) {
		val := uint16(0x1234)
		leaf := LeafFromUint16(val)

		buf := make([]byte, 32)
		binary.LittleEndian.PutUint16(buf[:2], val)

		if !bytes.Equal(leaf.n.value[:], buf) {
			t.Error("LeafFromUint16 value mismatch")
		}
	})

	t.Run("LeafFromUint8", func(t *testing.T) {
		val := uint8(0xAB)
		leaf := LeafFromUint8(val)

		buf := make([]byte, 32)
		buf[0] = val

		if !bytes.Equal(leaf.n.value[:], buf) {
			t.Error("LeafFromUint8 value mismatch")
		}
	})

	t.Run("LeafFromBool", func(t *testing.T) {
		// Test true
		leafTrue := LeafFromBool(true)
		bufTrue := make([]byte, 32)
		bufTrue[0] = 1

		if !bytes.Equal(leafTrue.n.value[:], bufTrue) {
			t.Error("LeafFromBool(true) value mismatch")
		}

		// Test false
		leafFalse := LeafFromBool(false)
		bufFalse := make([]byte, 32)

		if !bytes.Equal(leafFalse.n.value[:], bufFalse) {
			t.Error("LeafFromBool(false) value mismatch")
		}
	})

	t.Run("LeafFromBytes", func(t *testing.T) {
		// Test < 32 bytes
		smallBytes := make([]byte, 0, 32)
		smallBytes = append(smallBytes, 1, 2, 3, 4)
		leafSmall := LeafFromBytes(smallBytes)
		expectedSmall := make([]byte, 0, 32)
		expectedSmall = append(expectedSmall, smallBytes...)
		expectedSmall = append(expectedSmall, make([]byte, 28)...)

		if !bytes.Equal(leafSmall.n.value[:], expectedSmall) {
			t.Error("LeafFromBytes (small) value mismatch")
		}

		// Test exactly 32 bytes
		fullBytes := bytes.Repeat([]byte{0xFF}, 32)
		leafFull := LeafFromBytes(fullBytes)

		if !bytes.Equal(leafFull.n.value[:], fullBytes) {
			t.Error("LeafFromBytes (32 bytes) value mismatch")
		}
	})

	t.Run("EmptyLeaf", func(t *testing.T) {
		leaf := EmptyLeaf()

		if !bytes.Equal(leaf.n.value[:], sszutils.ZeroBytes()[:32]) {
			t.Error("EmptyLeaf value mismatch")
		}
		if !leaf.n.isEmpty {
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

		if !bytes.Equal(leaves[0].n.value[:], buf) {
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

func TestTreeFromNodesWithMixinNonPowerOfTwoLimit(t *testing.T) {
	nodes := []*Node{
		NewNodeWithValue([]byte{1}),
		NewNodeWithValue([]byte{2}),
		NewNodeWithValue([]byte{3}),
	}

	// limit 3 is not a power of 2; TreeFromNodesWithMixin should normalize it
	tree, err := TreeFromNodesWithMixin(nodes, 3, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tree.IsLeaf() {
		t.Error("expected branch node")
	}
}

func TestTreeFromNodesWithMixinZeroLimit(t *testing.T) {
	nodes := []*Node{NewNodeWithValue([]byte{1})}

	// The mixin path clamps an under-sized limit up to the chunk count, the
	// same fallback the Hasher applies, so no leaf is silently dropped.
	tree, err := TreeFromNodesWithMixin(nodes, 1, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reference, err := TreeFromNodesWithMixin(nodes, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(tree.Hash(), reference.Hash()) {
		t.Errorf("clamped limit root mismatch: %x != %x", tree.Hash(), reference.Hash())
	}
}

// A limit below the leaf count overflows the type it declares. The exported
// form refuses it; the form the walkers reduce through takes the depth the
// leaves need, as hasher.Hasher does, so every leaf reaches the root and two
// values that differ cannot share one.
func TestTreeFromNodesWithMixinLimitBelowCount(t *testing.T) {
	nodes := make([]*Node, 8)
	for i := range nodes {
		nodes[i] = NewNodeWithValue([]byte{byte(i + 1)})
	}

	if _, err := TreeFromNodesWithMixin(nodes, 8, 2); !errors.Is(err, sszutils.ErrChunkLimitExceeded) {
		t.Fatalf("a limit below the leaf count was accepted: %v", err)
	}

	rawTree, err := treeFromNodesWithMixin64(nodesOf(nodes), 8, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tree := &Node{n: rawTree, alg: hasher.SHA256}

	// No depth holds more leaves than the limit allows, so the tree takes the
	// depth the leaves need and every one of them reaches the root.
	reference, err := TreeFromNodesWithMixin(nodes, 8, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(tree.Hash(), reference.Hash()) {
		t.Errorf("over-capacity root %x, want the root of every leaf %x", tree.Hash(), reference.Hash())
	}

	// A leaf past the limit is part of the value, so it moves the root: two
	// values that differ there cannot share one.
	altered := make([]*Node, len(nodes))
	copy(altered, nodes)
	altered[7] = NewNodeWithValue([]byte{0xff})
	rawAlt, err := treeFromNodesWithMixin64(nodesOf(altered), 8, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	alt := &Node{n: rawAlt, alg: hasher.SHA256}
	if bytes.Equal(tree.Hash(), alt.Hash()) {
		t.Errorf("a leaf past the limit left the root unchanged")
	}
}

func TestTreeFromNodesWithMixinEmpty(t *testing.T) {
	tree, err := TreeFromNodesWithMixin(nil, 0, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
}

func TestTreeFromNodesProgressiveWithMixinEmpty(t *testing.T) {
	tree, err := TreeFromNodesProgressiveWithMixin(nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
}

func TestTreeFromNodesProgressiveWithActiveFieldsEmpty(t *testing.T) {
	tree, err := TreeFromNodesProgressiveWithActiveFields(nil, []byte{0x07})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
}

// A bitvector wider than one chunk is merkleized to its root before the mixin,
// as hasher.Hasher does, so the fields past the first chunk reach the tree.
func TestTreeFromNodesProgressiveWithActiveFieldsWide(t *testing.T) {
	leaves := []*Node{LeafFromUint64(1), LeafFromUint64(2)}

	activeFields := make([]byte, 65)
	activeFields[0] = 0x03
	tree, err := TreeFromNodesProgressiveWithActiveFields(leaves, activeFields)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	altered := make([]byte, len(activeFields))
	copy(altered, activeFields)
	altered[64] = 0x01
	alt, err := TreeFromNodesProgressiveWithActiveFields(leaves, altered)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bytes.Equal(tree.Hash(), alt.Hash()) {
		t.Fatal("a field past the first chunk did not reach the root")
	}
}

// TreeFromNodes and TreeFromNodesProgressive must reject nil leaves at
// construction time so a malformed tree is never returned.
func TestTreeFromNodesRejectsNilLeaf(t *testing.T) {
	leaf := LeafFromUint64(1)

	// nil leaves are rejected at construction so a later Hash() can never panic.
	if n, err := TreeFromNodes([]*Node{nil, leaf}, 2); err == nil || n != nil {
		t.Errorf("TreeFromNodes: expected nil tree and error for nil leaf, got tree=%v err=%v", n, err)
	}
	if n, err := TreeFromNodes([]*Node{nil}, 1); err == nil || n != nil {
		t.Errorf("TreeFromNodes single: expected nil tree and error for nil leaf, got tree=%v err=%v", n, err)
	}
	if n, err := TreeFromNodesProgressive([]*Node{leaf, nil}); err == nil || n != nil {
		t.Errorf("TreeFromNodesProgressive: expected nil tree and error for nil leaf, got tree=%v err=%v", n, err)
	}

	// A tree assembled directly with a nil child still hashes to a clean panic
	// rather than a raw nil dereference (the hashNode guard).
	defer func() {
		r := recover()
		if msg, ok := r.(string); !ok || msg != "Tree incomplete" {
			t.Errorf("expected 'Tree incomplete' panic, got %v", r)
		}
	}()
	_ = NewNodeWithLR(leaf, nil).Hash()
}

func TestTreeFromNodesRejectsExcessLeaves(t *testing.T) {
	nodes := []*Node{
		NewNodeWithValue([]byte{1}),
		NewNodeWithValue([]byte{2}),
		NewNodeWithValue([]byte{3}),
	}

	// More leaves than the limit allows would be dropped silently, producing a
	// valid-looking root for a different tree.
	if _, err := TreeFromNodes(nodes, 2); err == nil {
		t.Fatal("expected error for 3 leaves with limit 2")
	}
	if _, err := TreeFromNodes(nodes, 1); err == nil {
		t.Fatal("expected error for 3 leaves with limit 1")
	}
	if _, err := TreeFromNodes(nodes, 0); err == nil {
		t.Fatal("expected error for 3 leaves with limit 0")
	}

	// The boundary case still works.
	tree, err := TreeFromNodes(nodes[:2], 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tree == nil {
		t.Fatal("expected non-nil tree")
	}
}

// --- tree construction error paths (moved from errpath_test.go) ---
// --- treeFromNodesProgressiveImpl: left child build error ---

// TestTreeFromNodes64EdgeCases covers the zero-capacity, excess-leaf, non-pow2
// and depth-boundary branches of the uint64 tree builders and their int
// forwards.
func TestTreeFromNodes64EdgeCases(t *testing.T) {
	leaf := LeafFromUint64(1)

	// Zero capacity with a leaf is rejected; with no leaves it yields an empty node.
	if _, err := TreeFromNodes64([]*Node{leaf}, 0); err == nil {
		t.Error("TreeFromNodes64(leaf, 0) should error")
	}
	if n, err := TreeFromNodes64(nil, 0); err != nil || n == nil {
		t.Errorf("TreeFromNodes64(nil, 0) = %v, %v", n, err)
	}
	// Leaves exceeding the limit, and a non-power-of-two limit, are rejected.
	if _, err := TreeFromNodes64([]*Node{leaf, leaf, leaf}, 2); err == nil {
		t.Error("TreeFromNodes64 exceeding limit should error")
	}
	if _, err := TreeFromNodes64([]*Node{leaf}, 3); err == nil {
		t.Error("TreeFromNodes64 non-power-of-two limit should error")
	}
	// limit 1 (depth 0) with a single leaf returns that leaf.
	if n, err := TreeFromNodes64([]*Node{leaf}, 1); err != nil || n != leaf {
		t.Errorf("TreeFromNodes64(leaf, 1) = %v, %v; want the leaf", n, err)
	}
	// treeFromNodesToDepth clamps a negative depth to an empty node.
	if n, err := treeFromNodesToDepth(nil, -1); err != nil || n == nil {
		t.Errorf("treeFromNodesToDepth(nil, -1) = %v, %v", n, err)
	}
	// Int forwards clamp negative limit/num to zero.
	if _, err := TreeFromNodesWithMixin(nil, -1, -1); err != nil {
		t.Errorf("TreeFromNodesWithMixin(nil, -1, -1): %v", err)
	}
	// The mixin builder clamps an under-sized capacity up to the chunk count.
	if _, err := TreeFromNodesWithMixin64([]*Node{leaf}, 1, 0); err != nil {
		t.Errorf("TreeFromNodesWithMixin64(leaf, 1, 0): %v", err)
	}
}

// A non-positive limit cannot represent a usable capacity, so TreeFromNodes must
// error when leaves are present instead of silently dropping them and returning
// a valid-looking empty-tree root. A negative limit is the int-overflow artifact
// of a huge capacity on 32-bit builds.
func TestTreeFromNodesRejectsNonPositiveLimitWithLeaves(t *testing.T) {
	leaves := []*Node{LeafFromUint64(1)}
	for _, limit := range []int{0, -1, -1024} {
		if _, err := TreeFromNodes(leaves, limit); err == nil {
			t.Fatalf("TreeFromNodes(1 leaf, limit=%d) should error, got nil", limit)
		}
	}

	// No leaves with a non-positive limit is a valid empty tree.
	if _, err := TreeFromNodes(nil, 0); err != nil {
		t.Fatalf("TreeFromNodes(no leaves, limit=0) unexpected error: %v", err)
	}
}

// A leaf cannot hold more than its 32-byte chunk; NewNodeWithValue panics
// on longer input instead of truncating it into a silently wrong root.
// LeafFromBytes stays the constructor for longer input, merkleizing it into
// a subtree.
func TestNewNodeWithValueOversized(t *testing.T) {
	if n := NewNodeWithValue(bytes.Repeat([]byte{7}, 32)); !n.n.hasValue {
		t.Fatal("32-byte value must build a leaf")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for a 33-byte leaf value")
		}
	}()
	NewNodeWithValue(bytes.Repeat([]byte{7}, 33))
}

// TestNodeValuesDoNotAliasCallerMemory pins that a leaf owns its bytes. The
// 32-byte branch used to store the caller's slice while the shorter branch
// copied, so whether a later mutation of a reused scratch buffer corrupted
// every tree and root built from it depended on the input length.
func TestNodeValuesDoNotAliasCallerMemory(t *testing.T) {
	tests := []struct {
		name  string
		build func(buf []byte) *Node
		size  int
	}{
		{"NewNodeWithValue", func(buf []byte) *Node { return NewNodeWithValue(buf) }, 32},
		{"LeafFromBytes_exact", func(buf []byte) *Node { return LeafFromBytes(buf) }, 32},
		{"LeafFromBytes_short", func(buf []byte) *Node { return LeafFromBytes(buf) }, 16},
		{"LeafFromBytes_multichunk", func(buf []byte) *Node { return LeafFromBytes(buf) }, 64},
		{"TreeFromChunks", func(buf []byte) *Node {
			tree, err := TreeFromChunks([][]byte{buf})
			if err != nil {
				t.Fatalf("TreeFromChunks: %v", err)
			}
			return tree
		}, 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, tt.size)
			buf[0] = 1

			node := tt.build(buf)
			before := bytes.Clone(node.Hash())

			// The caller reuses its scratch buffer.
			buf[0] = 0xff

			if after := node.Hash(); !bytes.Equal(before, after) {
				t.Fatalf("node hash changed after the caller mutated its buffer: %x -> %x", before, after)
			}
		})
	}
}

// TestTreeFromChunksRequiresFullChunks pins the chunk-length validation.
// hashPair copies each side into a fixed 64-byte block, so a short chunk was
// zero-extended and a long one truncated: distinct inputs collided, and a
// single-chunk tree returned fewer than the 32 bytes Node.Hash() documents.
func TestTreeFromChunksRequiresFullChunks(t *testing.T) {
	chunk := func(b byte) []byte {
		c := make([]byte, 32)
		c[0] = b
		return c
	}

	tests := []struct {
		name    string
		chunks  [][]byte
		wantErr string
	}{
		{"short_single", [][]byte{{1, 2, 3}}, "chunk 0 has length 3, want 32"},
		{"short_pair", [][]byte{{1}, {2}}, "chunk 0 has length 1, want 32"},
		{"short_second", [][]byte{chunk(1), {2}}, "chunk 1 has length 1, want 32"},
		{"long", [][]byte{make([]byte, 40)}, "chunk 0 has length 40, want 32"},
		{"empty_chunk", [][]byte{{}}, "chunk 0 has length 0, want 32"},
		{"exact", [][]byte{chunk(1), chunk(2)}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := TreeFromChunks(tt.chunks)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("TreeFromChunks error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(tree.Hash()); got != 32 {
				t.Fatalf("Hash() length = %d, want 32", got)
			}
		})
	}
}

// Finalize(WithHashFn) hashes through the given function and produces the
// same root as the default backend.
func TestFinalizeWithHashFn(t *testing.T) {
	chunks := finalizeTestChunks(64)

	ref, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Clone(refHash(ref))

	var calls int
	countingFn := func(dst, input []byte) error {
		calls++
		return batchHashFn(dst, input)
	}

	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if finalizeErr := tree.Finalize(WithHashFn(countingFn)); finalizeErr != nil {
		t.Fatalf("Finalize: %v", finalizeErr)
	}
	if got := tree.Hash(); !bytes.Equal(got, want) {
		t.Fatalf("Finalize(WithHashFn) root %x, want %x", got, want)
	}
	if calls == 0 {
		t.Fatal("custom hash function was not used")
	}
}

// WithHashFn reaches trees below the batching threshold through the
// recursive path, and a failing function surfaces its error while leaving a
// resumable tree.
func TestFinalizeWithHashFnSmallTree(t *testing.T) {
	chunks := finalizeTestChunks(8)

	ref, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Clone(refHash(ref))

	var calls int
	countingFn := func(dst, input []byte) error {
		calls++
		return batchHashFn(dst, input)
	}
	tree, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if finalizeErr := tree.Finalize(WithHashFn(countingFn)); finalizeErr != nil {
		t.Fatalf("Finalize: %v", finalizeErr)
	}
	if calls == 0 {
		t.Fatal("custom hash function was not used below the batching threshold")
	}
	if got := tree.Hash(); !bytes.Equal(got, want) {
		t.Fatalf("small-tree WithHashFn root %x, want %x", got, want)
	}

	failing, err := TreeFromChunks(chunks)
	if err != nil {
		t.Fatal(err)
	}
	if finalizeErr := failing.Finalize(WithHashFn(func(_, _ []byte) error {
		return errors.New("backend failure")
	})); finalizeErr == nil {
		t.Fatal("expected the failing hash function's error")
	}
	if finalizeErr := failing.Finalize(); finalizeErr != nil {
		t.Fatalf("resumed Finalize: %v", finalizeErr)
	}
	if got := failing.Hash(); !bytes.Equal(got, want) {
		t.Fatalf("resumed root %x, want %x", got, want)
	}
	assertAllBranchesHashed(t, failing)
}

// A branch belongs to the Algorithm it was built for: it is refused in a
// tree of another hash, while a leaf is taken by any tree.
func TestSubtreeOfAnotherHashIsRefused(t *testing.T) {
	other := customAlgorithm(t, 0x02, nil)
	otherOpt := WithAlgorithm(other)
	leaf := LeafFromUint64(1)
	shaBranch := NewNodeWithLR(LeafFromUint64(1), LeafFromUint64(2))
	nativeBranch := NewNodeWithLR(LeafFromUint64(1), LeafFromUint64(2), WithAlgorithm(hasher.SHA256Native))
	otherBranch := NewNodeWithLR(LeafFromUint64(1), LeafFromUint64(2), otherOpt)

	// A leaf and a branch of a compatible Algorithm are taken.
	for name, n := range map[string]*Node{"leaf": leaf, "native branch": nativeBranch, "branch": shaBranch} {
		w := NewWrapper()
		idx := w.Index()
		w.AddNode(n)
		w.AddNode(LeafFromUint64(3))
		w.Merkleize(idx)
		if _, err := w.HashRoot(); err != nil {
			t.Errorf("%s in a SHA256 tree: %v", name, err)
		}
	}

	w := NewWrapper(otherOpt)
	idx := w.Index()
	w.AddNode(shaBranch)
	w.AddNode(leaf)
	w.Merkleize(idx)
	if _, err := w.HashRoot(); !errors.Is(err, sszutils.ErrHashAlgorithmMismatch) {
		t.Errorf("SHA256 branch in a tree of another hash: err = %v, want ErrHashAlgorithmMismatch", err)
	}

	// A refused subtree is not hashed by the tree that refused it: read mid
	// walk or reduced with its region, it keeps the root of its own hash.
	for name, walk := range map[string]func(w *Wrapper, n *Node){
		"read mid walk": func(w *Wrapper, n *Node) {
			walkIdx := w.Index()
			w.AddNode(n)
			_ = w.Hash()
			w.Merkleize(walkIdx)
		},
		"reduced with a value": func(w *Wrapper, n *Node) {
			walkIdx := w.Index()
			w.AddNode(n)
			w.PutUint64(7)
			w.Merkleize(walkIdx)
		},
	} {
		unhashed := NewNodeWithLR(LeafFromUint64(1), LeafFromUint64(2))
		refusing := NewWrapper(otherOpt)
		walk(refusing, unhashed)
		if _, err := refusing.Root(); !errors.Is(err, sszutils.ErrHashAlgorithmMismatch) {
			t.Errorf("%s: err = %v, want ErrHashAlgorithmMismatch", name, err)
		}
		if want := hashPair(LeafFromUint64(1).Hash(), LeafFromUint64(2).Hash()); !bytes.Equal(unhashed.Hash(), want) {
			t.Errorf("%s: the refused subtree hashes to %x, want its SHA256 root %x", name, unhashed.Hash(), want)
		}
	}

	if _, err := TreeFromNodes([]*Node{shaBranch, leaf}, 2, otherOpt); !errors.Is(err, sszutils.ErrHashAlgorithmMismatch) {
		t.Errorf("TreeFromNodes: err = %v, want ErrHashAlgorithmMismatch", err)
	}
	tree, err := TreeFromNodes([]*Node{otherBranch, leaf}, 4, otherOpt)
	if err != nil {
		t.Fatalf("TreeFromNodes of a branch of that hash: %v", err)
	}
	if tree.Algorithm() != other {
		t.Error("the tree does not carry the algorithm it was built for")
	}

	// A tree that is one leaf carries the Algorithm it was asked for, and a
	// padding node reads as the zero subtree root of that Algorithm.
	single, err := TreeFromNodes([]*Node{leaf}, 1, otherOpt)
	if err != nil || single.Algorithm() != other {
		t.Errorf("a tree of one leaf: algorithm of the leaf's handle, err = %v", err)
	}
	padding, err := TreeFromNodes([]*Node{NewEmptyNode(hasher.SHA256.ZeroHash(5))}, 1, otherOpt)
	if err != nil || !bytes.Equal(padding.Hash(), other.ZeroHash(5)) {
		t.Errorf("a tree of one padding node: root %x, err = %v", padding.Hash(), err)
	}

	// A branch cannot be made over a subtree of another hash.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("NewNodeWithLR over a subtree of another hash did not panic")
			}
		}()
		NewNodeWithLR(shaBranch, leaf, otherOpt)
	}()
}

// Every public constructor and builder makes a tree of the named Algorithm:
// its root is the one a hasher of that Algorithm gives for the same chunks.
func TestBuildersPerAlgorithm(t *testing.T) {
	chunk := func(v uint64) []byte {
		b := make([]byte, 32)
		binary.LittleEndian.PutUint64(b, v)
		return b
	}
	walk := func(n int, reduce func(hh sszutils.HashWalker, idx int)) func(hh sszutils.HashWalker) {
		return func(hh sszutils.HashWalker) {
			idx := hh.StartTree(sszutils.TreeTypeNone)
			for i := 0; i < n; i++ {
				hh.Append(chunk(uint64(i + 1)))
			}
			reduce(hh, idx)
		}
	}
	activeFields := bytes.Repeat([]byte{0x55}, 70)

	for name, alg := range testAlgorithms(t) {
		opt := WithAlgorithm(alg)
		leaves := func(n int) []*Node {
			out := make([]*Node, n)
			for i := range out {
				out[i] = NewNodeWithValue(chunk(uint64(i+1)), opt)
			}
			return out
		}
		for _, tc := range []struct {
			name  string
			build func() (*Node, error)
			walk  func(hh sszutils.HashWalker)
		}{
			{
				"chunks",
				func() (*Node, error) { return TreeFromChunks([][]byte{chunk(1), chunk(2), chunk(3), chunk(4)}, opt) },
				walk(4, func(hh sszutils.HashWalker, idx int) { hh.Merkleize(idx) }),
			},
			{
				"mixin",
				func() (*Node, error) { return TreeFromNodesWithMixin(leaves(5), 5, 64, opt) },
				walk(5, func(hh sszutils.HashWalker, idx int) { hh.MerkleizeWithMixin(idx, 5, 64) }),
			},
			{
				"mixin 64",
				func() (*Node, error) { return TreeFromNodesWithMixin64(leaves(5), 5, 64, opt) },
				walk(5, func(hh sszutils.HashWalker, idx int) { hh.MerkleizeWithMixin(idx, 5, 64) }),
			},
			{
				"padded under a length",
				func() (*Node, error) {
					data, err := TreeFromNodes(leaves(5), 64, opt)
					if err != nil {
						return nil, err
					}
					return NewNodeWithLR(data, LeafFromUint64(5, opt), opt), nil
				},
				walk(5, func(hh sszutils.HashWalker, idx int) { hh.MerkleizeWithMixin(idx, 5, 64) }),
			},
			{
				"padded 64 under a length",
				func() (*Node, error) {
					data, err := TreeFromNodes64(leaves(5), 64, opt)
					if err != nil {
						return nil, err
					}
					return NewNodeWithLR(data, LeafFromUint32(5, opt), opt), nil
				},
				walk(5, func(hh sszutils.HashWalker, idx int) { hh.MerkleizeWithMixin(idx, 5, 64) }),
			},
			{
				"progressive",
				func() (*Node, error) { return TreeFromNodesProgressive(leaves(7), opt) },
				walk(7, func(hh sszutils.HashWalker, idx int) { hh.MerkleizeProgressive(idx) }),
			},
			{
				"progressive mixin",
				func() (*Node, error) { return TreeFromNodesProgressiveWithMixin(leaves(7), 7, opt) },
				walk(7, func(hh sszutils.HashWalker, idx int) { hh.MerkleizeProgressiveWithMixin(idx, 7) }),
			},
			{
				"progressive mixin 64",
				func() (*Node, error) { return TreeFromNodesProgressiveWithMixin64(leaves(7), 7, opt) },
				walk(7, func(hh sszutils.HashWalker, idx int) { hh.MerkleizeProgressiveWithMixin(idx, 7) }),
			},
			{
				"progressive active fields",
				func() (*Node, error) { return TreeFromNodesProgressiveWithActiveFields(leaves(3), activeFields, opt) },
				walk(3, func(hh sszutils.HashWalker, idx int) { hh.MerkleizeProgressiveWithActiveFields(idx, activeFields) }),
			},
			{
				"long leaf",
				func() (*Node, error) { return LeafFromBytes(bytes.Repeat([]byte{7}, 96), opt), nil },
				func(hh sszutils.HashWalker) { hh.PutBytes(bytes.Repeat([]byte{7}, 96)) },
			},
			{
				"packed leaves with padding",
				func() (*Node, error) {
					packed := LeavesFromUint64([]uint64{1, 2, 3, 4, 5}, opt)
					return TreeFromNodes([]*Node{packed[0], packed[1], EmptyLeaf(opt), NewEmptyNode(make([]byte, 32), opt)}, 4, opt)
				},
				func(hh sszutils.HashWalker) {
					idx := hh.StartTree(sszutils.TreeTypeNone)
					for _, v := range []uint64{1, 2, 3, 4, 5} {
						hh.AppendUint64(v)
					}
					hh.FillUpTo32()
					hh.Append(make([]byte, 64))
					hh.Merkleize(idx)
				},
			},
			{
				"small leaves",
				func() (*Node, error) {
					return TreeFromNodes([]*Node{LeafFromUint16(7, opt), LeafFromUint8(9, opt), LeafFromBool(true, opt), LeafFromBool(false, opt)}, 4, opt)
				},
				func(hh sszutils.HashWalker) {
					idx := hh.StartTree(sszutils.TreeTypeNone)
					hh.PutUint16(7)
					hh.PutUint8(9)
					hh.PutBool(true)
					hh.PutBool(false)
					hh.Merkleize(idx)
				},
			},
		} {
			tree, err := tc.build()
			if err != nil {
				t.Fatalf("%s %s: build: %v", name, tc.name, err)
			}
			if tree.Algorithm() != alg {
				t.Errorf("%s %s: the tree does not carry its algorithm", name, tc.name)
			}
			want := algorithmRoot(t, alg, tc.walk)
			if !bytes.Equal(tree.Hash(), want[:]) {
				t.Errorf("%s %s: root %x, hasher root %x", name, tc.name, tree.Hash(), want)
			}
		}
		if len(LeavesFromUint64(nil, opt)) != 0 {
			t.Errorf("%s: LeavesFromUint64 of nothing made leaves", name)
		}
	}
}

// A node marked empty whose value is no zero subtree root of any hash in use
// has nothing beneath it.
func TestEmptyNodeWithUnknownRootHasNoChildren(t *testing.T) {
	node := NewEmptyNode(bytes.Repeat([]byte{0xff}, 32))
	if child, err := node.Get(2); err == nil || child != nil {
		t.Errorf("Get(2) = %v, %v; want an error", child, err)
	}
}

// A wrapper made for a bare function builds the tree of the hash that
// function computes.
func TestWrapperWithHashFnDerivesItsAlgorithm(t *testing.T) {
	for name, tc := range map[string]struct {
		fn      hasher.HashFn
		alg     *hasher.Algorithm
		foreign bool
	}{
		"custom hash":    {saltedSHA256(0x05, nil), customAlgorithm(t, 0x05, nil), true},
		"sha256 backend": {saltedSHA256(0x00, nil), hasher.SHA256, false},
	} {
		w := NewWrapperWithHashFn(tc.fn)
		if !w.Algorithm().Compatible(tc.alg) {
			t.Fatalf("%s: the wrapper did not derive the algorithm of its function", name)
		}
		paddedListWalk(w)
		node, err := w.Root()
		if err != nil {
			t.Fatalf("%s: Root: %v", name, err)
		}
		want := algorithmRoot(t, tc.alg, paddedListWalk)
		if !bytes.Equal(node.Hash(), want[:]) {
			t.Errorf("%s: root %x, want %x", name, node.Hash(), want)
		}
	}
}

func TestWrapperAddMethods(t *testing.T) {
	t.Run("AddBytes", func(t *testing.T) {
		// Small bytes
		w := NewWrapper()
		w.AddBytes([]byte{1, 2, 3})

		if nodeCount(w) != 1 {
			t.Error("AddBytes (small) should add one node")
		}

		// Large bytes
		w2 := NewWrapper()
		w2.AddBytes(bytes.Repeat([]byte{1}, 100))

		if len(w2.nodes) != 1 {
			t.Error("AddBytes (large) should create merkleized node")
		}
	})

	t.Run("AddUint64", func(t *testing.T) {
		w := NewWrapper()
		w.AddUint64(0xFFFFFFFFFFFFFFFF)

		if nodeCount(w) != 1 {
			t.Error("AddUint64 should add one node")
		}

		expected := make([]byte, 32)
		binary.LittleEndian.PutUint64(expected[:8], 0xFFFFFFFFFFFFFFFF)
		if !bytes.Equal(nodeAt(w, 0).n.value[:], expected) {
			t.Error("AddUint64 value mismatch")
		}
	})

	t.Run("AddSmallUints", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			add  func(w *Wrapper)
			want []byte
		}{
			{"AddUint32", func(w *Wrapper) { w.AddUint32(0x11223344) }, []byte{0x44, 0x33, 0x22, 0x11}},
			{"AddUint16", func(w *Wrapper) { w.AddUint16(0x1122) }, []byte{0x22, 0x11}},
			{"AddUint8", func(w *Wrapper) { w.AddUint8(0x11) }, []byte{0x11}},
		} {
			w := NewWrapper()
			tc.add(w)
			if nodeCount(w) != 1 {
				t.Fatalf("%s added %d chunks, want 1", tc.name, nodeCount(w))
			}
			want := make([]byte, 32)
			copy(want, tc.want)
			if got := nodeAt(w, 0).n.value[:]; !bytes.Equal(got, want) {
				t.Errorf("%s value = %x, want %x", tc.name, got, want)
			}
		}
	})

	t.Run("AddBytesEmpty", func(t *testing.T) {
		// A value added through this API takes a place in the leaf order, an
		// empty one included: it is a leaf of zeros, as it has been since v1.
		w := NewWrapper()
		w.AddBytes(nil)
		root, err := w.Root()
		if err != nil {
			t.Fatalf("empty AddBytes: %v", err)
		}
		if !bytes.Equal(root.Hash(), make([]byte, 32)) {
			t.Errorf("empty AddBytes gave %x, want a leaf of zeros", root.Hash())
		}

		// Among siblings it keeps its position rather than shifting the rest.
		w2 := NewWrapper()
		idx := w2.Index()
		w2.AddUint64(1)
		w2.AddBytes(nil)
		w2.AddUint64(2)
		w2.Merkleize(idx)
		with, err := w2.Root()
		if err != nil {
			t.Fatalf("with the empty value: %v", err)
		}

		w3 := NewWrapper()
		idx = w3.Index()
		w3.AddUint64(1)
		w3.AddNode(LeafFromBytes(nil))
		w3.AddUint64(2)
		w3.Merkleize(idx)
		spelled, err := w3.Root()
		if err != nil {
			t.Fatalf("with the leaf spelled out: %v", err)
		}
		if !bytes.Equal(with.Hash(), spelled.Hash()) {
			t.Errorf("root = %x, want the same as an explicit zero leaf %x", with.Hash(), spelled.Hash())
		}
	})

	t.Run("AddNode", func(t *testing.T) {
		w := NewWrapper()
		node := NewNodeWithValue(bytes.Repeat([]byte{42}, 32))
		w.AddNode(node)

		if nodeCount(w) != 1 || w.nodes[0].node != node.n {
			t.Error("AddNode failed")
		}
	})

	t.Run("AddEmpty", func(t *testing.T) {
		w := NewWrapper()
		w.addEmpty()

		if nodeCount(w) != 1 {
			t.Error("AddEmpty should add one node")
		}

		if !nodeAt(w, 0).IsEmpty() {
			t.Error("AddEmpty should add empty node")
		}
	})
}

// The v1 exported Commit*/AddEmpty API is retained as thin aliases over the
// Merkleize* methods; each must produce the same root as its counterpart.
func TestWrapperCommitAliases(t *testing.T) {
	build := func(fn func(w *Wrapper)) []byte {
		w := NewWrapper()
		w.PutUint64(1)
		w.PutUint64(2)
		w.PutUint64(3)
		fn(w)
		return w.Node().Hash()
	}

	if !bytes.Equal(build(func(w *Wrapper) { w.Commit(0) }),
		build(func(w *Wrapper) { w.Merkleize(0) })) {
		t.Error("Commit should match Merkleize")
	}
	if !bytes.Equal(build(func(w *Wrapper) { w.CommitWithMixin(0, 3, 8) }),
		build(func(w *Wrapper) { w.MerkleizeWithMixin(0, 3, 8) })) {
		t.Error("CommitWithMixin should match MerkleizeWithMixin")
	}
	if !bytes.Equal(build(func(w *Wrapper) { w.CommitProgressive(0) }),
		build(func(w *Wrapper) { w.MerkleizeProgressive(0) })) {
		t.Error("CommitProgressive should match MerkleizeProgressive")
	}
	if !bytes.Equal(build(func(w *Wrapper) { w.CommitProgressiveWithMixin(0, 3) }),
		build(func(w *Wrapper) { w.MerkleizeProgressiveWithMixin(0, 3) })) {
		t.Error("CommitProgressiveWithMixin should match MerkleizeProgressiveWithMixin")
	}
	activeFields := []byte{0x07}
	if !bytes.Equal(build(func(w *Wrapper) { w.CommitProgressiveWithActiveFields(0, activeFields) }),
		build(func(w *Wrapper) { w.MerkleizeProgressiveWithActiveFields(0, activeFields) })) {
		t.Error("CommitProgressiveWithActiveFields should match MerkleizeProgressiveWithActiveFields")
	}

	// AddEmpty adds a zero leaf like the internal addEmpty.
	w := NewWrapper()
	w.AddEmpty()
	if nodeCount(w) != 1 || !w.nodes[0].node.isEmpty {
		t.Error("AddEmpty should append a single zero leaf")
	}
}

// A node added through the legacy Add* API keeps its identity as a leaf of the
// enclosing scope, even when bytes were appended before it: it takes a chunk
// of its own rather than being smeared across two.
func TestWrapperAddNodeKeepsLeafIdentity(t *testing.T) {
	w := NewWrapper()
	idx := w.StartTree(sszutils.TreeTypeNone)
	w.AppendUint64(0x1122334455667788)
	w.AddUint64(0xdeadbeef)
	w.Merkleize(idx)

	root, err := w.Root()
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if root.n.left == nil || root.n.right == nil {
		t.Fatalf("root is not a two-leaf tree: %v", root)
	}
	want := LeafFromUint64(0xdeadbeef)
	if !bytes.Equal(root.Right().Hash(), want.Hash()) {
		t.Fatalf("second leaf = %x, want the added node %x", root.Right().Hash(), want.Hash())
	}
}

func TestAddNodeWithNilSlice(t *testing.T) {
	// Create wrapper with zero value (nil nodes) to cover the nil check in AddNode
	w := &Wrapper{}
	w.AddNode(NewNodeWithValue([]byte{1}))

	if nodeCount(w) != 1 {
		t.Fatalf("expected 1 node, got %d", nodeCount(w))
	}
}

func TestWrapperAddNodeNil(t *testing.T) {
	w := NewWrapper()

	// Test AddNode with nil node - should handle gracefully
	w.AddNode(nil)

	// Should have one node (even if nil)
	if nodeCount(w) != 1 {
		t.Error("AddNode should add the node even if nil")
	}
}
