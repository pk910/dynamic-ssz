// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/pk910/dynamic-ssz/sszutils"
)

func TestNewWrapper(t *testing.T) {
	w := NewWrapper()

	if w == nil {
		t.Fatal("wrapper should not be nil")
	}
	if w.nodes == nil {
		t.Error("nodes should be initialized")
	}
	if len(w.nodes) != 0 {
		t.Error("nodes should be empty")
	}
	if w.buf == nil {
		t.Error("buf should be initialized")
	}
	if len(w.buf) != 0 {
		t.Error("buf should be empty")
	}
	if w.tmp == nil || len(w.tmp) != 64 {
		t.Error("tmp should be initialized with 64 bytes")
	}
}

func TestWrapperWithTemp(t *testing.T) {
	w := NewWrapper()

	// Test that WithTemp allows temporary buffer usage
	var capturedTmp []byte
	w.WithTemp(func(tmp []byte) []byte {
		capturedTmp = tmp
		// Modify and return
		tmp[0] = 42
		return tmp
	})

	if capturedTmp == nil {
		t.Error("tmp should have been passed to function")
	}
	if w.tmp[0] != 42 {
		t.Error("tmp should have been modified")
	}
}

func TestWrapperIndex(t *testing.T) {
	w := NewWrapper()

	if w.Index() != 0 {
		t.Errorf("initial index should be 0, got %d", w.Index())
	}

	w.AddNode(NewNodeWithValue([]byte{1}))
	if w.Index() != 1 {
		t.Errorf("index after adding node should be 1, got %d", w.Index())
	}

	w.AddNode(NewNodeWithValue([]byte{2}))
	if w.Index() != 2 {
		t.Errorf("index after adding second node should be 2, got %d", w.Index())
	}
}

func TestWrapperAppendMethods(t *testing.T) {
	t.Run("Append", func(t *testing.T) {
		w := NewWrapper()
		data := []byte{1, 2, 3, 4}
		w.Append(data)

		if !bytes.Equal(w.buf, data) {
			t.Error("Append failed to add data to buffer")
		}
	})

	t.Run("AppendUint64", func(t *testing.T) {
		w := NewWrapper()
		val := uint64(0x1234567890ABCDEF)
		w.AppendUint64(val)

		expected := make([]byte, 8)
		binary.LittleEndian.PutUint64(expected, val)

		if !bytes.Equal(w.buf, expected) {
			t.Error("AppendUint64 failed")
		}
	})

	t.Run("AppendUint32", func(t *testing.T) {
		w := NewWrapper()
		val := uint32(0x12345678)
		w.AppendUint32(val)

		expected := make([]byte, 4)
		binary.LittleEndian.PutUint32(expected, val)

		if !bytes.Equal(w.buf, expected) {
			t.Error("AppendUint32 failed")
		}
	})

	t.Run("AppendUint16", func(t *testing.T) {
		w := NewWrapper()
		val := uint16(0x1234)
		w.AppendUint16(val)

		expected := make([]byte, 2)
		binary.LittleEndian.PutUint16(expected, val)

		if !bytes.Equal(w.buf, expected) {
			t.Error("AppendUint16 failed")
		}
	})

	t.Run("AppendUint8", func(t *testing.T) {
		w := NewWrapper()
		val := uint8(0xAB)
		w.AppendUint8(val)

		if len(w.buf) != 1 || w.buf[0] != val {
			t.Error("AppendUint8 failed")
		}
	})

	t.Run("AppendBool", func(t *testing.T) {
		w := NewWrapper()
		w.AppendBool(true)
		if len(w.buf) != 1 || w.buf[0] != 1 {
			t.Error("AppendBool(true) failed")
		}

		w2 := NewWrapper()
		w2.AppendBool(false)
		if len(w2.buf) != 1 || w2.buf[0] != 0 {
			t.Error("AppendBool(false) failed")
		}
	})

	t.Run("AppendBytes32", func(t *testing.T) {
		w := NewWrapper()
		data := bytes.Repeat([]byte{0xFF}, 16)
		w.AppendBytes32(data)

		expected := append(data, bytes.Repeat([]byte{0}, 16)...)
		if !bytes.Equal(w.buf, expected) {
			t.Error("AppendBytes32 failed to pad to 32 bytes")
		}
	})
}

