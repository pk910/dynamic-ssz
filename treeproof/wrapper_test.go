// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// The wrapper keeps its bytes the way hasher.Hasher does and cuts them into
// leaves when a region is reduced, so what used to be "the node list" is now
// the chunks the buffer holds. These helpers state the old assertions against
// that state.

// nodeCount reports how many chunks the walker holds.
func nodeCount(w *Wrapper) int { return (len(w.buf) + 31) / 32 }

// nodeAt returns what sits in chunk i: the subtree root that begins there, or
// a leaf of the chunk's bytes.
func nodeAt(w *Wrapper, i int) *Node {
	off := i * 32
	if j := w.nodeIndexAt(off); j >= 0 {
		return w.nodes[j].node
	}
	w.fillRegionUpTo32(0)
	w.materialize(off, off+32)
	return LeafFromBytes(w.buf[off : off+32])
}

// reduceBinaryAt reduces the region that began at off with no limit, which is
// what the removed commit helper did.
func (w *Wrapper) reduceBinaryAt(off int) { w.reduceBinary(off) }

// reduceProgressiveWithActiveFields reduces the region progressively and mixes
// in an active-fields bitvector.
func (w *Wrapper) reduceProgressiveWithActiveFields(off int, activeFields []byte) {
	w.MerkleizeProgressiveWithActiveFields(off, activeFields)
}

