// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pk910/dynamic-ssz/hasher"
	"github.com/pk910/dynamic-ssz/sszutils"
)

// Helper function to convert [32]byte to []byte
func sum256ToBytes(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

func TestVerifyProof(t *testing.T) {
	tests := []struct {
		name        string
		root        []byte
		proof       *Proof
		expectValid bool
		expectError bool
	}{
		{
			name: "valid single leaf proof",
			root: func() []byte {
				// Create a simple tree with 4 leaves
				leaf0 := sum256ToBytes([]byte("leaf0"))
				leaf1 := sum256ToBytes([]byte("leaf1"))
				leaf2 := sum256ToBytes([]byte("leaf2"))
				leaf3 := sum256ToBytes([]byte("leaf3"))

				// Build the tree
				node0 := sum256ToBytes(append(leaf0, leaf1...))
				node1 := sum256ToBytes(append(leaf2, leaf3...))
				root := sum256ToBytes(append(node0, node1...))
				return root
			}(),
			proof: &Proof{
				Index: 4, // Index of leaf0 in generalized index
				Leaf:  sum256ToBytes([]byte("leaf0")),
				Hashes: [][]byte{
					sum256ToBytes([]byte("leaf1")),
					func() []byte {
						leaf2 := sum256ToBytes([]byte("leaf2"))
						leaf3 := sum256ToBytes([]byte("leaf3"))
						return sum256ToBytes(append(leaf2, leaf3...))
					}(),
				},
			},
			expectValid: true,
			expectError: false,
		},
		{
			name: "invalid proof - wrong leaf",
			root: func() []byte {
				leaf0 := sum256ToBytes([]byte("leaf0"))
				leaf1 := sum256ToBytes([]byte("leaf1"))
				leaf2 := sum256ToBytes([]byte("leaf2"))
				leaf3 := sum256ToBytes([]byte("leaf3"))

				node0 := sum256ToBytes(append(leaf0, leaf1...))
				node1 := sum256ToBytes(append(leaf2, leaf3...))
				root := sum256ToBytes(append(node0, node1...))
				return root
			}(),
			proof: &Proof{
				Index: 4,
				Leaf:  sum256ToBytes([]byte("wrong_leaf")),
				Hashes: [][]byte{
					sum256ToBytes([]byte("leaf1")),
					func() []byte {
						leaf2 := sum256ToBytes([]byte("leaf2"))
						leaf3 := sum256ToBytes([]byte("leaf3"))
						return sum256ToBytes(append(leaf2, leaf3...))
					}(),
				},
			},
			expectValid: false,
			expectError: false,
		},
		{
			name: "invalid proof length",
			root: []byte{1, 2, 3},
			proof: &Proof{
				Index:  4, // requires 2 hashes
				Leaf:   make([]byte, 32),
				Hashes: [][]byte{{1, 2, 3}}, // only 1 hash provided
			},
			expectValid: false,
			expectError: true,
		},
		{
			name: "proof for rightmost leaf",
			root: func() []byte {
				leaf0 := sum256ToBytes([]byte("leaf0"))
				leaf1 := sum256ToBytes([]byte("leaf1"))
				leaf2 := sum256ToBytes([]byte("leaf2"))
				leaf3 := sum256ToBytes([]byte("leaf3"))

				node0 := sum256ToBytes(append(leaf0, leaf1...))
				node1 := sum256ToBytes(append(leaf2, leaf3...))
				root := sum256ToBytes(append(node0, node1...))
				return root
			}(),
			proof: &Proof{
				Index: 7, // Index of leaf3 in generalized index
				Leaf:  sum256ToBytes([]byte("leaf3")),
				Hashes: [][]byte{
					sum256ToBytes([]byte("leaf2")),
					func() []byte {
						leaf0 := sum256ToBytes([]byte("leaf0"))
						leaf1 := sum256ToBytes([]byte("leaf1"))
						return sum256ToBytes(append(leaf0, leaf1...))
					}(),
				},
			},
			expectValid: true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := VerifyProof(tt.root, tt.proof)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if valid != tt.expectValid {
				t.Errorf("expected valid=%v, got valid=%v", tt.expectValid, valid)
			}
		})
	}
}

func TestVerifyMultiproof(t *testing.T) {
	tests := []struct {
		name        string
		root        []byte
		proof       [][]byte
		leaves      [][]byte
		indices     []int
		expectValid bool
		expectError bool
	}{
		{
			name: "valid multiproof for two leaves",
			root: func() []byte {
				leaf0 := sum256ToBytes([]byte("leaf0"))
				leaf1 := sum256ToBytes([]byte("leaf1"))
				leaf2 := sum256ToBytes([]byte("leaf2"))
				leaf3 := sum256ToBytes([]byte("leaf3"))

				node0 := sum256ToBytes(append(leaf0, leaf1...))
				node1 := sum256ToBytes(append(leaf2, leaf3...))
				root := sum256ToBytes(append(node0, node1...))
				return root
			}(),
			leaves: [][]byte{
				sum256ToBytes([]byte("leaf0")),
				sum256ToBytes([]byte("leaf3")),
			},
			indices: []int{4, 7}, // generalized indices for leaf0 and leaf3
			proof: [][]byte{
				sum256ToBytes([]byte("leaf2")),
				sum256ToBytes([]byte("leaf1")),
			},
			expectValid: true,
			expectError: false,
		},
		{
			name: "proof hash count mismatch",
			root: func() []byte {
				leaf0 := sum256ToBytes([]byte("leaf0"))
				leaf1 := sum256ToBytes([]byte("leaf1"))
				leaf2 := sum256ToBytes([]byte("leaf2"))
				leaf3 := sum256ToBytes([]byte("leaf3"))

				node0 := sum256ToBytes(append(leaf0, leaf1...))
				node1 := sum256ToBytes(append(leaf2, leaf3...))
				root := sum256ToBytes(append(node0, node1...))
				return root
			}(),
			leaves: [][]byte{
				sum256ToBytes([]byte("leaf0")),
				sum256ToBytes([]byte("leaf3")),
			},
			indices: []int{4, 7},
			proof: [][]byte{
				sum256ToBytes([]byte("leaf2")),
				sum256ToBytes([]byte("leaf1")),
				sum256ToBytes([]byte("surplus")),
			},
			expectValid: false,
			expectError: true,
		},
		{
			name:        "empty indices",
			root:        []byte{1, 2, 3},
			proof:       [][]byte{},
			leaves:      [][]byte{},
			indices:     []int{},
			expectValid: false,
			expectError: true,
		},
		{
			name:        "mismatched leaves and indices",
			root:        []byte{1, 2, 3},
			proof:       [][]byte{},
			leaves:      [][]byte{{1}, {2}},
			indices:     []int{1},
			expectValid: false,
			expectError: true,
		},
		{
			name: "missing required proof nodes",
			root: []byte{1, 2, 3},
			leaves: [][]byte{
				{1, 2, 3},
				{4, 5, 6},
			},
			indices:     []int{4, 5},
			proof:       [][]byte{}, // Should have sibling hashes
			expectValid: false,
			expectError: true,
		},
		{
			name: "invalid multiproof - wrong leaf data",
			root: func() []byte {
				leaf0 := sum256ToBytes([]byte("leaf0"))
				leaf1 := sum256ToBytes([]byte("leaf1"))
				leaf2 := sum256ToBytes([]byte("leaf2"))
				leaf3 := sum256ToBytes([]byte("leaf3"))

				node0 := sum256ToBytes(append(leaf0, leaf1...))
				node1 := sum256ToBytes(append(leaf2, leaf3...))
				root := sum256ToBytes(append(node0, node1...))
				return root
			}(),
			leaves: [][]byte{
				sum256ToBytes([]byte("wrong_leaf")),
				sum256ToBytes([]byte("leaf3")),
			},
			indices: []int{4, 7},
			proof: [][]byte{
				sum256ToBytes([]byte("leaf2")),
				sum256ToBytes([]byte("leaf1")),
			},
			expectValid: false,
			expectError: false,
		},
		{
			name: "multiproof for all leaves",
			root: func() []byte {
				leaf0 := sum256ToBytes([]byte("leaf0"))
				leaf1 := sum256ToBytes([]byte("leaf1"))
				leaf2 := sum256ToBytes([]byte("leaf2"))
				leaf3 := sum256ToBytes([]byte("leaf3"))

				node0 := sum256ToBytes(append(leaf0, leaf1...))
				node1 := sum256ToBytes(append(leaf2, leaf3...))
				root := sum256ToBytes(append(node0, node1...))
				return root
			}(),
			leaves: [][]byte{
				sum256ToBytes([]byte("leaf0")),
				sum256ToBytes([]byte("leaf1")),
				sum256ToBytes([]byte("leaf2")),
				sum256ToBytes([]byte("leaf3")),
			},
			indices:     []int{4, 5, 6, 7},
			proof:       [][]byte{}, // No proof needed when all leaves are provided
			expectValid: true,
			expectError: false,
		},
		{
			name: "multiproof with duplicate indices",
			root: func() []byte {
				leaf0 := sum256ToBytes([]byte("leaf0"))
				leaf1 := sum256ToBytes([]byte("leaf1"))
				leaf2 := sum256ToBytes([]byte("leaf2"))
				leaf3 := sum256ToBytes([]byte("leaf3"))

				node0 := sum256ToBytes(append(leaf0, leaf1...))
				node1 := sum256ToBytes(append(leaf2, leaf3...))
				root := sum256ToBytes(append(node0, node1...))
				return root
			}(),
			leaves: [][]byte{
				sum256ToBytes([]byte("leaf0")),
				sum256ToBytes([]byte("leaf0")), // Duplicate leaf
			},
			indices:     []int{4, 4}, // Duplicate index
			proof:       [][]byte{sum256ToBytes([]byte("leaf1")), sum256ToBytes(append(sum256ToBytes([]byte("leaf2")), sum256ToBytes([]byte("leaf3"))...))},
			expectValid: true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, err := VerifyMultiproof(tt.root, tt.proof, tt.leaves, tt.indices)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if valid != tt.expectValid {
				t.Errorf("expected valid=%v, got valid=%v", tt.expectValid, valid)
			}
		})
	}
}