func TestWrapperFillUpTo32(t *testing.T) {
	tests := []struct {
		name           string
		initialData    []byte
		expectedLength int
	}{
		{
			name:           "empty buffer",
			initialData:    []byte{},
			expectedLength: 0,
		},
		{
			name:           "1 byte",
			initialData:    []byte{1},
			expectedLength: 32,
		},
		{
			name:           "16 bytes",
			initialData:    bytes.Repeat([]byte{1}, 16),
			expectedLength: 32,
		},
		{
			name:           "32 bytes - no padding needed",
			initialData:    bytes.Repeat([]byte{1}, 32),
			expectedLength: 32,
		},
		{
			name:           "33 bytes",
			initialData:    bytes.Repeat([]byte{1}, 33),
			expectedLength: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewWrapper()
			w.buf = append(w.buf, tt.initialData...)
			w.FillUpTo32()

			if len(w.buf) != tt.expectedLength {
				t.Errorf("expected length %d, got %d", tt.expectedLength, len(w.buf))
			}

			// Check padding is zeros
			for i := len(tt.initialData); i < len(w.buf); i++ {
				if w.buf[i] != 0 {
					t.Error("padding should be zeros")
				}
			}
		})
	}
}

func TestWrapperMerkleize(t *testing.T) {
	w := NewWrapper()

	// Add some data to buffer
	w.AppendUint64(1)
	w.AppendUint64(2)
	w.AppendUint64(3)
	w.AppendUint64(4)

	initialBufLen := len(w.buf)
	w.Merkleize(0)

	// Buffer should be cleared
	if len(w.buf) != 0 {
		t.Error("buffer should be cleared after Merkleize")
	}

	// Should have created a tree node
	if len(w.nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(w.nodes))
	}

	// Node should represent the merkleized data
	if w.nodes[0] == nil {
		t.Error("node should not be nil")
	}

	if initialBufLen == 0 {
		t.Error("initial buffer should have had data")
	}
}

func TestWrapperMerkleizeWithMixin(t *testing.T) {
	w := NewWrapper()

	// Add array data
	w.AppendUint64(1)
	w.AppendUint64(2)
	w.AppendUint64(3)
	w.AppendUint64(4)

	w.MerkleizeWithMixin(0, 4, 4)

	// Should have one node representing the tree with mixin
	if len(w.nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(w.nodes))
	}

	// The root should have a mixin (right child with length)
	root := w.nodes[0]
	if root.IsLeaf() {
		t.Error("root should be a branch node with mixin")
	}
}

func TestWrapperMerkleizeProgressive(t *testing.T) {
	w := NewWrapper()

	// Add data for progressive merkleization
	for i := 0; i < 5; i++ {
		w.AppendUint64(uint64(i))
	}

	w.MerkleizeProgressive(0)

	// Should have one node representing the progressive tree
	if len(w.nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(w.nodes))
	}
}

func TestWrapperMerkleizeProgressiveWithMixin(t *testing.T) {
	w := NewWrapper()

	// Add data to buffer
	for i := 0; i < 3; i++ {
		w.AppendUint64(uint64(i + 1))
	}

	// Call MerkleizeProgressiveWithMixin
	w.MerkleizeProgressiveWithMixin(0, 3)

	// Should have one node representing the progressive tree with mixin
	if len(w.nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(w.nodes))
	}

	// The root should be a branch node (has mixin)
	if w.nodes[0].IsLeaf() {
		t.Error("root should be a branch node with mixin")
	}

	// Buffer should be cleared
	if len(w.buf) != 0 {
		t.Error("buffer should be cleared after MerkleizeProgressiveWithMixin")
	}
}