func TestNewWrapper(t *testing.T) {
	w := NewWrapper()

	if w == nil {
		t.Fatal("wrapper should not be nil")
	}
	if nodeCount(w) != 0 {
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

	// A scope opens where the buffer stands, and every node occupies a chunk,
	// so both indices count bytes.
	w.AddNode(NewNodeWithValue([]byte{1}))
	if w.Index() != 32 {
		t.Errorf("index after adding node should be 32, got %d", w.Index())
	}

	w.AddNode(NewNodeWithValue([]byte{2}))
	if w.Index() != 64 {
		t.Errorf("index after adding second node should be 64, got %d", w.Index())
	}

	if w.CurrentIndex() != 64 {
		t.Errorf("current index should be 64 after two nodes, got %d", w.CurrentIndex())
	}
	w.Append([]byte{1, 2, 3})
	if w.CurrentIndex() != 67 {
		t.Errorf("current index should track buffered bytes, got %d", w.CurrentIndex())
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

		expected := make([]byte, 0, 32)
		expected = append(expected, data...)
		expected = append(expected, make([]byte, 16)...)
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

	// The region is replaced by the one chunk its root occupies.
	if len(w.buf) != 32 {
		t.Errorf("buffer holds %d bytes after Merkleize, want one chunk", len(w.buf))
	}

	// Should have created a tree node
	if nodeCount(w) != 1 {
		t.Errorf("expected 1 node, got %d", nodeCount(w))
	}

	// Node should represent the merkleized data
	if nodeAt(w, 0) == nil {
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
	if nodeCount(w) != 1 {
		t.Errorf("expected 1 node, got %d", nodeCount(w))
	}

	// The root should have a mixin (right child with length)
	root := nodeAt(w, 0)
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
	if nodeCount(w) != 1 {
		t.Errorf("expected 1 node, got %d", nodeCount(w))
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
	if nodeCount(w) != 1 {
		t.Errorf("expected 1 node, got %d", nodeCount(w))
	}

	// The root should be a branch node (has mixin)
	if nodeAt(w, 0).IsLeaf() {
		t.Error("root should be a branch node with mixin")
	}

	// The region is replaced by the one chunk its root occupies.
	if len(w.buf) != 32 {
		t.Errorf("buffer holds %d bytes after MerkleizeProgressiveWithMixin, want one chunk", len(w.buf))
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
	if nodeCount(w) != 1 {
		t.Errorf("expected 1 node, got %d", nodeCount(w))
	}

	// The root should be a branch node (has active fields mixin)
	if nodeAt(w, 0).IsLeaf() {
		t.Error("root should be a branch node with active fields mixin")
	}

	// The region is replaced by the one chunk its root occupies.
	if len(w.buf) != 32 {
		t.Errorf("buffer holds %d bytes after MerkleizeProgressiveWithActiveFields, want one chunk", len(w.buf))
	}
}

func TestWrapperPutBitlist(t *testing.T) {
	w := NewWrapper()

	// Create a bitlist: 11110000 10000000 (14 bits, last byte indicates length)
	bitlist := []byte{0b11110000, 0b10000001}
	maxSize := uint64(16)

	w.PutBitlist(bitlist, maxSize)

	// Should have created a tree with mixin
	if nodeCount(w) != 1 {
		t.Errorf("expected 1 node, got %d", nodeCount(w))
	}
}

func TestWrapperPutProgressiveBitlist(t *testing.T) {
	w := NewWrapper()

	// Create a bitlist
	bitlist := []byte{0b11111111, 0b11111111, 0b10000001} // 17 bits

	w.PutProgressiveBitlist(bitlist)

	// Should have created a progressive tree with mixin
	if nodeCount(w) != 1 {
		t.Errorf("expected 1 node, got %d", nodeCount(w))
	}
}

// A field write buffers its chunk, as hasher.Hasher does; the leaf appears
// when the buffer is flushed, at the next scope boundary or node addition.
func TestWrapperPutMethods(t *testing.T) {
	t.Run("PutBool", func(t *testing.T) {
		w := NewWrapper()
		w.PutBool(true)

		if nodeCount(w) != 1 {
			t.Error("PutBool should add one node")
		}

		expected := make([]byte, 32)
		expected[0] = 1
		if !bytes.Equal(nodeAt(w, 0).value[:], expected) {
			t.Error("PutBool value mismatch")
		}
	})

	t.Run("PutBytes", func(t *testing.T) {
		// Test small bytes (<= 32)
		w := NewWrapper()
		smallBytes := []byte{1, 2, 3, 4}
		w.PutBytes(smallBytes)

		if nodeCount(w) != 1 {
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

	for _, tc := range []struct {
		name string
		put  func(w *Wrapper)
	}{
		{"PutUint64", func(w *Wrapper) { w.PutUint64(12345) }},
		{"PutUint32", func(w *Wrapper) { w.PutUint32(12345) }},
		{"PutUint16", func(w *Wrapper) { w.PutUint16(12345) }},
		{"PutUint8", func(w *Wrapper) { w.PutUint8(123) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWrapper()
			tc.put(w)

			if nodeCount(w) != 1 {
				t.Errorf("%s should add one node", tc.name)
			}
		})
	}
}

func TestWrapperPutUint64Array(t *testing.T) {
	t.Run("fixed size array", func(t *testing.T) {
		w := NewWrapper()
		arr := []uint64{1, 2, 3, 4}
		w.PutUint64Array(arr)

		if nodeCount(w) != 1 {
			t.Error("PutUint64Array should create one merkleized node")
		}
	})

	t.Run("dynamic array with max capacity", func(t *testing.T) {
		w := NewWrapper()
		arr := []uint64{1, 2, 3, 4, 5}
		maxCap := uint64(10)
		w.PutUint64Array(arr, maxCap)

		if nodeCount(w) != 1 {
			t.Error("PutUint64Array with max capacity should create one merkleized node")
		}

		// Should be a tree with mixin
		if nodeAt(w, 0).IsLeaf() {
			t.Error("dynamic array should create branch node with mixin")
		}
	})
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
		if !bytes.Equal(nodeAt(w, 0).value[:], expected) {
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
			if got := nodeAt(w, 0).value[:]; !bytes.Equal(got, want) {
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

		if nodeCount(w) != 1 || w.nodes[0].node != node {
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
	expectedHash := nodeAt(w, nodeCount(w)-1).Hash()
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

// A top-level packed value (e.g. a uint256/uint128 via TypeWrapper) is appended
// to the buffer and never merkleized into a container, so the walk ends with
// buffered bytes and no nodes. Node()/HashRoot() must flush that buffer into a
// single leaf instead of panicking or reporting an empty tree.
func TestWrapperNodeFlushesTopLevelBuffer(t *testing.T) {
	var v [32]byte
	v[0], v[31] = 0x11, 0x22

	w := NewWrapper()
	w.AppendBytes32(v[:])
	node := w.Node()
	if !bytes.Equal(node.Value(), v[:]) {
		t.Fatalf("node value = %x; want %x", node.Value(), v[:])
	}

	w2 := NewWrapper()
	w2.AppendBytes32(v[:])
	root, err := w2.HashRoot()
	if err != nil {
		t.Fatalf("HashRoot error: %v", err)
	}
	if !bytes.Equal(root[:], v[:]) {
		t.Fatalf("root = %x; want %x", root, v[:])
	}
}

// Hash() is called for verbose logging between subvalues, when the first packed
// element sits in the buffer and no nodes exist yet. It must peek that buffer
// (and tolerate a wholly empty wrapper) rather than indexing nodes[-1].
func TestWrapperHashToleratesUnflushedBuffer(t *testing.T) {
	var v [32]byte
	v[0] = 0x33

	w := NewWrapper()
	w.AppendBytes32(v[:]) // buffer holds 32 bytes, node list still empty
	got := w.Hash()
	if !bytes.Equal(got, v[:]) {
		t.Fatalf("Hash() = %x; want %x", got, v[:])
	}

	// With more than 32 buffered bytes, Hash() peeks the most recent chunk.
	var v2 [32]byte
	v2[0] = 0x44
	w.AppendBytes32(v2[:])
	got = w.Hash()
	if !bytes.Equal(got, v2[:]) {
		t.Fatalf("Hash() after second chunk = %x; want %x", got, v2[:])
	}

	// An empty walker has nothing to report, as hasher.Hasher has not.
	empty := NewWrapper()
	if z := empty.Hash(); len(z) != 0 {
		t.Fatalf("empty Hash() = %x; want no bytes", z)
	}
	if z := hasher.NewHasher().Hash(); len(z) != 0 {
		t.Fatalf("empty hasher Hash() = %x; want no bytes", z)
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
	w.reduceBinaryAt(0)

	// Should have one merkleized node
	if nodeCount(w) != 1 {
		t.Errorf("expected 1 node after commit, got %d", nodeCount(w))
	}

	// Test partial commit
	w2 := NewWrapper()
	w2.AddNode(NewNodeWithValue([]byte{1}))
	w2.AddNode(NewNodeWithValue([]byte{2}))
	w2.AddNode(NewNodeWithValue([]byte{3}))
	w2.AddNode(NewNodeWithValue([]byte{4}))

	// Reduce only the last 2 nodes, which begin at the third chunk.
	w2.reduceBinaryAt(64)

	// Should have 3 chunks: first 2 original + 1 merkleized
	if nodeCount(w2) != 3 {
		t.Errorf("expected 3 chunks after partial commit, got %d", nodeCount(w2))
	}
}

func TestWrapperCommitWithMixin(t *testing.T) {
	w := NewWrapper()

	// Add array elements
	for i := 0; i < 4; i++ {
		w.AddNode(LeafFromUint64(uint64(i)))
	}

	w.reduceBinaryWithMixin(0, 4, 4)

	// Should have one node with mixin
	if nodeCount(w) != 1 {
		t.Error("CommitWithMixin should produce one node")
	}

	// Root should be a branch (has mixin)
	if nodeAt(w, 0).IsLeaf() {
		t.Error("node should have mixin")
	}
}

func TestWrapperCommitProgressive(t *testing.T) {
	w := NewWrapper()

	// Add nodes for progressive merkleization
	for i := 0; i < 7; i++ {
		w.AddNode(LeafFromUint64(uint64(i)))
	}

	w.reduceProgressive(0)

	// Should have one progressive tree node
	if nodeCount(w) != 1 {
		t.Error("CommitProgressive should produce one node")
	}
}

func TestWrapperCommitProgressiveWithMixin(t *testing.T) {
	w := NewWrapper()

	// Add array elements
	for i := 0; i < 5; i++ {
		w.AddNode(LeafFromUint64(uint64(i)))
	}

	w.reduceProgressiveWithMixin(0, 5)

	// Should have one node with mixin
	if nodeCount(w) != 1 {
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
	w.reduceProgressiveWithActiveFields(0, activeFields)

	// Should have one node with active fields mixin
	if nodeCount(w) != 1 {
		t.Error("CommitProgressiveWithActiveFields should produce one node")
	}
}

// Bytes become leaves when a region is reduced: the region is padded to whole
// chunks counted from its start, and each chunk is one leaf. An empty value
// contributes nothing, matching hasher.Hasher.
// The terminal methods report what is missing: an open scope, or a buffer that
// is not one chunk. Hash reports the last chunk of a longer buffer, and
// Collapse is a hint this walker has no use for.
func TestWrapperTerminalStates(t *testing.T) {
	w := NewWrapper()
	idx := w.StartTree(sszutils.TreeTypeBinary)
	w.PutUint64(1)
	w.Collapse()
	if _, err := w.Root(); err == nil || !strings.Contains(err.Error(), "unfinished hashing scopes") {
		t.Fatalf("Root with an open scope: err = %v", err)
	}
	w.Merkleize(idx)

	w.PutUint64(2)
	if _, err := w.HashRoot(); err == nil || !strings.Contains(err.Error(), "want 32") {
		t.Fatalf("HashRoot with two chunks: err = %v", err)
	}
	// Hash reports the last chunk, not the whole buffer.
	tail := w.Hash()
	if len(tail) != 32 || !bytes.Equal(tail, w.buf[32:]) {
		t.Fatalf("Hash() = %x, want the last chunk %x", tail, w.buf[32:])
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
	if root.left == nil || root.right == nil {
		t.Fatalf("root is not a two-leaf tree: %v", root)
	}
	want := LeafFromUint64(0xdeadbeef)
	if !bytes.Equal(root.right.Hash(), want.Hash()) {
		t.Fatalf("second leaf = %x, want the added node %x", root.right.Hash(), want.Hash())
	}
}

func TestWrapperRegionLeaves(t *testing.T) {
	t.Run("empty region", func(t *testing.T) {
		w := NewWrapper()
		if leaves := w.regionLeaves(0); len(leaves) != 0 {
			t.Errorf("empty region gave %d leaves, want none", len(leaves))
		}
	})

	t.Run("exact 32 bytes", func(t *testing.T) {
		w := NewWrapper()
		data := bytes.Repeat([]byte{0xAB}, 32)
		w.Append(data)

		leaves := w.regionLeaves(0)
		if len(leaves) != 1 {
			t.Fatalf("32 bytes gave %d leaves, want 1", len(leaves))
		}
		if !bytes.Equal(leaves[0].value[:], data) {
			t.Error("leaf value mismatch")
		}
	})

	t.Run("non-32 byte aligned", func(t *testing.T) {
		w := NewWrapper()
		w.Append(bytes.Repeat([]byte{0xFF}, 50))

		if leaves := w.regionLeaves(0); len(leaves) != 2 {
			t.Errorf("50 bytes gave %d leaves, want 2 (padded to 64)", len(leaves))
		}
	})

	t.Run("region that began off a chunk boundary", func(t *testing.T) {
		w := NewWrapper()
		w.Append(bytes.Repeat([]byte{0x01}, 5))
		w.Append(bytes.Repeat([]byte{0xFF}, 50))

		// Chunks are counted from the region start, not from the buffer start.
		if leaves := w.regionLeaves(5); len(leaves) != 2 {
			t.Errorf("50 bytes at offset 5 gave %d leaves, want 2", len(leaves))
		}
	})
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
		if nodeCount(w) != 2 {
			t.Errorf("expected 2 nodes, got %d", nodeCount(w))
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
		_, err := TreeFromNodes([]*Node{nodeAt(w, 0)}, 3) // 3 is not a power of 2, should error
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
		initialCount := nodeCount(w)
		w.reduceBinaryAt(1) // Should merkleize the nodes from index 1 onwards

		// Should have fewer or equal nodes after commit
		if nodeCount(w) > initialCount {
			t.Errorf("commit should not increase node count: initial=%d, after=%d", initialCount, nodeCount(w))
		}
	})

	t.Run("commit with single node", func(t *testing.T) {
		w := NewWrapper()
		w.AddNode(NewNodeWithValue([]byte{1}))

		// Commit from index 0 with only one node
		w.reduceBinaryAt(0)

		// Should still have 1 node
		if nodeCount(w) != 1 {
			t.Errorf("expected 1 node, got %d", nodeCount(w))
		}
	})
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

// Wrapper and Hasher must agree for empty PutBytes/PutProgressiveBitlist input:
// a phantom zero leaf would shift siblings and give a spec-incorrect root.
func TestWrapperEmptyInputMatchesHasher(t *testing.T) {
	// PutBytes([]) followed by a sibling.
	w := NewWrapper()
	idx := w.StartTree(sszutils.TreeTypeNone)
	w.PutBytes([]byte{})
	w.PutUint64(42)
	w.Merkleize(idx)
	wRoot := w.Hash()

	hh := hasher.NewHasher()
	hidx := hh.StartTree(sszutils.TreeTypeNone)
	hh.PutBytes([]byte{})
	hh.PutUint64(42)
	hh.Merkleize(hidx)
	hRoot, err := hh.HashRoot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wRoot, hRoot[:]) {
		t.Errorf("PutBytes empty: wrapper=%x hasher=%x", wRoot[:8], hRoot[:8])
	}

	// Empty progressive bitlist.
	w2 := NewWrapper()
	w2.PutProgressiveBitlist([]byte{0x01})
	hh2 := hasher.NewHasher()
	hh2.PutProgressiveBitlist([]byte{0x01})
	h2Root, err := hh2.HashRoot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(w2.Hash(), h2Root[:]) {
		t.Errorf("empty progressive bitlist: wrapper=%x hasher=%x", w2.Hash()[:8], h2Root[:8])
	}
}

// Wrapper.PutBitlist with maxSize 0 must behave like Hasher.PutBitlist (a
// root, not a panic) for a degenerate but legal input.
func TestWrapperPutBitlistZeroMaxMatchesHasher(t *testing.T) {
	var wRoot []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Wrapper.PutBitlist(_, 0) panicked: %v", r)
			}
		}()
		w := NewWrapper()
		w.PutBitlist([]byte{0x01}, 0)
		wRoot = w.Hash()
	}()

	hh := hasher.NewHasher()
	hh.PutBitlist([]byte{0x01}, 0)
	hRoot, err := hh.HashRoot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wRoot, hRoot[:]) {
		t.Errorf("PutBitlist zero max: wrapper=%x hasher=%x", wRoot[:8], hRoot[:8])
	}
}

// Wrapper.MerkleizeWithMixin with a limit below the chunk count clamps the
// limit up to the count like Hasher does, so the two produce the same bytes,
// and both report the reduction that was handed more chunks than its limit
// holds rather than panicking or answering with a root.
func TestWrapperMixinLimitBelowChunksMatchesHasher(t *testing.T) {
	var wRoot []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Wrapper.MerkleizeWithMixin(limit < count) panicked: %v", r)
			}
		}()
		w := NewWrapper()
		idx := w.StartTree(sszutils.TreeTypeBinary)
		for i := range 8 {
			w.PutUint64(uint64(i))
		}
		w.MerkleizeWithMixin(idx, 8, 2)
		wRoot = w.Hash()
	}()

	hh := hasher.NewHasher()
	hidx := hh.StartTree(sszutils.TreeTypeBinary)
	for i := range 8 {
		hh.PutUint64(uint64(i))
	}
	hh.MerkleizeWithMixin(hidx, 8, 2)
	hRoot := hh.Hash()
	if !bytes.Equal(wRoot, hRoot) {
		t.Errorf("mixin limit below chunks: wrapper=%x hasher=%x", wRoot[:8], hRoot[:8])
	}
	if _, err := hh.HashRoot(); !errors.Is(err, sszutils.ErrChunkLimitExceeded) {
		t.Errorf("hasher root: err = %v, want the chunk limit reported", err)
	}

	// The same sequence through the public convenience API.
	var w2Root []byte
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Wrapper.PutUint64Array(cap < len) panicked: %v", r)
			}
		}()
		w2 := NewWrapper()
		w2.PutUint64Array(make([]uint64, 100), 8)
		w2Root = w2.Hash()
	}()

	hh2 := hasher.NewHasher()
	hh2.PutUint64Array(make([]uint64, 100), 8)
	h2Root := hh2.Hash()
	if !bytes.Equal(w2Root, h2Root) {
		t.Errorf("PutUint64Array cap overflow: wrapper=%x hasher=%x", w2Root[:8], h2Root[:8])
	}
	if _, err := hh2.HashRoot(); !errors.Is(err, sszutils.ErrChunkLimitExceeded) {
		t.Errorf("hasher root: err = %v, want the chunk limit reported", err)
	}

	// Both engines collapse a scope every 256 elements, which reduces it by a
	// different path. A hint is optional, so it cannot decide whether the walk
	// is refused: the two walkers answer alike on either side of that size.
	collapsed := func(w sszutils.HashWalker, n int) ([]byte, error) {
		idx := w.StartTree(sszutils.TreeTypeBinary)
		for i := range n {
			w.AppendBytes32([]byte{byte(i), byte(i >> 8)})
			if (i+1)%256 == 0 {
				w.Collapse()
			}
		}
		w.MerkleizeWithMixin(idx, uint64(n), 1)

		return w.Hash(), w.HashErr()
	}
	for _, n := range []int{255, 256, 257, 600} {
		wRoot, wErr := collapsed(NewWrapper(), n)
		hRoot, hErr := collapsed(hasher.NewHasher(), n)
		if !errors.Is(wErr, sszutils.ErrChunkLimitExceeded) || !errors.Is(hErr, sszutils.ErrChunkLimitExceeded) {
			t.Errorf("%d chunks against a limit of 1: wrapper err = %v, hasher err = %v", n, wErr, hErr)
		}
		if !bytes.Equal(wRoot, hRoot) {
			t.Errorf("%d chunks collapsed: wrapper=%x hasher=%x", n, wRoot[:8], hRoot[:8])
		}
	}
}

// AppendBytes32 on a buffer that is not chunk-aligned must pad identically in
// both walkers (whole-buffer alignment) so they produce the same root.
func TestWrapperAppendBytes32UnalignedMatchesHasher(t *testing.T) {
	w := NewWrapper()
	idx := w.StartTree(sszutils.TreeTypeNone)
	w.Append([]byte{1, 2, 3})
	w.AppendBytes32([]byte{4, 5})
	w.Merkleize(idx)
	wRoot := w.Hash()

	hh := hasher.NewHasher()
	hidx := hh.StartTree(sszutils.TreeTypeNone)
	hh.Append([]byte{1, 2, 3})
	hh.AppendBytes32([]byte{4, 5})
	hh.Merkleize(hidx)
	hRoot, err := hh.HashRoot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wRoot, hRoot[:]) {
		t.Errorf("AppendBytes32 unaligned: wrapper=%x hasher=%x", wRoot[:8], hRoot[:8])
	}
}

// Wrapper.Merkleize* must clamp an out-of-range scope index like Hasher does,
// instead of panicking with a raw slice-bounds error.
func TestWrapperMerkleizeIndexClamped(t *testing.T) {
	cases := []func(w *Wrapper){
		func(w *Wrapper) { w.Merkleize(999) },
		func(w *Wrapper) { w.Merkleize(-1) },
		func(w *Wrapper) { w.MerkleizeWithMixin(-1, 1, 4) },
		func(w *Wrapper) { w.MerkleizeProgressive(999) },
		func(w *Wrapper) { w.MerkleizeProgressiveWithMixin(-1, 1) },
	}
	for i, fn := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("case %d panicked on out-of-range index: %v", i, r)
				}
			}()
			w := NewWrapper()
			w.AppendUint64(1)
			w.FillUpTo32()
			fn(w)
		}()
	}
}