func TestVerifyMultiproofAllLeavesDescendingOrder(t *testing.T) {
	root, leaves, _ := buildMerkleTree(4)

	// This checks the full-tree fast path when the caller provides all leaves
	// in descending generalized-index order.
	valid, err := VerifyMultiproof(root, nil, [][]byte{
		leaves[3],
		leaves[2],
		leaves[1],
		leaves[0],
	}, []int{7, 6, 5, 4})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Fatal("expected descending full-tree proof to verify")
	}
}

func TestVerifyFullTreeLeaves(t *testing.T) {
	root, leaves, _ := buildMerkleTree(4)

	t.Run("ascending order", func(t *testing.T) {
		// This is the normal fast-path case: all leaves are present in ascending order.
		ok, valid, _ := verifyFullTreeLeaves(hasher.SHA256, root, leaves, []int{4, 5, 6, 7})
		if !ok {
			t.Fatal("expected fast path to accept ascending full-tree leaves")
		}
		if !valid {
			t.Fatal("expected ascending full-tree leaves to verify")
		}
	})

	t.Run("descending order", func(t *testing.T) {
		// This checks that the helper also accepts the same full tree in reverse order.
		ok, valid, _ := verifyFullTreeLeaves(hasher.SHA256, root, [][]byte{
			leaves[3],
			leaves[2],
			leaves[1],
			leaves[0],
		}, []int{7, 6, 5, 4})
		if !ok {
			t.Fatal("expected fast path to accept descending full-tree leaves")
		}
		if !valid {
			t.Fatal("expected descending full-tree leaves to verify")
		}
	})

	t.Run("rejects non power of two", func(t *testing.T) {
		// A full binary tree must have a power-of-two leaf count for this shortcut.
		ok, valid, _ := verifyFullTreeLeaves(hasher.SHA256, root, leaves[:3], []int{4, 5, 6})
		if ok || valid {
			t.Fatalf("expected helper to reject non-power-of-two leaf count, got ok=%v valid=%v", ok, valid)
		}
	})

	t.Run("rejects malformed ascending indices", func(t *testing.T) {
		// This makes sure the helper rejects leaves that look ascending but skip the expected order.
		ok, valid, _ := verifyFullTreeLeaves(hasher.SHA256, root, leaves, []int{4, 5, 7, 6})
		if ok || valid {
			t.Fatalf("expected helper to reject malformed ascending indices, got ok=%v valid=%v", ok, valid)
		}
	})

	t.Run("rejects malformed descending indices", func(t *testing.T) {
		// This is the same guard as above, but for the descending-order shortcut.
		ok, valid, _ := verifyFullTreeLeaves(hasher.SHA256, root, [][]byte{
			leaves[3],
			leaves[2],
			leaves[1],
			leaves[0],
		}, []int{7, 6, 4, 5})
		if ok || valid {
			t.Fatalf("expected helper to reject malformed descending indices, got ok=%v valid=%v", ok, valid)
		}
	})

	t.Run("rejects unsupported starting index", func(t *testing.T) {
		// The fast path only supports exact full-tree ranges, so unrelated starting indices must fail.
		ok, valid, _ := verifyFullTreeLeaves(hasher.SHA256, root, leaves, []int{5, 6, 7, 8})
		if ok || valid {
			t.Fatalf("expected helper to reject unsupported starting index, got ok=%v valid=%v", ok, valid)
		}
	})
}

func TestGetPosAtLevel(t *testing.T) {
	tests := []struct {
		index    int
		level    int
		expected bool
	}{
		{index: 4, level: 0, expected: false}, // 100 in binary, bit 0 is 0
		{index: 4, level: 1, expected: false}, // 100 in binary, bit 1 is 0
		{index: 4, level: 2, expected: true},  // 100 in binary, bit 2 is 1
		{index: 5, level: 0, expected: true},  // 101 in binary, bit 0 is 1
		{index: 5, level: 1, expected: false}, // 101 in binary, bit 1 is 0
		{index: 5, level: 2, expected: true},  // 101 in binary, bit 2 is 1
		{index: 7, level: 0, expected: true},  // 111 in binary, bit 0 is 1
		{index: 7, level: 1, expected: true},  // 111 in binary, bit 1 is 1
		{index: 7, level: 2, expected: true},  // 111 in binary, bit 2 is 1
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := getPosAtLevel(tt.index, tt.level)
			if result != tt.expected {
				t.Errorf("getPosAtLevel(%d, %d) = %v, want %v", tt.index, tt.level, result, tt.expected)
			}
		})
	}
}