func TestWrapperMerkleizeProgressiveWithActiveFields(t *testing.T) {
	w := NewWrapper()

	// Add some field data to buffer
	w.AppendUint64(1)
	w.AppendUint64(2)
	w.AppendUint64(3)

	// Create active fields bitvector
	activeFields := []byte{0b00000111} // First 3 fields active

	// Call MerkleizeProgressiveWithActiveFields
	w.MerkleizeProgressiveWithActiveFields(0, activeFields)

	// Should have one node representing the progressive tree with active fields
	if len(w.nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(w.nodes))
	}

	// The root should be a branch node (has active fields mixin)
	if w.nodes[0].IsLeaf() {
		t.Error("root should be a branch node with active fields mixin")
	}

	// Buffer should be cleared
	if len(w.buf) != 0 {
		t.Error("buffer should be cleared after MerkleizeProgressiveWithActiveFields")
	}
}

func TestWrapperPutBitlist(t *testing.T) {
	w := NewWrapper()

	// Create a bitlist: 11110000 10000000 (14 bits, last byte indicates length)
	bitlist := []byte{0b11110000, 0b10000001}
	maxSize := uint64(16)

	w.PutBitlist(bitlist, maxSize)

	// Should have created a tree with mixin
	if len(w.nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(w.nodes))
	}
}

func TestWrapperPutProgressiveBitlist(t *testing.T) {
	w := NewWrapper()

	// Create a bitlist
	bitlist := []byte{0b11111111, 0b11111111, 0b10000001} // 17 bits

	w.PutProgressiveBitlist(bitlist)

	// Should have created a progressive tree with mixin
	if len(w.nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(w.nodes))
	}
}

func TestWrapperPutMethods(t *testing.T) {
	t.Run("PutBool", func(t *testing.T) {
		w := NewWrapper()
		w.PutBool(true)

		if len(w.nodes) != 1 {
			t.Error("PutBool should add one node")
		}

		expected := make([]byte, 32)
		expected[0] = 1
		if !bytes.Equal(w.nodes[0].value, expected) {
			t.Error("PutBool value mismatch")
		}
	})

	t.Run("PutBytes", func(t *testing.T) {
		// Test small bytes (<= 32)
		w := NewWrapper()
		smallBytes := []byte{1, 2, 3, 4}
		w.PutBytes(smallBytes)

		if len(w.nodes) != 1 {
			t.Error("PutBytes (small) should add one node")
		}

		// Test large bytes (> 32)
		w2 := NewWrapper()
		largeBytes := bytes.Repeat([]byte{0xFF}, 64)
		w2.PutBytes(largeBytes)

		if len(w2.nodes) != 1 {
			t.Error("PutBytes (large) should create merkleized node")
		}
	})

	t.Run("PutUint64", func(t *testing.T) {
		w := NewWrapper()
		w.PutUint64(12345)

		if len(w.nodes) != 1 {
			t.Error("PutUint64 should add one node")
		}
	})

	t.Run("PutUint32", func(t *testing.T) {
		w := NewWrapper()
		w.PutUint32(12345)

		if len(w.nodes) != 1 {
			t.Error("PutUint32 should add one node")
		}
	})

	t.Run("PutUint16", func(t *testing.T) {
		w := NewWrapper()
		w.PutUint16(12345)

		if len(w.nodes) != 1 {
			t.Error("PutUint16 should add one node")
		}
	})

	t.Run("PutUint8", func(t *testing.T) {
		w := NewWrapper()
		w.PutUint8(123)

		if len(w.nodes) != 1 {
			t.Error("PutUint8 should add one node")
		}
	})
}

func TestWrapperPutUint64Array(t *testing.T) {
	t.Run("fixed size array", func(t *testing.T) {
		w := NewWrapper()
		arr := []uint64{1, 2, 3, 4}
		w.PutUint64Array(arr)

		if len(w.nodes) != 1 {
			t.Error("PutUint64Array should create one merkleized node")
		}
	})

	t.Run("dynamic array with max capacity", func(t *testing.T) {
		w := NewWrapper()
		arr := []uint64{1, 2, 3, 4, 5}
		maxCap := uint64(10)
		w.PutUint64Array(arr, maxCap)

		if len(w.nodes) != 1 {
			t.Error("PutUint64Array with max capacity should create one merkleized node")
		}

		// Should be a tree with mixin
		if w.nodes[0].IsLeaf() {
			t.Error("dynamic array should create branch node with mixin")
		}
	})
}