// --- moved from errpath_test.go ---
// expectPanicWithError recovers from a panic and checks that the recovered
// value matches the expected error.
func expectPanicWithError(t *testing.T, expected error, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("expected error panic, got: %T %v", r, r)
		}
		if !errors.Is(err, expected) {
			t.Fatalf("expected panic with %q, got: %q", expected, err)
		}
	}()
	fn()
}

// --- Wrapper.Collapse: no-op coverage ---

func TestWrapperCollapseNoop(t *testing.T) {
	w := NewWrapper()
	w.AddNode(LeafFromUint64(1))
	w.Collapse()
	if nodeCount(w) != 1 {
		t.Fatal("Collapse should not modify the wrapper")
	}
}

// --- Wrapper.Commit: panic on tree build error ---

func TestWrapperCommitPanicInjected(t *testing.T) {
	injected := errors.New("injected commit error")
	treeFromNodesToDepthFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesToDepthFn = treeFromNodesToDepth }()

	expectPanicWithError(t, injected, func() {
		w := NewWrapper()
		w.AddNode(LeafFromUint64(1))
		w.reduceBinaryAt(0)
	})
}

// --- Wrapper.CommitWithMixin: panic on tree build error ---

func TestWrapperCommitWithMixinPanicInjected(t *testing.T) {
	injected := errors.New("injected commit error")
	treeFromNodesToDepthFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesToDepthFn = treeFromNodesToDepth }()

	expectPanicWithError(t, injected, func() {
		w := NewWrapper()
		w.AddNode(LeafFromUint64(1))
		w.reduceBinaryWithMixin(0, 1, 1)
	})
}