func TestGetPathLength(t *testing.T) {
	tests := []struct {
		index    int
		expected int
	}{
		{index: 1, expected: 0},  // Root node
		{index: 2, expected: 1},  // Level 1
		{index: 3, expected: 1},  // Level 1
		{index: 4, expected: 2},  // Level 2
		{index: 7, expected: 2},  // Level 2
		{index: 8, expected: 3},  // Level 3
		{index: 15, expected: 3}, // Level 3
		{index: 16, expected: 4}, // Level 4
		{index: 31, expected: 4}, // Level 4
		{index: 32, expected: 5}, // Level 5
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := getPathLength(tt.index)
			if result != tt.expected {
				t.Errorf("getPathLength(%d) = %d, want %d", tt.index, result, tt.expected)
			}
		})
	}
}

func TestGetSibling(t *testing.T) {
	tests := []struct {
		index    int
		expected int
	}{
		{index: 1, expected: 0}, // Root's sibling (edge case)
		{index: 2, expected: 3}, // Left child's sibling is right
		{index: 3, expected: 2}, // Right child's sibling is left
		{index: 4, expected: 5},
		{index: 5, expected: 4},
		{index: 6, expected: 7},
		{index: 7, expected: 6},
		{index: 8, expected: 9},
		{index: 9, expected: 8},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := getSibling(tt.index)
			if result != tt.expected {
				t.Errorf("getSibling(%d) = %d, want %d", tt.index, result, tt.expected)
			}
		})
	}
}

func TestGetParent(t *testing.T) {
	tests := []struct {
		index    int
		expected int
	}{
		{index: 1, expected: 0}, // Root's parent (edge case)
		{index: 2, expected: 1}, // Children of root
		{index: 3, expected: 1},
		{index: 4, expected: 2}, // Grandchildren
		{index: 5, expected: 2},
		{index: 6, expected: 3},
		{index: 7, expected: 3},
		{index: 8, expected: 4},
		{index: 9, expected: 4},
		{index: 10, expected: 5},
		{index: 11, expected: 5},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := getParent(tt.index)
			if result != tt.expected {
				t.Errorf("getParent(%d) = %d, want %d", tt.index, result, tt.expected)
			}
		})
	}
}

func TestGetRequiredIndicesEmpty(t *testing.T) {
	result := getRequiredIndices([]int{})
	if result != nil {
		t.Fatalf("expected nil for empty input, got %v", result)
	}
}

func TestGetRequiredIndices(t *testing.T) {
	tests := []struct {
		name             string
		leafIndices      []int
		expectedLen      int
		shouldContain    []int
		shouldNotContain []int
	}{
		{
			name:          "single leaf",
			leafIndices:   []int{4},
			expectedLen:   2,
			shouldContain: []int{5, 3}, // sibling and parent's sibling
		},
		{
			name:          "two adjacent leaves",
			leafIndices:   []int{4, 5},
			expectedLen:   1,
			shouldContain: []int{3}, // only parent's sibling needed
		},
		{
			name:          "two non-adjacent leaves",
			leafIndices:   []int{4, 7},
			expectedLen:   2,
			shouldContain: []int{5, 6}, // siblings of each leaf
		},
		{
			name:        "all four leaves",
			leafIndices: []int{4, 5, 6, 7},
			expectedLen: 0, // no additional hashes needed
		},
		{
			name:          "three leaves",
			leafIndices:   []int{4, 5, 6},
			expectedLen:   1,
			shouldContain: []int{7}, // sibling of leaf 6
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRequiredIndices(tt.leafIndices)

			if len(result) != tt.expectedLen {
				t.Errorf("expected %d required indices, got %d", tt.expectedLen, len(result))
			}

			// Check that result contains expected indices
			for _, expected := range tt.shouldContain {
				found := false
				for _, idx := range result {
					if idx == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected to find index %d in result, but it was not found", expected)
				}
			}

			// Check that result is sorted in descending order
			for i := 1; i < len(result); i++ {
				if result[i] >= result[i-1] {
					t.Errorf("result not sorted in descending order: %v", result)
					break
				}
			}
		})
	}
}

func TestVerifyMultiproofUnsortedIndices(t *testing.T) {
	// Build a tree with 8 leaves (indices 8..15)
	root, _, allNodes := buildMerkleTree(8)

	// Indices that are neither ascending nor descending exercise
	// the sort fallback in both descendingIndices and getRequiredIndices.
	indices := []int{10, 8, 13}
	leafData := make([][]byte, len(indices))
	for i, idx := range indices {
		leafData[i] = allNodes[idx]
	}
	proofHashes := findProofHashes(indices, allNodes)

	valid, err := VerifyMultiproof(root, proofHashes, leafData, indices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Fatal("expected valid proof with unsorted indices")
	}
}

func TestVerifyMultiproofMixedDepthIndices(t *testing.T) {
	// Build a tree with 8 leaves (generalized indices 8..15, depth 3).
	// Verify a multiproof that covers two different depths simultaneously:
	//   index 15 (leaf, depth 3) and index 4 (intermediate node, depth 2).
	root, _, allNodes := buildMerkleTree(8)

	indices := []int{15, 4}
	leafData := [][]byte{allNodes[15], allNodes[4]}
	proofHashes := [][]byte{
		allNodes[14],
		allNodes[6],
		allNodes[5],
	}

	valid, err := VerifyMultiproof(root, proofHashes, leafData, indices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Fatal("expected valid proof for mixed-depth indices")
	}
}

func TestVerifyMultiproofMixedDepthWithoutExplicitProof(t *testing.T) {
	root, _, allNodes := buildMerkleTree(4)

	indices := []int{4, 5, 3}
	leafData := [][]byte{allNodes[4], allNodes[5], allNodes[3]}

	valid, err := VerifyMultiproof(root, nil, leafData, indices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Fatal("expected valid proof with mixed-depth indices and no explicit proof")
	}
}

func TestVerifyMultiproofMissingNodes(t *testing.T) {
	// Build a tree with 4 leaves (indices 4..7)
	root, leaves, allNodes := buildMerkleTree(4)

	// Prove leaf 4 with correct proof
	indices := []int{4}
	leafData := [][]byte{leaves[0]}
	proofHashes := findProofHashes(indices, allNodes)

	// Verify it works normally
	valid, err := VerifyMultiproof(root, proofHashes, leafData, indices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Fatal("expected valid proof")
	}

	// Now corrupt a proof hash so verification fails but doesn't crash
	corruptProof := make([][]byte, len(proofHashes))
	for i := range proofHashes {
		corruptProof[i] = make([]byte, 32)
	}
	valid, err = VerifyMultiproof(root, corruptProof, leafData, indices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if valid {
		t.Fatal("expected invalid proof with corrupted hashes")
	}
}

func TestGetRequiredIndicesUnsorted(t *testing.T) {
	// Provide unsorted indices to exercise the sort branch (lines 294-297)
	result := getRequiredIndices([]int{7, 4, 6})

	// Should still return valid required indices in descending order
	for i := 1; i < len(result); i++ {
		if result[i] >= result[i-1] {
			t.Errorf("result not sorted in descending order: %v", result)
			break
		}
	}
}

func TestGetRequiredIndicesMixedDepth(t *testing.T) {
	tests := []struct {
		name        string
		leafIndices []int
		expected    []int
	}{
		{
			name:        "depth 3 + depth 2",
			leafIndices: []int{15, 4},
			expected:    []int{14, 6, 5},
		},
		{
			name:        "depth 4 + depth 3",
			leafIndices: []int{31, 8},
			expected:    []int{30, 14, 9, 6, 5},
		},
		{
			name:        "depth 3 pair + depth 2",
			leafIndices: []int{14, 15, 4},
			expected:    []int{6, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRequiredIndices(tt.leafIndices)

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d required indices %v, got %d: %v",
					len(tt.expected), tt.expected, len(result), result)
			}

			for i := range result {
				if result[i] != tt.expected[i] {
					t.Fatalf("expected required indices %v, got %v", tt.expected, result)
				}
			}
		})
	}
}