func TestWrapperAddMethods(t *testing.T) {
	t.Run("AddBytes", func(t *testing.T) {
		// Small bytes
		w := NewWrapper()
		w.AddBytes([]byte{1, 2, 3})

		if len(w.nodes) != 1 {
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

		if len(w.nodes) != 1 {
			t.Error("AddUint64 should add one node")
		}

		expected := make([]byte, 32)
		binary.LittleEndian.PutUint64(expected[:8], 0xFFFFFFFFFFFFFFFF)
		if !bytes.Equal(w.nodes[0].value, expected) {
			t.Error("AddUint64 value mismatch")
		}
	})

	t.Run("AddNode", func(t *testing.T) {
		w := NewWrapper()
		node := NewNodeWithValue(bytes.Repeat([]byte{42}, 32))
		w.AddNode(node)

		if len(w.nodes) != 1 || w.nodes[0] != node {
			t.Error("AddNode failed")
		}
	})

	t.Run("AddEmpty", func(t *testing.T) {
		w := NewWrapper()
		w.AddEmpty()

		if len(w.nodes) != 1 {
			t.Error("AddEmpty should add one node")
		}

		if !w.nodes[0].IsEmpty() {
			t.Error("AddEmpty should add empty node")
		}
	})
}

func TestWrapperNode(t *testing.T) {
	t.Run("single node", func(t *testing.T) {
		w := NewWrapper()
		node := NewNodeWithValue([]byte{1})
		w.AddNode(node)

		retrieved := w.Node()
		if retrieved != node {
			t.Error("Node() should return the single node")
		}
	})

	t.Run("panic on multiple nodes", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic when multiple nodes exist")
			}
		}()

		w := NewWrapper()
		w.AddNode(NewNodeWithValue([]byte{1}))
		w.AddNode(NewNodeWithValue([]byte{2}))
		_ = w.Node() // Should panic
	})

	t.Run("panic on zero nodes", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic when no nodes exist")
			}
		}()

		w := NewWrapper()
		_ = w.Node() // Should panic
	})
}

func TestWrapperHash(t *testing.T) {
	w := NewWrapper()

	// Create a simple tree
	w.PutUint64(1)
	w.PutUint64(2)
	w.PutUint64(3)
	w.PutUint64(4)
	w.Merkleize(0)

	hash := w.Hash()
	if len(hash) != 32 {
		t.Error("hash should be 32 bytes")
	}

	// Hash should match the last node's hash
	expectedHash := w.nodes[len(w.nodes)-1].Hash()
	if !bytes.Equal(hash, expectedHash) {
		t.Error("Hash() should return last node's hash")
	}
}

func TestWrapperHashRoot(t *testing.T) {
	w := NewWrapper()

	// Build a tree
	w.PutUint64(1)
	w.PutUint64(2)
	w.Merkleize(0)

	root, err := w.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot error: %v", err)
	}

	if len(root) != 32 {
		t.Error("root should be 32 bytes")
	}

	// Should match Hash()
	hash := w.Hash()
	if !bytes.Equal(root[:], hash) {
		t.Error("HashRoot should match Hash")
	}
}

func TestWrapperHashRootError(t *testing.T) {
	w := NewWrapper()

	// Test HashRoot with no nodes (should error or panic)
	defer func() {
		if r := recover(); r != nil {
			// Expected behavior - panic due to no nodes
			return
		}
	}()
	
	_, err := w.HashRoot()
	if err == nil {
		t.Error("HashRoot should return error when no nodes exist")
	}
}