// --- Wrapper.CommitProgressive: panic on tree build error ---

func TestWrapperCommitProgressivePanicInjected(t *testing.T) {
	injected := errors.New("injected commit error")
	treeFromNodesToDepthFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesToDepthFn = treeFromNodesToDepth }()

	expectPanicWithError(t, injected, func() {
		w := NewWrapper()
		w.AddNode(LeafFromUint64(1))
		w.reduceProgressive(0)
	})
}

// --- Wrapper.CommitProgressiveWithMixin: panic on tree build error ---

func TestWrapperCommitProgressiveWithMixinPanicInjected(t *testing.T) {
	injected := errors.New("injected commit error")
	treeFromNodesToDepthFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesToDepthFn = treeFromNodesToDepth }()

	expectPanicWithError(t, injected, func() {
		w := NewWrapper()
		w.AddNode(LeafFromUint64(1))
		w.reduceProgressiveWithMixin(0, 1)
	})
}

// --- Wrapper.CommitProgressiveWithActiveFields: panic on tree build error ---

func TestWrapperCommitProgressiveWithActiveFieldsPanicInjected(t *testing.T) {
	injected := errors.New("injected commit error")
	treeFromNodesToDepthFn = func([]*Node, int) (*Node, error) {
		return nil, injected
	}
	defer func() { treeFromNodesToDepthFn = treeFromNodesToDepth }()

	expectPanicWithError(t, injected, func() {
		w := NewWrapper()
		w.AddNode(LeafFromUint64(1))
		w.reduceProgressiveWithActiveFields(0, []byte{0x01})
	})
}