func descendingIndices(indices []int) []int {
	switch {
	case len(indices) == 0:
		return nil
	case intsSortedDescending(indices):
		return indices
	case sort.IntsAreSorted(indices):
		out := make([]int, len(indices))
		for i := range indices {
			out[i] = indices[len(indices)-1-i]
		}
		return out
	default:
		out := make([]int, len(indices))
		copy(out, indices)
		sort.Sort(sort.Reverse(sort.IntSlice(out)))
		return out
	}
}

func TestDescendingIndicesUnsorted(t *testing.T) {
	// Neither ascending nor descending: exercises the default sort branch
	indices := []int{5, 3, 7, 1}
	result := descendingIndices(indices)

	// Should be sorted descending
	expected := []int{7, 5, 3, 1}
	if len(result) != len(expected) {
		t.Fatalf("expected %d indices, got %d", len(expected), len(result))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("result[%d] = %d, want %d", i, result[i], v)
		}
	}
}

func TestNewDescendingIndexCursorEmpty(t *testing.T) {
	cursor := newDescendingIndexCursor(nil)
	if cursor.ok() {
		t.Fatal("expected empty cursor to report not ok")
	}
	if cursor.pos != -1 {
		t.Fatalf("expected empty cursor position -1, got %d", cursor.pos)
	}
}

func TestAppendProofLeafNormalizesChunkWidth(t *testing.T) {
	h := hasher.NewHasher()
	zeroChunk := hasher.GetZeroHash(0)

	oversized := make([]byte, 40)
	for i := range oversized {
		oversized[i] = byte(i + 1)
	}
	appendProofLeaf(h, zeroChunk, oversized)
	if !bytes.Equal(h.Hash(), oversized[:32]) {
		t.Fatalf("expected oversized leaf to be truncated to first 32 bytes, got %x", h.Hash())
	}

	h.Reset()

	shortLeaf := []byte{0xAA, 0xBB, 0xCC}
	appendProofLeaf(h, zeroChunk, shortLeaf)
	expected := make([]byte, 32)
	copy(expected, shortLeaf)
	if !bytes.Equal(h.Hash(), expected) {
		t.Fatalf("expected short leaf to be right-padded to 32 bytes, got %x", h.Hash())
	}

	h.Reset()

	appendProofLeaf(h, zeroChunk, nil)
	if !bytes.Equal(h.Hash(), zeroChunk) {
		t.Fatalf("expected empty leaf to use zero chunk, got %x", h.Hash())
	}
}

func hashFn(data []byte) []byte {
	res := sha256.Sum256(data)
	return res[:]
}

func TestHashFn(t *testing.T) {
	// Test that hashFn produces correct SHA256 hash
	input := []byte("test data")
	expected := sha256.Sum256(input)
	result := hashFn(input)

	if !bytes.Equal(result, expected[:]) {
		t.Errorf("hashFn produced incorrect hash")
	}

	// Test determinism
	result2 := hashFn(input)
	if !bytes.Equal(result, result2) {
		t.Errorf("hashFn is not deterministic")
	}
}

// hashData is a helper to generate a unique 32-byte hash from an integer.
func hashData(i int) []byte {
	h := sha256.New()
	h.Write([]byte(strconv.Itoa(i)))
	return h.Sum(nil)
}

// buildMerkleTree creates a complete Merkle tree up to the root,
// returning the root hash and all leaves/nodes needed for benchmarking.
func buildMerkleTree(numLeaves int) (root []byte, leaves [][]byte, nodes map[int][]byte) {
	if numLeaves == 0 {
		return nil, nil, nil
	}

	leaves = make([][]byte, numLeaves)
	nodes = make(map[int][]byte)

	// Generalized index for the first leaf is numLeaves (2^N)
	leafStartIndex := numLeaves

	// 1. Generate leaves and store them in the 'nodes' map
	for i := range numLeaves {
		leafData := hashData(i)
		leaves[i] = leafData
		nodes[leafStartIndex+i] = leafData
	}

	// 2. Compute intermediate hashes bottom-up
	currentLayer := leafStartIndex
	for currentLayer > 1 {
		parentLayer := currentLayer / 2
		for i := 0; i < currentLayer; i += 2 {
			leftIndex := currentLayer + i
			rightIndex := currentLayer + i + 1

			leftHash := nodes[leftIndex]
			rightHash := nodes[rightIndex]

			parentHash := hashFn(append(leftHash, rightHash...))
			nodes[parentLayer+(i/2)] = parentHash
		}
		currentLayer = parentLayer
	}

	return nodes[1], leaves, nodes
}

// findProofHashes is a simplified version of NodeProveMulti's logic
// to extract the required proof hashes (the siblings not provided as leaves)
// for the VerifyMultiproof benchmark setup.
func findProofHashes(indices []int, allNodes map[int][]byte) [][]byte {
	requiredIndices := getRequiredIndices(indices)
	proofHashes := make([][]byte, len(requiredIndices))

	for i, idx := range requiredIndices {
		proofHashes[i] = allNodes[idx]
	}
	return proofHashes
}

// BenchmarkVerifyMultiproof measures the verification time for different proof sizes
func BenchmarkVerifyMultiproof(b *testing.B) {
	// 2^16 = 65536 leaves, Tree Depth 16. A large, realistic size.
	const numLeaves = 1 << 16

	// Build the large tree once for all benchmarks
	root, allLeaves, allNodes := buildMerkleTree(numLeaves)

	// --- Scenario 1: Proving two adjacent leaves (e.g., 65536 and 65537) ---
	indicesAdj := []int{numLeaves, numLeaves + 1}
	leavesAdj := [][]byte{allLeaves[0], allLeaves[1]}
	proofAdj := findProofHashes(indicesAdj, allNodes)

	b.Run("Prove_2_Adjacent_Leaves", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = VerifyMultiproof(root, proofAdj, leavesAdj, indicesAdj)
		}
	})

	// --- Scenario 2: Proving 16 scattered leaves (low density proof) ---
	// Scattered leaves require more proof hashes and a deeper traversal.
	indicesScattered := make([]int, 16)
	leavesScattered := make([][]byte, 16)
	for i := range 16 {
		idx := numLeaves + i*1000 // Widely scattered indices
		indicesScattered[i] = idx
		leavesScattered[i] = allNodes[idx]
	}
	proofScattered := findProofHashes(indicesScattered, allNodes)

	b.Run("Prove_16_Scattered_Leaves", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = VerifyMultiproof(root, proofScattered, leavesScattered, indicesScattered)
		}
	})

	// --- Scenario 3: Proving all leaves (high density proof) ---
	// Should require zero proof hashes, only computation from leaves.
	indicesAll := make([]int, numLeaves)
	for i := range numLeaves {
		indicesAll[i] = numLeaves + i
	}
	// Note: 'allLeaves' already contains all leaf hashes
	proofAll := findProofHashes(indicesAll, allNodes) // Should be empty

	b.Run("Prove_All_Leaves", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = VerifyMultiproof(root, proofAll, allLeaves, indicesAll)
		}
	})
}