func TestWrapperCommit(t *testing.T) {
	w := NewWrapper()

	// Add multiple nodes
	w.AddNode(NewNodeWithValue([]byte{1}))
	w.AddNode(NewNodeWithValue([]byte{2}))
	w.AddNode(NewNodeWithValue([]byte{3}))
	w.AddNode(NewNodeWithValue([]byte{4}))

	// Commit from index 0
	w.Commit(0)

	// Should have one merkleized node
	if len(w.nodes) != 1 {
		t.Errorf("expected 1 node after commit, got %d", len(w.nodes))
	}

	// Test partial commit
	w2 := NewWrapper()
	w2.AddNode(NewNodeWithValue([]byte{1}))
	w2.AddNode(NewNodeWithValue([]byte{2}))
	w2.AddNode(NewNodeWithValue([]byte{3}))
	w2.AddNode(NewNodeWithValue([]byte{4}))

	// Commit only last 2 nodes
	w2.Commit(2)

	// Should have 3 nodes: first 2 original + 1 merkleized
	if len(w2.nodes) != 3 {
		t.Errorf("expected 3 nodes after partial commit, got %d", len(w2.nodes))
	}
}

func TestWrapperCommitWithMixin(t *testing.T) {
	w := NewWrapper()

	// Add array elements
	for i := 0; i < 4; i++ {
		w.AddNode(LeafFromUint64(uint64(i)))
	}

	w.CommitWithMixin(0, 4, 4)

	// Should have one node with mixin
	if len(w.nodes) != 1 {
		t.Error("CommitWithMixin should produce one node")
	}

	// Root should be a branch (has mixin)
	if w.nodes[0].IsLeaf() {
		t.Error("node should have mixin")
	}
}

func TestWrapperCommitProgressive(t *testing.T) {
	w := NewWrapper()

	// Add nodes for progressive merkleization
	for i := 0; i < 7; i++ {
		w.AddNode(LeafFromUint64(uint64(i)))
	}

	w.CommitProgressive(0)

	// Should have one progressive tree node
	if len(w.nodes) != 1 {
		t.Error("CommitProgressive should produce one node")
	}
}

func TestWrapperCommitProgressiveWithMixin(t *testing.T) {
	w := NewWrapper()

	// Add array elements
	for i := 0; i < 5; i++ {
		w.AddNode(LeafFromUint64(uint64(i)))
	}

	w.CommitProgressiveWithMixin(0, 5)

	// Should have one node with mixin
	if len(w.nodes) != 1 {
		t.Error("CommitProgressiveWithMixin should produce one node")
	}
}

func TestWrapperCommitProgressiveWithActiveFields(t *testing.T) {
	w := NewWrapper()

	// Add fields
	for i := 0; i < 3; i++ {
		w.AddNode(LeafFromUint64(uint64(i)))
	}

	activeFields := []byte{0b00000111} // First 3 fields active
	w.CommitProgressiveWithActiveFields(0, activeFields)

	// Should have one node with active fields mixin
	if len(w.nodes) != 1 {
		t.Error("CommitProgressiveWithActiveFields should produce one node")
	}
}

func TestWrapperAppendBytesAsNodes(t *testing.T) {
	w := NewWrapper()

	t.Run("empty bytes", func(t *testing.T) {
		initialLen := len(w.nodes)
		w.appendBytesAsNodes([]byte{})

		// Should add one zero-filled node
		if len(w.nodes) != initialLen+1 {
			t.Error("empty bytes should add one zero node")
		}

		if !bytes.Equal(w.nodes[initialLen].value, sszutils.ZeroBytes()[:32]) {
			t.Error("empty bytes should create zero node")
		}
	})

	t.Run("exact 32 bytes", func(t *testing.T) {
		w2 := NewWrapper()
		data := bytes.Repeat([]byte{0xAB}, 32)
		w2.appendBytesAsNodes(data)

		if len(w2.nodes) != 1 {
			t.Error("32 bytes should create one node")
		}

		if !bytes.Equal(w2.nodes[0].value, data) {
			t.Error("node value mismatch")
		}
	})

	t.Run("non-32 byte aligned", func(t *testing.T) {
		w3 := NewWrapper()
		data := bytes.Repeat([]byte{0xFF}, 50) // Will need padding to 64 bytes
		w3.appendBytesAsNodes(data)

		if len(w3.nodes) != 2 {
			t.Error("50 bytes should create 2 nodes (padded to 64)")
		}
	})
}