// --- moved from wrapper_overflow_test.go ---

// A list capacity (limit) and element count (num) are uint64. A capacity only
// determines the merkle tree depth (at most 64), so values above the platform
// int range — e.g. the practically-unbounded ssz-max:"18446744073709551615" —
// must produce a valid root rather than crash, matching the streaming hasher.

func TestMerkleizeWithMixinHugeNum(t *testing.T) {
	w := NewWrapper()
	w.AppendUint64(1)
	w.MerkleizeWithMixin(0, math.MaxUint64, 1)
	if _, err := w.HashRoot(); err != nil {
		t.Fatalf("HashRoot: %v", err)
	}
}

func TestMerkleizeWithMixinHugeLimit(t *testing.T) {
	w := NewWrapper()
	w.AppendUint64(1)
	w.MerkleizeWithMixin(0, 1, math.MaxUint64)
	if _, err := w.HashRoot(); err != nil {
		t.Fatalf("HashRoot: %v", err)
	}
}

func TestMerkleizeProgressiveWithMixinHugeNum(t *testing.T) {
	w := NewWrapper()
	w.AppendUint64(1)
	w.MerkleizeProgressiveWithMixin(0, math.MaxUint64)
	if _, err := w.HashRoot(); err != nil {
		t.Fatalf("HashRoot: %v", err)
	}
}

