// Copyright (c) 2025 pk910
// SPDX-License-Identifier: Apache-2.0
// This file is part of the dynamic-ssz library.

package treeproof

import (
	"bytes"
	"crypto/sha256"
	"strconv"
	"testing"
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
			name:        "empty indices",
			root:        []byte{1, 2, 3},
			proof:       [][]byte{},
			leaves:      [][]byte{},
			indices:     []int{},
			expectValid: false,
			expectError: true,
		},
		{
			name:    "mismatched leaves and indices",
			root:    []byte{1, 2, 3},
			proof:   [][]byte{},
			leaves:  [][]byte{{1}, {2}},
			indices: []int{1},
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

func TestGetPosAtLevel(t *testing.T) {
	tests := []struct {
		index    int
		level    int
		expected bool
	}{
		{index: 4, level: 0, expected: false},  // 100 in binary, bit 0 is 0
		{index: 4, level: 1, expected: false},  // 100 in binary, bit 1 is 0
		{index: 4, level: 2, expected: true},   // 100 in binary, bit 2 is 1
		{index: 5, level: 0, expected: true},   // 101 in binary, bit 0 is 1
		{index: 5, level: 1, expected: false},  // 101 in binary, bit 1 is 0
		{index: 5, level: 2, expected: true},   // 101 in binary, bit 2 is 1
		{index: 7, level: 0, expected: true},   // 111 in binary, bit 0 is 1
		{index: 7, level: 1, expected: true},   // 111 in binary, bit 1 is 1
		{index: 7, level: 2, expected: true},   // 111 in binary, bit 2 is 1
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
		{index: 1, expected: 0},   // Root node
		{index: 2, expected: 1},   // Level 1
		{index: 3, expected: 1},   // Level 1
		{index: 4, expected: 2},   // Level 2
		{index: 7, expected: 2},   // Level 2
		{index: 8, expected: 3},   // Level 3
		{index: 15, expected: 3},  // Level 3
		{index: 16, expected: 4},  // Level 4
		{index: 31, expected: 4},  // Level 4
		{index: 32, expected: 5},  // Level 5
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
		{index: 1, expected: 0},   // Root's sibling (edge case)
		{index: 2, expected: 3},   // Left child's sibling is right
		{index: 3, expected: 2},   // Right child's sibling is left
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
		{index: 1, expected: 0},  // Root's parent (edge case)
		{index: 2, expected: 1},  // Children of root
		{index: 3, expected: 1},
		{index: 4, expected: 2},  // Grandchildren
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

func TestGetRequiredIndices(t *testing.T) {
	tests := []struct {
		name         string
		leafIndices  []int
		expectedLen  int
		shouldContain []int
		shouldNotContain []int
	}{
		{
			name:         "single leaf",
			leafIndices:  []int{4},
			expectedLen:  2,
			shouldContain: []int{5, 3}, // sibling and parent's sibling
		},
		{
			name:         "two adjacent leaves",
			leafIndices:  []int{4, 5},
			expectedLen:  1,
			shouldContain: []int{3}, // only parent's sibling needed
		},
		{
			name:         "two non-adjacent leaves",
			leafIndices:  []int{4, 7},
			expectedLen:  2,
			shouldContain: []int{5, 6}, // siblings of each leaf
		},
		{
			name:         "all four leaves",
			leafIndices:  []int{4, 5, 6, 7},
			expectedLen:  0, // no additional hashes needed
		},
		{
			name:         "three leaves",
			leafIndices:  []int{4, 5, 6},
			expectedLen:  1,
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
            nodes[parentLayer + (i/2)] = parentHash
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
        for b.Loop() {
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
        for b.Loop() {
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
        for b.Loop() {
            _, _ = VerifyMultiproof(root, proofAll, allLeaves, indicesAll)
        }
    })
}