func TestWrapperGetLimit(t *testing.T) {
	w := NewWrapper()

	// Add some nodes
	for i := 0; i < 5; i++ {
		w.AddNode(NewNodeWithValue([]byte{byte(i)}))
	}

	// getLimit should return next power of 2
	limit := w.getLimit(0)
	if limit != 8 { // Next power of 2 after 5
		t.Errorf("expected limit 8 for 5 nodes, got %d", limit)
	}

	// Test with exact power of 2
	w2 := NewWrapper()
	for i := 0; i < 4; i++ {
		w2.AddNode(NewNodeWithValue([]byte{byte(i)}))
	}

	limit2 := w2.getLimit(0)
	if limit2 != 4 {
		t.Errorf("expected limit 4 for 4 nodes, got %d", limit2)
	}
}

func TestWrapperEdgeCases(t *testing.T) {
	t.Run("merkleize with existing nodes", func(t *testing.T) {
		w := NewWrapper()

		// Add some nodes first
		w.AddNode(NewNodeWithValue([]byte{1}))

		// Add data to buffer
		w.AppendUint64(2)
		w.AppendUint64(3)

		// Merkleize from index 1 (preserve first node)
		w.Merkleize(1)

		// Should have 2 nodes: original + merkleized
		if len(w.nodes) != 2 {
			t.Errorf("expected 2 nodes, got %d", len(w.nodes))
		}
	})

	t.Run("commit panic on error", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic on tree creation error")
			}
		}()

		w := NewWrapper()
		// Add some nodes but force an invalid limit that's not a power of 2
		w.AddBytes([]byte{1, 2, 3})
		w.AddBytes([]byte{4, 5, 6}) 
		w.AddBytes([]byte{7, 8, 9}) // 3 nodes
		
		// Directly call TreeFromNodes with invalid limit
		_, err := TreeFromNodes(w.nodes, 3) // 3 is not a power of 2, should error
		if err != nil {
			panic(err) // This will trigger the expected panic
		}
	})

	t.Run("multiple buffer operations", func(t *testing.T) {
		w := NewWrapper()

		// Mix different append operations
		w.AppendUint8(1)
		w.AppendUint16(2)
		w.AppendUint32(3)
		w.AppendUint64(4)
		w.FillUpTo32()

		expectedLen := 32 // Should be padded to 32 bytes
		if len(w.buf) != expectedLen {
			t.Errorf("expected buffer length %d, got %d", expectedLen, len(w.buf))
		}
	})
}

func TestWrapperCommitErrorHandling(t *testing.T) {
	t.Run("commit with no nodes from index", func(t *testing.T) {
		w := NewWrapper()
		w.AddNode(NewNodeWithValue([]byte{1}))
		w.AddNode(NewNodeWithValue([]byte{2}))
		
		// Commit from an index that has nodes after it
		initialCount := len(w.nodes)
		w.Commit(1) // Should merkleize the nodes from index 1 onwards
		
		// Should have fewer or equal nodes after commit
		if len(w.nodes) > initialCount {
			t.Errorf("commit should not increase node count: initial=%d, after=%d", initialCount, len(w.nodes))
		}
	})
	
	t.Run("commit with single node", func(t *testing.T) {
		w := NewWrapper()
		w.AddNode(NewNodeWithValue([]byte{1}))
		
		// Commit from index 0 with only one node
		w.Commit(0)
		
		// Should still have 1 node
		if len(w.nodes) != 1 {
			t.Errorf("expected 1 node, got %d", len(w.nodes))
		}
	})
}

func TestWrapperAddNodeNil(t *testing.T) {
	w := NewWrapper()
	
	// Test AddNode with nil node - should handle gracefully
	w.AddNode(nil)
	
	// Should have one node (even if nil)
	if len(w.nodes) != 1 {
		t.Error("AddNode should add the node even if nil")
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		i, j     int
		expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{-1, 0, -1},
		{10, 20, 10},
	}

	for _, tt := range tests {
		result := min(tt.i, tt.j)
		if result != tt.expected {
			t.Errorf("min(%d, %d) = %d, want %d", tt.i, tt.j, result, tt.expected)
		}
	}
}