func TestPutBitlistNormal(t *testing.T) {
	w := NewWrapper()
	w.PutBitlist([]byte{0x07}, 256) // 2 bits set, sentinel at bit 2
}

func TestPutBitlistHugeMaxSize(t *testing.T) {
	// A degenerate ssz-max can push the derived chunk limit above the platform
	// int range; it only affects the padding depth and must not panic.
	w := NewWrapper()
	w.PutBitlist([]byte{0x01}, math.MaxUint64) // sentinel only, size=0
	if _, err := w.HashRoot(); err != nil {
		t.Fatalf("HashRoot: %v", err)
	}
}

func TestPutProgressiveBitlistNormal(t *testing.T) {
	w := NewWrapper()
	w.PutProgressiveBitlist([]byte{0x07}) // 2 bits, sentinel at bit 2
}

// --- moved from wrapper_interleave_test.go ---

// TestWrapperInterleavedAppendPut is a regression test for
// https://github.com/pk910/dynamic-ssz/issues/191: the Wrapper must produce
// the same root as hasher.Hasher when a container interleaves Append*-buffered
// fields with Put* fields or child scopes. The Wrapper used to flush buffered
// bytes only at Merkleize, reordering leaves relative to directly-added nodes.
func TestWrapperInterleavedAppendPut(t *testing.T) {
	fieldA := [32]byte{0x11}
	fieldB := [32]byte{0x22}

	tests := []struct {
		name string
		walk func(hh sszutils.HashWalker)
	}{
		{
			// e.g. a uint256 root emitted via AppendBytes32 followed by a
			// bytes32 field emitted via PutBytes (deneb.ExecutionPayloadHeader
			// BaseFeePerGas -> BlockHash layout)
			name: "append then put",
			walk: func(hh sszutils.HashWalker) {
				idx := hh.StartTree(sszutils.TreeTypeNone)
				hh.AppendBytes32(fieldA[:])
				hh.PutBytes(fieldB[:])
				hh.Merkleize(idx)
			},
		},
		{
			name: "append then put uint64",
			walk: func(hh sszutils.HashWalker) {
				idx := hh.StartTree(sszutils.TreeTypeNone)
				hh.AppendBytes32(fieldA[:])
				hh.PutUint64(42)
				hh.Merkleize(idx)
			},
		},
		{
			// an Append*-buffered field followed by a child scope: the buffered
			// bytes must become leaves of the parent, not of the child list
			name: "append then child scope",
			walk: func(hh sszutils.HashWalker) {
				idx := hh.StartTree(sszutils.TreeTypeNone)
				hh.AppendBytes32(fieldA[:])
				cidx := hh.StartTree(sszutils.TreeTypeNone)
				hh.AppendUint64(42)
				hh.AppendUint64(43)
				hh.FillUpTo32()
				hh.MerkleizeWithMixin(cidx, 2, 4)
				hh.Merkleize(idx)
			},
		},
		{
			// deneb.ExecutionPayloadHeader-like tail: dynamic list scope,
			// buffered uint256 root, then direct bytes32/uint64 fields
			name: "payload header field tail",
			walk: func(hh sszutils.HashWalker) {
				idx := hh.StartTree(sszutils.TreeTypeNone)
				hh.PutBytes(fieldA[:])                      // PrevRandao
				hh.PutUint64(1)                             // BlockNumber
				hh.PutUint64(2)                             // GasLimit
				cidx := hh.StartTree(sszutils.TreeTypeNone) // ExtraData
				hh.Append([]byte{0xff, 0xee})
				hh.FillUpTo32()
				hh.MerkleizeWithMixin(cidx, 2, 1)
				hh.AppendBytes32(fieldB[:]) // BaseFeePerGas root
				hh.PutBytes(fieldA[:])      // BlockHash
				hh.PutUint64(3)             // BlobGasUsed
				hh.Merkleize(idx)
			},
		},
		{
			name: "append then bitlist",
			walk: func(hh sszutils.HashWalker) {
				idx := hh.StartTree(sszutils.TreeTypeNone)
				hh.AppendBytes32(fieldA[:])
				hh.PutBitlist([]byte{0xff, 0x01}, 64)
				hh.Merkleize(idx)
			},
		},
		{
			name: "append then large put bytes",
			walk: func(hh sszutils.HashWalker) {
				idx := hh.StartTree(sszutils.TreeTypeNone)
				hh.AppendBytes32(fieldA[:])
				large := make([]byte, 96)
				large[0] = 0x33
				hh.PutBytes(large)
				hh.Merkleize(idx)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := hasher.FastHasherPool.Get()
			defer hasher.FastHasherPool.Put(h)

			tt.walk(h)
			expected, err := h.HashRoot()
			if err != nil {
				t.Fatalf("hasher HashRoot failed: %v", err)
			}

			w := NewWrapper()
			tt.walk(w)
			actual, err := w.HashRoot()
			if err != nil {
				t.Fatalf("wrapper HashRoot failed: %v", err)
			}

			if expected != actual {
				t.Errorf("wrapper root %x does not match hasher root %x", actual, expected)
			}
		})
	}
}