// VerifyProof must reject a nil proof with a clean error instead of panicking.
func TestVerifyProofNil(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("VerifyProof panicked on nil proof: %v", r)
		}
	}()

	ok, err := VerifyProof(make([]byte, 32), nil)
	if err == nil {
		t.Fatal("expected error for nil proof")
	}
	if ok {
		t.Fatal("nil proof must not verify")
	}
}

// VerifyMultiproof must reject non-positive generalized indices with a clean
// error and must never panic or hang on them.
func TestVerifyMultiproofRejectsNonPositiveIndex(t *testing.T) {
	root := make([]byte, 32)

	for _, indices := range [][]int{{0}, {0, -1}, {4, 0}, {-1, 2}} {
		leaves := make([][]byte, len(indices))
		for i := range leaves {
			leaves[i] = make([]byte, 32)
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("VerifyMultiproof panicked on %v: %v", indices, r)
				}
			}()
			ok, err := VerifyMultiproof(root, nil, leaves, indices)
			if err == nil {
				t.Errorf("expected error for indices %v", indices)
			}
			if ok {
				t.Errorf("indices %v must not verify", indices)
			}
		}()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("VerifyMultiproof hung on indices %v", indices)
		}
	}
}

func TestVerifyMultiproofAncestorDescendantTamper(t *testing.T) {
	root, _, allNodes := buildMerkleTree(8)

	// Index set containing an intermediate node (2) together with one of its
	// descendant leaves (8).
	indices := []int{8, 2}
	proofHashes := findProofHashes(indices, allNodes)

	valid, err := VerifyMultiproof(root, proofHashes, [][]byte{allNodes[8], allNodes[2]}, indices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Fatal("expected valid proof for consistent ancestor+descendant index set")
	}

	// Tampering only the descendant leaf must invalidate the proof even though
	// the supplied ancestor hash is still consistent with the root.
	tampered := append([]byte{}, allNodes[8]...)
	tampered[0] ^= 0xff
	valid, err = VerifyMultiproof(root, proofHashes, [][]byte{tampered, allNodes[2]}, indices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if valid {
		t.Fatal("expected tampered descendant leaf to be rejected")
	}
}

func TestVerifyMultiproofRootInSetTamper(t *testing.T) {
	root, _, allNodes := buildMerkleTree(8)

	// Index set containing the root itself (generalized index 1) plus a leaf.
	indices := []int{1, 8}
	proofHashes := findProofHashes(indices, allNodes)

	valid, err := VerifyMultiproof(root, proofHashes, [][]byte{allNodes[1], allNodes[8]}, indices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Fatal("expected valid proof with root in index set")
	}

	// The supplied root hash must not short-circuit verification of the other
	// leaves: a tampered leaf has to be rejected.
	tampered := append([]byte{}, allNodes[8]...)
	tampered[0] ^= 0xff
	valid, err = VerifyMultiproof(root, proofHashes, [][]byte{allNodes[1], tampered}, indices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if valid {
		t.Fatal("expected tampered leaf to be rejected when root is in the index set")
	}
}

func TestVerifyMultiproofDuplicateIndexConflict(t *testing.T) {
	root, _, allNodes := buildMerkleTree(8)

	proofHashes := findProofHashes([]int{8}, allNodes)
	good := allNodes[8]
	bad := append([]byte{}, good...)
	bad[0] ^= 0xff

	// Conflicting values for the same generalized index must be rejected
	// regardless of their order in the leaf list.
	for _, leaves := range [][][]byte{{bad, good}, {good, bad}} {
		valid, err := VerifyMultiproof(root, proofHashes, leaves, []int{8, 8})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if valid {
			t.Fatal("expected conflicting duplicate leaves to be rejected")
		}
	}

	// Consistent duplicates stay verifiable.
	valid, err := VerifyMultiproof(root, proofHashes, [][]byte{good, good}, []int{8, 8})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Fatal("expected consistent duplicate leaves to verify")
	}
}

// A chunk shorter than 32 bytes would be zero-extended, so a chunk ending in
// zero bytes would have as many encodings as it has trailing zeros -- all
// verifying against the same root. Every chunk a verifier is handed has to be
// exactly one length.
func TestVerifyRejectsUndersizedChunks(t *testing.T) {
	// A leaf whose value ends in zero bytes: truncating it is lossless once the
	// verifier pads it back, so every truncation used to verify against this
	// same root.
	zeroTailed := make([][]byte, 4)
	for i := range zeroTailed {
		chunk := make([]byte, 32)
		chunk[0] = byte(i + 1)
		zeroTailed[i] = chunk
	}
	tree, err := TreeFromChunks(zeroTailed)
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	zeroRoot := tree.Hash()
	zeroProof, err := tree.Prove(4)
	if err != nil {
		t.Fatalf("prove: %v", err)
	}
	if valid, err := VerifyProof(zeroRoot, zeroProof); err != nil || !valid {
		t.Fatalf("the full-length proof must verify: valid=%v err=%v", valid, err)
	}

	t.Run("leaf", func(t *testing.T) {
		for _, n := range []int{0, 1, 16, 31} {
			short := &Proof{Index: zeroProof.Index, Leaf: zeroProof.Leaf[:n], Hashes: zeroProof.Hashes}
			if valid, err := VerifyProof(zeroRoot, short); err == nil || valid {
				t.Errorf("a %d-byte leaf verified: valid=%v err=%v", n, valid, err)
			}
		}
	})

	root, leaves, allNodes := buildMerkleTree(4)
	full := &Proof{Index: 4, Leaf: leaves[0], Hashes: [][]byte{allNodes[5], allNodes[3]}}
	if valid, err := VerifyProof(root, full); err != nil || !valid {
		t.Fatalf("the reference proof must verify: valid=%v err=%v", valid, err)
	}

	t.Run("proof hash", func(t *testing.T) {
		short := &Proof{Index: 4, Leaf: leaves[0], Hashes: [][]byte{allNodes[5][:16], allNodes[3]}}
		if valid, err := VerifyProof(root, short); err == nil || valid {
			t.Errorf("a 16-byte proof hash verified: valid=%v err=%v", valid, err)
		}
	})

	t.Run("multiproof leaf", func(t *testing.T) {
		indices := []int{4}
		hashes := findProofHashes(indices, allNodes)
		if valid, err := VerifyMultiproof(root, hashes, [][]byte{allNodes[4][:8]}, indices); err == nil || valid {
			t.Errorf("an 8-byte leaf verified: valid=%v err=%v", valid, err)
		}
	})

	t.Run("multiproof root", func(t *testing.T) {
		// VerifyMultiproof compared the root as a padded chunk, so a truncated
		// root -- or one with bytes appended past the first 32 -- was accepted.
		indices := []int{4}
		hashes := findProofHashes(indices, allNodes)
		for _, bad := range [][]byte{root[:16], append(append([]byte{}, root...), 0xff)} {
			if valid, err := VerifyMultiproof(bad, hashes, [][]byte{allNodes[4]}, indices); err == nil || valid {
				t.Errorf("a %d-byte root verified: valid=%v err=%v", len(bad), valid, err)
			}
		}
	})
}

func TestVerifyProofRejectsOversizedLeaf(t *testing.T) {
	root, leaves, allNodes := buildMerkleTree(4)

	proof := &Proof{
		Index:  4,
		Leaf:   append(append([]byte{}, leaves[0]...), 0xde, 0xad),
		Hashes: [][]byte{allNodes[5], allNodes[3]},
	}

	// Anything beyond 32 bytes would be silently truncated and verify against
	// a value the caller never proved.
	valid, err := VerifyProof(root, proof)
	if err == nil {
		t.Fatal("expected error for oversized leaf")
	}
	if valid {
		t.Fatal("oversized leaf must not verify")
	}
}

func TestVerifyMultiproofRejectsOversizedLeaf(t *testing.T) {
	root, _, allNodes := buildMerkleTree(4)

	indices := []int{4}
	proofHashes := findProofHashes(indices, allNodes)
	oversized := append(append([]byte{}, allNodes[4]...), 0x00)

	valid, err := VerifyMultiproof(root, proofHashes, [][]byte{oversized}, indices)
	if err == nil {
		t.Fatal("expected error for oversized leaf")
	}
	if valid {
		t.Fatal("oversized leaf must not verify")
	}
}

// VerifyProof / VerifyMultiproof reject leaves longer than 32 bytes; they must
// reject oversized proof hashes for the same reason (silent truncation would
// verify against a value the caller never supplied).
func TestVerifyRejectsOversizedProofHashes(t *testing.T) {
	root, _, allNodes := buildMerkleTree(4)

	over := append(append([]byte{}, allNodes[5]...), 0xde, 0xad) // 34 bytes

	proof := &Proof{
		Index:  4,
		Leaf:   allNodes[4],
		Hashes: [][]byte{over, allNodes[3]},
	}
	if ok, err := VerifyProof(root, proof); ok || err == nil {
		t.Errorf("VerifyProof accepted an oversized proof hash: ok=%v err=%v", ok, err)
	}

	if ok, err := VerifyMultiproof(root, [][]byte{over, allNodes[3]}, [][]byte{allNodes[4]}, []int{4}); ok || err == nil {
		t.Errorf("VerifyMultiproof accepted an oversized proof hash: ok=%v err=%v", ok, err)
	}
}

// --- moved from errpath_test.go ---
// --- VerifyMultiproof: missing required nodes error path ---

func TestVerifyMultiproofMissingNodeInjected(t *testing.T) {
	root, leaves, allNodes := buildMerkleTree(4)
	indices := []int{4}
	leafData := [][]byte{leaves[0]}

	// Real required indices would be [5, 3]. Return only [3] so that
	// sibling 5 is never populated, triggering the missing-node error.
	getRequiredIndicesFn = func([]int) []int {
		return []int{3}
	}
	defer func() { getRequiredIndicesFn = getRequiredIndices }()

	proofHashes := [][]byte{allNodes[3]}

	_, err := VerifyMultiproof(root, proofHashes, leafData, indices)
	if err == nil {
		t.Fatal("expected missing-node error")
	}
	if !strings.Contains(err.Error(), "proof is missing required nodes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A same-depth proof whose required-node list is shorter than the frontier
// leaves the verifier without a sibling it needs; it must report the gap
// rather than reading past the proof slice.
func TestVerifyMultiproofSameDepthProofExhausted(t *testing.T) {
	root, leaves, _ := buildMerkleTree(4)

	getRequiredIndicesFn = func([]int) []int { return nil }
	defer func() { getRequiredIndicesFn = getRequiredIndices }()

	_, err := VerifyMultiproof(root, nil, [][]byte{leaves[0]}, []int{4})
	if err == nil || !strings.Contains(err.Error(), "proof is missing required nodes") {
		t.Fatalf("expected missing-node error, got %v", err)
	}
}

// More than inlineIndexedChunkCapacity same-depth leaves forces the same-depth
// verifier onto its heap-backed scratch slices instead of the inline arrays.
func TestVerifyMultiproofManySameDepthLeaves(t *testing.T) {
	const numLeaves = 128
	root, _, allNodes := buildMerkleTree(numLeaves)

	const proven = inlineIndexedChunkCapacity + 1
	indices := make([]int, proven)
	leaves := make([][]byte, proven)
	for i := range indices {
		idx := numLeaves + i
		indices[i] = idx
		leaves[i] = allNodes[idx]
	}
	proof := findProofHashes(indices, allNodes)

	valid, err := VerifyMultiproof(root, proof, leaves, indices)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Fatal("expected a same-depth multiproof of 65 leaves to verify")
	}
}

// Both verifiers keep a defensive backstop for the impossible case where the
// upward walk never reconstructs the root. Empty inputs drive that guard
// directly, since the public entry point cannot produce this shape.
func TestVerifyMultiproofRootNeverComputed(t *testing.T) {
	root := make([]byte, 32)

	handled, valid, err := verifyMultiproofSameDepth(hasher.SHA256, root, nil, nil, nil, nil)
	if !handled || valid || err == nil {
		t.Fatalf("same-depth backstop: handled=%v valid=%v err=%v", handled, valid, err)
	}

	valid, err = verifyMultiproofGeneral(hasher.SHA256, root, nil, nil, nil, nil)
	if valid || err == nil {
		t.Fatalf("general backstop: valid=%v err=%v", valid, err)
	}
}

// Exercises the short-circuit and mismatch branches of the internal index
// helpers that the cached happy paths never reach.
func TestProofIndexHelperEdgeCases(t *testing.T) {
	if got := computeRequiredIndices(nil); got != nil {
		t.Fatalf("computeRequiredIndices(nil) = %v, want nil", got)
	}
	if got := descendingUniqueIndices(nil); got != nil {
		t.Fatalf("descendingUniqueIndices(nil) = %v, want nil", got)
	}

	// Ascending input with a duplicate must be rejected during population.
	dst := make([]indexedChunk, 3)
	leaves := [][]byte{{1}, {2}, {3}}
	if populateDescendingIndexedChunks(dst, []int{4, 5, 5}, leaves) {
		t.Fatal("expected ascending duplicate indices to be rejected")
	}

	// intsEqual separates differing lengths from differing elements.
	if intsEqual([]int{1, 2}, []int{1, 2, 3}) {
		t.Fatal("intsEqual: differing lengths must be unequal")
	}
	if intsEqual([]int{1, 2, 3}, []int{1, 9, 3}) {
		t.Fatal("intsEqual: differing element must be unequal")
	}
}

// VerifyProof must reject a non-positive generalized index, mirroring
// VerifyMultiproof. Otherwise Index=-1 (whose path length is 63) would be
// processed as a valid 63-deep proof.
func TestVerifyProofRejectsNonPositiveIndex(t *testing.T) {
	for _, idx := range []int{0, -1} {
		proof := &Proof{Index: idx, Leaf: make([]byte, 32)}
		ok, err := VerifyProof(make([]byte, 32), proof)
		if err == nil || ok {
			t.Fatalf("VerifyProof(Index=%d) = (%v, %v); want (false, error)", idx, ok, err)
		}
	}
}

// saltedSHA256 is a hash of its own: SHA256 with the first byte of every
// digest flipped by salt. It refuses to hash while fail is set.
func saltedSHA256(salt byte, fail *atomic.Bool) hasher.HashFn {
	return func(dst, input []byte) error {
		if fail != nil && fail.Load() {
			return errBackendRefused
		}
		for i := 0; i+64 <= len(input); i += 64 {
			sum := sha256.Sum256(input[i : i+64])
			sum[0] ^= salt
			copy(dst[i/2:], sum[:])
		}
		return nil
	}
}

var errBackendRefused = errors.New("backend refused")

func customAlgorithm(t *testing.T, salt byte, fail *atomic.Bool) *hasher.Algorithm {
	t.Helper()
	alg, err := hasher.AlgorithmFor(saltedSHA256(salt, fail))
	if err != nil {
		t.Fatalf("AlgorithmFor: %v", err)
	}
	return alg
}

func testAlgorithms(t *testing.T) map[string]*hasher.Algorithm {
	t.Helper()
	return map[string]*hasher.Algorithm{
		"sha256":           hasher.SHA256,
		"sha256 native":    hasher.SHA256Native,
		"keccak256":        hasher.Keccak256,
		"keccak256 native": hasher.Keccak256Native,
		"custom":           customAlgorithm(t, 0x01, nil),
	}
}

// paddedListWalk hashes a list of five chunks with room for 64: the data
// tree holds zero subtrees of several depths.
func paddedListWalk(hh sszutils.HashWalker) {
	idx := hh.StartTree(sszutils.TreeTypeBinary)
	for i := uint64(0); i < 5; i++ {
		hh.PutUint64(i + 1)
	}
	hh.MerkleizeWithMixin(idx, 5, 64)
}

func fullTreeWalk(hh sszutils.HashWalker) {
	idx := hh.StartTree(sszutils.TreeTypeBinary)
	for i := uint64(0); i < 4; i++ {
		hh.PutUint64(i + 1)
	}
	hh.Merkleize(idx)
}

var algorithmWalks = []struct {
	name string
	walk func(hh sszutils.HashWalker)
}{
	{"padded list", paddedListWalk},
	{"full tree", fullTreeWalk},
	{"single chunk", func(hh sszutils.HashWalker) { hh.PutUint64(7) }},
	{"empty list", func(hh sszutils.HashWalker) {
		idx := hh.StartTree(sszutils.TreeTypeBinary)
		hh.MerkleizeWithMixin(idx, 0, 1024)
	}},
	{"vector", func(hh sszutils.HashWalker) {
		idx := hh.StartTree(sszutils.TreeTypeBinary)
		for i := uint64(0); i < 5; i++ {
			hh.PutUint64(i)
		}
		hh.Merkleize(idx)
	}},
	{"progressive list", func(hh sszutils.HashWalker) {
		idx := hh.StartTree(sszutils.TreeTypeProgressive)
		for i := uint64(0); i < 7; i++ {
			hh.PutUint64(i)
		}
		hh.MerkleizeProgressiveWithMixin(idx, 7)
	}},
	{"progressive container", func(hh sszutils.HashWalker) {
		idx := hh.StartTree(sszutils.TreeTypeProgressive)
		for i := uint64(0); i < 3; i++ {
			hh.PutUint64(i)
		}
		// 70 bytes of active fields span three chunks.
		hh.MerkleizeProgressiveWithActiveFields(idx, bytes.Repeat([]byte{0x55}, 70))
	}},
	{"list of containers", func(hh sszutils.HashWalker) {
		idx := hh.StartTree(sszutils.TreeTypeBinary)
		for i := uint64(0); i < 3; i++ {
			ci := hh.StartTree(sszutils.TreeTypeNone)
			hh.PutUint64(i)
			hh.PutBytes(bytes.Repeat([]byte{byte(i)}, 48))
			hh.PutUint64(i + 7)
			hh.Merkleize(ci)
		}
		hh.MerkleizeWithMixin(idx, 3, 16)
	}},
}

func algorithmRoot(t *testing.T, alg *hasher.Algorithm, walk func(hh sszutils.HashWalker)) [32]byte {
	t.Helper()
	hh := alg.Pool().Get()
	defer alg.Pool().Put(hh)
	walk(hh)
	root, err := hh.HashRoot()
	if err != nil {
		t.Fatalf("hasher HashRoot: %v", err)
	}
	return root
}

func algorithmTree(t *testing.T, alg *hasher.Algorithm, walk func(hh sszutils.HashWalker)) (*Node, []byte) {
	t.Helper()
	w := NewWrapper(WithAlgorithm(alg))
	walk(w)
	tree, err := w.Root()
	if err != nil {
		t.Fatalf("wrapper Root: %v", err)
	}
	root := tree.Hash()

	return tree, root
}

func TestVerifyProofWithAlgorithm(t *testing.T) {
	other := customAlgorithm(t, 0x02, nil)

	for name, alg := range testAlgorithms(t) {
		// The list root is 1, its data tree 2 and its 64 leaves 128..191:
		// 130 is a value, 168 lies in the zero padding.
		tree, root := algorithmTree(t, alg, paddedListWalk)
		for _, index := range []int{130, 168, 3} {
			proof, err := tree.Prove(index)
			if err != nil {
				t.Fatalf("%s: Prove(%d): %v", name, index, err)
			}
			ok, err := VerifyProof(root, proof, WithAlgorithm(alg))
			if err != nil || !ok {
				t.Errorf("%s: VerifyProof(%d) = %v, %v", name, index, ok, err)
			}
			ok, err = VerifyProof(root, proof, WithAlgorithm(other))
			if err != nil || ok {
				t.Errorf("%s: VerifyProof(%d) under another hash = %v, %v", name, index, ok, err)
			}
			// The plain function reads the proof as one of a SHA256 tree.
			ok, err = VerifyProof(root, proof)
			if err != nil || ok != alg.Compatible(hasher.SHA256) {
				t.Errorf("%s: VerifyProof(%d) = %v, %v", name, index, ok, err)
			}

			node, err := tree.Get(index)
			if err != nil {
				t.Fatalf("%s: Get(%d): %v", name, index, err)
			}
			if !bytes.Equal(node.Hash(), proof.Leaf) {
				t.Errorf("%s: Get(%d) = %x, proof leaf %x", name, index, node.Hash(), proof.Leaf)
			}
		}
	}
}

func TestVerifyMultiproofWithAlgorithm(t *testing.T) {
	other := customAlgorithm(t, 0x02, nil)

	for name, alg := range testAlgorithms(t) {
		for _, tc := range []struct {
			name    string
			walk    func(hh sszutils.HashWalker)
			indices []int
		}{
			{"same depth", paddedListWalk, []int{128, 131}},
			{"mixed depth", paddedListWalk, []int{130, 3}},
			{"padding", paddedListWalk, []int{129, 168}},
			{"full leaf layer", fullTreeWalk, []int{4, 5, 6, 7}},
		} {
			tree, root := algorithmTree(t, alg, tc.walk)
			proof, err := tree.ProveMulti(tc.indices)
			if err != nil {
				t.Fatalf("%s %s: ProveMulti: %v", name, tc.name, err)
			}
			ok, err := VerifyMultiproof(root, proof.Hashes, proof.Leaves, proof.Indices, WithAlgorithm(alg))
			if err != nil || !ok {
				t.Errorf("%s %s: VerifyMultiproof = %v, %v", name, tc.name, ok, err)
			}
			ok, err = VerifyMultiproof(root, proof.Hashes, proof.Leaves, proof.Indices, WithAlgorithm(other))
			if err != nil || ok {
				t.Errorf("%s %s: VerifyMultiproof under another hash = %v, %v", name, tc.name, ok, err)
			}

			// A compressed proof omits the zero hashes of its Algorithm.
			compressed := proof.Compress(WithAlgorithm(alg))
			if tc.name == "same depth" {
				deep := 0
				for _, level := range compressed.ZeroLevels {
					if level > 0 {
						deep++
					}
				}
				if deep == 0 {
					t.Errorf("%s: no zero subtree was compressed: levels %v", name, compressed.ZeroLevels)
				}
			}
			restored := compressed.Decompress(WithAlgorithm(alg))
			for i := range proof.Hashes {
				if !bytes.Equal(restored.Hashes[i], proof.Hashes[i]) {
					t.Errorf("%s %s: restored hash %d = %x, want %x", name, tc.name, i, restored.Hashes[i], proof.Hashes[i])
				}
			}
			// To another hash those are plain hashes.
			for _, level := range proof.Compress(WithAlgorithm(other)).ZeroLevels {
				if level > 0 {
					t.Errorf("%s %s: another hash compressed a zero subtree of depth %d", name, tc.name, level)
				}
			}
		}
	}
}

// A backend that refuses to hash fails the tree and the verification instead
// of deciding them.
func TestAlgorithmBackendRefusal(t *testing.T) {
	var fail atomic.Bool
	alg := customAlgorithm(t, 0x03, &fail)

	tree, root := algorithmTree(t, alg, paddedListWalk)
	proof, err := tree.Prove(130)
	if err != nil {
		t.Fatalf("Prove: %v", err)
	}
	multiproofs := make(map[string]*Multiproof, 4)
	roots := make(map[string][]byte, 4)
	for name, indices := range map[string][]int{
		"same depth pair":    {128, 129},
		"same depth sibling": {128, 131},
		"mixed depth":        {130, 3},
	} {
		if multiproofs[name], err = tree.ProveMulti(indices); err != nil {
			t.Fatalf("%s: ProveMulti: %v", name, err)
		}
		roots[name] = root
	}
	fullTree, fullRoot := algorithmTree(t, alg, fullTreeWalk)
	if multiproofs["full leaf layer"], err = fullTree.ProveMulti([]int{4, 5, 6, 7}); err != nil {
		t.Fatalf("ProveMulti: %v", err)
	}
	roots["full leaf layer"] = fullRoot

	fail.Store(true)
	defer fail.Store(false)

	if ok, verifyErr := VerifyProof(root, proof, WithAlgorithm(alg)); !errors.Is(verifyErr, errBackendRefused) || ok {
		t.Errorf("VerifyProof = %v, %v; want the refusal", ok, verifyErr)
	}
	for name, mp := range multiproofs {
		ok, verifyErr := VerifyMultiproof(roots[name], mp.Hashes, mp.Leaves, mp.Indices, WithAlgorithm(alg))
		if !errors.Is(verifyErr, errBackendRefused) || ok {
			t.Errorf("%s: VerifyMultiproof = %v, %v; want the refusal", name, ok, verifyErr)
		}
	}

	// A tree that could not be hashed reports the refusal and has no root.
	w := NewWrapper(WithAlgorithm(alg))
	paddedListWalk(w)
	node, rootErr := w.Root()
	if rootErr != nil {
		t.Fatalf("wrapper Root: %v", rootErr)
	}
	if finalizeErr := node.Finalize(); !errors.Is(finalizeErr, errBackendRefused) {
		t.Errorf("Finalize = %v, want the refusal", finalizeErr)
	}
	if _, proveErr := node.Prove(130); !errors.Is(proveErr, errBackendRefused) {
		t.Errorf("Prove = %v, want the refusal", proveErr)
	}
	if got := node.Hash(); got != nil {
		t.Errorf("Hash = %x, want no root", got)
	}

	hashed := NewWrapper(WithAlgorithm(alg))
	paddedListWalk(hashed)
	if _, hashErr := hashed.HashRoot(); !errors.Is(hashErr, errBackendRefused) || !errors.Is(hashed.HashErr(), errBackendRefused) {
		t.Errorf("wrapper HashRoot = %v, HashErr = %v; want the refusal", hashErr, hashed.HashErr())
	}

	// Once the backend answers, the same tree completes with its Algorithm.
	fail.Store(false)
	if finalizeErr := node.Finalize(); finalizeErr != nil {
		t.Fatalf("resumed Finalize: %v", finalizeErr)
	}
	want := algorithmRoot(t, alg, paddedListWalk)
	if !bytes.Equal(node.Hash(), want[:]) {
		t.Errorf("resumed root %x, want %x", node.Hash(), want)
	}
}

// Rejected multiproofs over large, distinct index sets keep the required
// indices cache within its budget, a set over the budget is not stored, and
// small sets are still served from the cache.
func TestRequiredIndicesCacheBudget(t *testing.T) {
	root := make([]byte, 32)
	verify := func(indices []int) {
		leaves := make([][]byte, len(indices))
		for i := range leaves {
			leaves[i] = make([]byte, 32)
		}
		if ok, err := VerifyMultiproof(root, nil, leaves, indices); ok || err == nil {
			t.Fatalf("forged proof accepted (ok=%v, err=%v)", ok, err)
		}
	}
	sparse := func(n, offset int) []int {
		indices := make([]int, n)
		for i := range indices {
			indices[i] = 1<<28 + i*7 + offset
		}
		return indices
	}
	held := func() int {
		requiredIndicesCache.mu.RLock()
		defer requiredIndicesCache.mu.RUnlock()
		total := 0
		for i := range requiredIndicesCache.entries {
			total += len(requiredIndicesCache.entries[i].indices) + len(requiredIndicesCache.entries[i].required)
		}
		if total != requiredIndicesCache.ints {
			t.Fatalf("cache tracks %d ints but holds %d", requiredIndicesCache.ints, total)
		}
		return total
	}

	for k := range 2 * requiredIndicesCacheSize {
		verify(sparse(16384, k))
		if n := held(); n > requiredIndicesCacheMaxInts {
			t.Fatalf("cache holds %d ints, over the budget of %d", n, requiredIndicesCacheMaxInts)
		}
	}

	big := sparse(1<<17, 0)
	verify(big)
	for i := range requiredIndicesCache.entries {
		if slices.Equal(requiredIndicesCache.entries[i].indices, big) {
			t.Fatal("set over the budget was cached")
		}
	}

	small := []int{1<<16 + 3, 1<<16 + 1000}
	if first, second := getRequiredIndices(small), getRequiredIndices(small); &first[0] != &second[0] {
		t.Error("small set was not served from the cache")
	}
}