// Wrapper.PutBytes must not write zero padding into the caller's backing
// array (input memory is never mutated), and must produce the same root as
// Hasher.PutBytes, which copies before padding.
func TestWrapperPutBytesDoesNotMutateCallerMemory(t *testing.T) {
	backing := make([]byte, 64)
	for i := range backing {
		backing[i] = 0xEE
	}
	before := append([]byte(nil), backing...)

	w := NewWrapper()
	w.PutBytes(backing[:10:64])
	wRoot := w.Hash()

	if !bytes.Equal(backing, before) {
		t.Fatalf("Wrapper.PutBytes mutated caller memory:\n before: %x\n after:  %x", before, backing)
	}

	hh := hasher.NewHasher()
	hh.PutBytes(before[:10:64])
	hRoot, err := hh.HashRoot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wRoot, hRoot[:]) {
		t.Errorf("PutBytes short input: wrapper=%x hasher=%x", wRoot[:8], hRoot[:8])
	}

	// Multi-chunk unaligned input takes the appendBytesAsNodes padding path.
	w2 := NewWrapper()
	w2.PutBytes(backing[:40:64])
	if !bytes.Equal(backing, before) {
		t.Fatalf("Wrapper.PutBytes (multi-chunk) mutated caller memory")
	}

	// LeafFromBytes pads short inputs without touching the source array.
	leaf := LeafFromBytes(backing[:5:64])
	if !bytes.Equal(backing, before) {
		t.Fatalf("LeafFromBytes mutated caller memory")
	}
	if !bytes.Equal(leaf.Value()[:5], before[:5]) {
		t.Errorf("LeafFromBytes value mismatch")
	}
}

// TestWrapperHashRootRequiresCompleteMerkleization pins that HashRoot reports
// an incomplete merkleization instead of returning the last pending node's
// hash. The Wrapper is documented as a drop-in HashWalker producing the same
// root as hasher.Hasher for the same call sequence, and the Hasher errors on
// that state (its buffer holds more than one chunk); Node() panics on it. Only
// HashRoot silently handed back a wrong root with a nil error.
func TestWrapperHashRootRequiresCompleteMerkleization(t *testing.T) {
	t.Run("MultipleNodes", func(t *testing.T) {
		w := NewWrapper()
		w.AddUint64(1)
		w.AddUint64(2)

		root, err := w.HashRoot()
		if err == nil {
			t.Fatalf("HashRoot() = %x, want an incomplete-merkleization error", root)
		}
		if !strings.Contains(err.Error(), "wrapper holds 64 bytes, want 32") {
			t.Fatalf("unexpected error: %v", err)
		}

		// The Hasher rejects the equivalent state (more than one chunk left
		// un-merkleized), which is the parity this method is supposed to keep.
		h := hasher.NewHasher()
		for i := range 8 {
			h.AppendUint64(uint64(i))
		}
		if _, herr := h.HashRoot(); herr == nil {
			t.Fatal("hasher.Hasher accepted an incomplete merkleization; parity assumption no longer holds")
		}
	})

	t.Run("NoNodes", func(t *testing.T) {
		w := NewWrapper()
		if _, err := w.HashRoot(); err == nil {
			t.Fatal("expected an error for an empty wrapper")
		}
	})

	t.Run("SingleNode", func(t *testing.T) {
		// A completed merkleization still returns its root.
		w := NewWrapper()
		idx := w.Index()
		w.AddUint64(1)
		w.AddUint64(2)
		w.Merkleize(idx)

		root, err := w.HashRoot()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := hashPair(LeafFromUint64(1).Hash(), LeafFromUint64(2).Hash())
		if !bytes.Equal(root[:], want) {
			t.Fatalf("root = %x, want %x", root, want)
		}
	})
}

// A packed scope on the wrapper buffers the Put* forms like the Append* forms;
// a scope opened inside a packed one, and any scope after it, add whole
// leaves again.
// The convenience methods that merkleize a subtree of their own leave the
// scope stack as they found it, so a long sequence of them inside one scope
// keeps that scope's packing state intact.
func TestWrapperConvenienceMethodsKeepScopeStack(t *testing.T) {
	for name, put := range map[string]func(*Wrapper){
		"bitlist":             func(w *Wrapper) { w.PutBitlist([]byte{1}, 8) },
		"progressive-bitlist": func(w *Wrapper) { w.PutProgressiveBitlist([]byte{1}) },
		"long-bytes":          func(w *Wrapper) { w.PutBytes(make([]byte, 64)) },
	} {
		t.Run(name, func(t *testing.T) {
			w := NewWrapper()
			idx := w.StartTree(sszutils.TreeTypeBinary | sszutils.TreeTypePacked)
			for range 100 {
				put(w)
			}
			if got := len(w.scopes); got != 1 {
				t.Fatalf("scope stack holds %d entries inside one open scope, want 1", got)
			}
			if !w.inPackedScope() {
				t.Fatal("the open packed scope lost its packing state")
			}
			w.Merkleize(idx)
			if got := len(w.scopes); got != 0 {
				t.Fatalf("scope stack holds %d entries after the scope closed, want 0", got)
			}
		})
	}
}

func TestWrapperPackedScope(t *testing.T) {
	w := NewWrapper()
	idx := w.StartTree(sszutils.TreeTypeBinary | sszutils.TreeTypePacked)
	w.PutUint64(1)
	w.PutUint32(2)
	w.PutUint16(3)
	w.PutUint8(4)
	w.PutBool(true)
	w.PutBytes([]byte{5, 6})
	if got := w.CurrentIndex(); got != 8+4+2+1+1+2 {
		t.Fatalf("packed scope buffered %d bytes, want 18", got)
	}
	w.FillUpTo32()
	w.MerkleizeWithMixin(idx, 6, 4)

	h := hasher.NewHasher()
	defer h.Reset()
	hidx := h.StartTree(sszutils.TreeTypeBinary)
	h.AppendUint64(1)
	h.AppendUint32(2)
	h.AppendUint16(3)
	h.AppendUint8(4)
	h.AppendBool(true)
	h.Append([]byte{5, 6})
	h.FillUpTo32()
	h.MerkleizeWithMixin(hidx, 6, 4)
	if !bytes.Equal(w.Hash(), h.Hash()) {
		t.Fatalf("wrapper packed root %x != hasher root %x", w.Hash(), h.Hash())
	}

	w = NewWrapper()
	outer := w.StartTree(sszutils.TreeTypeBinary | sszutils.TreeTypePacked)
	inner := w.StartTree(sszutils.TreeTypeNone)
	w.PutUint64(1)
	// A scope inside a packed scope is not packed: the value takes a whole
	// chunk, which becomes its leaf when the scope closes.
	if nodeCount(w) != 1 || len(w.buf) != 32 {
		t.Fatalf("scope inside a packed scope holds %d chunks and %d bytes, want one chunk", nodeCount(w), len(w.buf))
	}
	w.Merkleize(inner)
	if nodeCount(w) != 1 || len(w.buf) != 32 {
		t.Fatalf("closing the inner scope left %d chunks and %d bytes, want one chunk", nodeCount(w), len(w.buf))
	}
	w.PutUint64(2)
	if len(w.buf) != 40 {
		t.Fatalf("packed scope after a nested scope holds %d bytes, want the chunk plus 8", len(w.buf))
	}
	w.FillUpTo32()
	w.Merkleize(outer)
	w.PutUint64(3)
	// Outside a packed scope the value takes a whole chunk, not its 8 packed
	// bytes, so the closed scope no longer packs.
	if len(w.buf) != 64 {
		t.Fatalf("closed packed scope buffered %d bytes, want a whole chunk", len(w.buf))
	}
}
