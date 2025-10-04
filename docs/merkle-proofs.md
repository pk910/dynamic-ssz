# Merkle Proofs

Dynamic SSZ provides comprehensive Merkle tree construction and proof generation capabilities for SSZ structures. This enables efficient verification of specific fields within complex data structures without requiring the complete data.

## Overview

Merkle proofs are cryptographic proofs that allow verification of specific data within a larger structure using only the Merkle root and a minimal set of hashes. Dynamic SSZ implements both traditional binary trees and progressive trees for advanced use cases.

### Key Features
- **Complete Tree Construction**: Build full Merkle trees from SSZ structures
- **Proof Generation**: Create proofs for any field using generalized indices
- **Proof Verification**: Standalone verification without original data
- **Multi-proofs**: Efficient batch proofs for multiple fields
- **Progressive Trees**: Support for EIP-7916/7495 progressive containers
- **Debug Tools**: Tree visualization and structure analysis

## Quick Start

### Basic Tree Construction and Proof Generation

```go
package main

import (
    "fmt"
    dynssz "github.com/pk910/dynamic-ssz"
    "github.com/pk910/dynamic-ssz/treeproof"
)

type BeaconBlock struct {
    Slot          uint64
    ProposerIndex uint64
    ParentRoot    [32]byte
    StateRoot     [32]byte
}

func main() {
    // Create your data structure
    block := &BeaconBlock{
        Slot:          12345,
        ProposerIndex: 42,
        ParentRoot:    [32]byte{1, 2, 3}, // ... 
        StateRoot:     [32]byte{4, 5, 6}, // ...
    }
    
    // Create DynSSZ instance
    ds := dynssz.NewDynSsz(nil)
    
    // Build the complete Merkle tree
    tree, err := ds.GetTree(block)
    if err != nil {
        panic(err)
    }
    
    // Display tree structure for debugging
    tree.Show(3) // Show 3 levels deep
    
    // Generate proof for StateRoot field (at generalized index 7 in this example)
    proof, err := tree.Prove(7)
    if err != nil {
        panic(err)
    }
    
    // Verify the proof
    isValid, err := treeproof.VerifyProof(tree.Hash(), proof)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Proof valid: %v\n", isValid)
    fmt.Printf("Proved field at index %d with %d sibling hashes\n", 
               proof.Index, len(proof.Hashes))
}
```

## Core Concepts

### Generalized Indices

Generalized indices provide a way to address any node in a binary Merkle tree using a single integer:

- **Index 1**: Root of the tree
- **Index 2n**: Left child of node n
- **Index 2n+1**: Right child of node n

```
                    1 (root)
           2                    3
       4       5            6       7
    8    9  10   11     12   13  14   15
```

For a 4-field container:
- Field 0: Index 4
- Field 1: Index 5  
- Field 2: Index 6
- Field 3: Index 7

### Tree Types

#### Binary Trees
Standard SSZ merkleization creates balanced binary trees where all leaves are at the same depth.

```go
// Regular container - produces binary tree
type StandardContainer struct {
    Field1 uint64
    Field2 uint64
    Field3 uint64
    Field4 uint64
}
```

#### Progressive Trees
Progressive containers (with `ssz-index` tags) create unbalanced trees optimized for sparse data.

```go
// Progressive container - produces progressive tree
type ProgressiveContainer struct {
    Field0 uint64 `ssz-index:"0"`
    Field1 uint64 `ssz-index:"1"`
    Field5 uint64 `ssz-index:"5"`  // Gap creates progressive structure
}
```

## API Reference

### GetTree Method

```go
func (d *DynSsz) GetTree(source any) (*treeproof.Node, error)
```

Builds and returns the complete Merkle tree for any SSZ-compatible structure.

**Parameters**:
- `source` - Any Go value that can be SSZ-encoded

**Returns**:
- `*treeproof.Node` - Root node of the complete tree
- `error` - Error if tree construction fails

### Node Methods

#### Navigation and Access

```go
// Get node at specific generalized index
func (n *Node) Get(index int) (*Node, error)

// Get hash of this node
func (n *Node) Hash() []byte
```

#### Proof Generation

```go
// Generate proof for single leaf
func (n *Node) Prove(index int) (*Proof, error)

// Generate multi-proof for multiple leaves
func (n *Node) ProveMulti(indices []int) (*Multiproof, error)
```

#### Debugging

```go
// Display tree structure with indices
func (n *Node) Show(maxDepth int)
```

### Proof Types

#### Single Proof

```go
type Proof struct {
    Index  int      // Generalized index of proven leaf
    Leaf   []byte   // 32-byte leaf value
    Hashes [][]byte // Sibling hashes for verification
}
```

#### Multi-proof

```go
type Multiproof struct {
    Indices []int      // Generalized indices of proven leaves
    Leaves  [][]byte   // 32-byte leaf values (ordered by Indices)
    Hashes  [][]byte   // Shared verification hashes
}
```

### Verification Functions

```go
// Verify single proof
func VerifyProof(root []byte, proof *Proof) (bool, error)

// Verify multi-proof
func VerifyMultiproof(root []byte, proof [][]byte, leaves [][]byte, indices []int) (bool, error)
```

## Advanced Usage

### Working with Progressive Containers

Progressive containers create different tree structures that affect generalized indices:

```go
type ProgressiveBlock struct {
    Slot            uint64    `ssz-index:"0"`
    ProposerIndex   uint64    `ssz-index:"1"`
    // Gap at indices 2-3
    ParentRoot      [32]byte  `ssz-index:"4"`
    StateRoot       [32]byte  `ssz-index:"5"`
    // Optional future fields can be added without breaking compatibility
}

func demonstrateProgressive() {
    block := &ProgressiveBlock{
        Slot:          12345,
        ProposerIndex: 42,
        ParentRoot:    [32]byte{1, 2, 3},
        StateRoot:     [32]byte{4, 5, 6},
    }
    
    ds := dynssz.NewDynSsz(nil)
    tree, _ := ds.GetTree(block)
    
    // Show tree structure - note the progressive layout
    tree.Show(4)
    
    // Progressive trees have different index mappings
    // Field indices depend on the progressive tree structure
}
```

### Multi-proof Generation

Generate efficient proofs for multiple fields:

```go
func generateMultiProof(tree *treeproof.Node) {
    // Prove multiple fields efficiently
    indices := []int{4, 5, 6, 7} // Multiple field indices
    
    multiproof, err := tree.ProveMulti(indices)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Multi-proof covers %d fields with %d shared hashes\n",
               len(multiproof.Indices), len(multiproof.Hashes))
    
    // Verify the multi-proof
    isValid, err := treeproof.VerifyMultiproof(
        tree.Hash(),
        multiproof.Hashes,
        multiproof.Leaves,
        multiproof.Indices,
    )
    
    fmt.Printf("Multi-proof valid: %v\n", isValid)
}
```

### Tree Structure Analysis

Debug and understand your tree structures:

```go
func analyzeTree(tree *treeproof.Node) {
    fmt.Println("=== Tree Structure Analysis ===")
    
    // Show complete structure
    tree.Show(0) // 0 = unlimited depth
    
    // Navigate to specific nodes
    node4, err := tree.Get(4)
    if err == nil {
        fmt.Printf("Node 4 value: %x\n", node4.Hash())
    }
    
    // Generate proof and show details
    proof, err := tree.Prove(4)
    if err == nil {
        fmt.Printf("Proof for index 4:\n")
        fmt.Printf("  Leaf: %x\n", proof.Leaf)
        for i, hash := range proof.Hashes {
            fmt.Printf("  Sibling %d: %x\n", i, hash)
        }
    }
}
```

## Performance Considerations

### Efficient Proof Generation

1. **Reuse Trees**: Build the tree once, generate multiple proofs
2. **Use Multi-proofs**: More efficient than multiple single proofs
3. **Cache Trees**: Store trees for repeated proof generation
4. **Progressive Types**: Use for sparse data structures

```go
// Good: Build once, use multiple times
tree, _ := ds.GetTree(data)
proof1, _ := tree.Prove(4)
proof2, _ := tree.Prove(5)
multiproof, _ := tree.ProveMulti([]int{6, 7, 8})

// Good: Multi-proof for batch operations
indices := []int{4, 5, 6, 7}
multiproof, _ := tree.ProveMulti(indices)

// Avoid: Rebuilding trees repeatedly
for _, index := range indices {
    tree, _ := ds.GetTree(data) // Inefficient!
    proof, _ := tree.Prove(index)
}
```

### Memory Management

```go
// For large trees, limit display depth
tree.Show(3) // Only show 3 levels

// Verify proofs without storing entire tree
root := tree.Hash()
// ... tree can be garbage collected
isValid, _ := treeproof.VerifyProof(root, proof)
```

## Integration Examples

### Ethereum Beacon Chain

```go
import "github.com/attestantio/go-eth2-client/spec/phase0"

func proveBeaconBlock(block *phase0.BeaconBlock) {
    // Setup dynamic SSZ with Ethereum specs
    specs := map[string]any{
        "SLOTS_PER_HISTORICAL_ROOT": 8192,
        "VALIDATOR_REGISTRY_LIMIT":  1099511627776,
        "MAX_PROPOSER_SLASHINGS":    16,
        "MAX_ATTESTER_SLASHINGS":    2,
        "MAX_ATTESTATIONS":          128,
        "MAX_DEPOSITS":              16,
        "MAX_VOLUNTARY_EXITS":       16,
    }
    ds := dynssz.NewDynSsz(specs)
    
    // Build tree for beacon block
    tree, err := ds.GetTree(block)
    if err != nil {
        panic(err)
    }
    
    // Prove specific fields (indices depend on BeaconBlock structure)
    // These are examples - actual indices depend on the struct layout
    slotProof, _ := tree.Prove(8)      // Slot field
    stateRootProof, _ := tree.Prove(11) // StateRoot field
    
    fmt.Printf("Generated proofs for beacon block at slot %d\n", block.Slot)
}
```

## Common Patterns

### Field Index Discovery

Since generalized indices depend on the tree structure, use the debug output to find field indices:

```go
func findFieldIndices(data any) {
    ds := dynssz.NewDynSsz(nil)
    tree, _ := ds.GetTree(data)
    
    // Show tree to see field mappings
    fmt.Println("Tree structure with field indices:")
    tree.Show(10) // Show enough levels to see all fields
    
    // Look for VALUE entries - these are your fields
    // The INDEX shown is the generalized index for that field
}
```

### Proof Serialization

```go
import "encoding/json"

func serializeProof(proof *treeproof.Proof) []byte {
    data, _ := json.Marshal(proof)
    return data
}

func deserializeProof(data []byte) *treeproof.Proof {
    var proof treeproof.Proof
    json.Unmarshal(data, &proof)
    return &proof
}
```

## Best Practices

1. **Tree Reuse**: Build trees once, generate multiple proofs
2. **Index Discovery**: Use `Show()` to understand field mappings
3. **Progressive Types**: Use for evolving data structures
4. **Multi-proofs**: Prefer for multiple field verification
5. **Verification**: Always verify proofs before trusting them
6. **Memory**: Limit debug output depth for large trees

## Troubleshooting

### Common Issues

**Problem**: "Node not found in tree" error
```go
// Check if index is valid for the tree structure
tree.Show(5) // Examine structure
// Ensure index corresponds to an actual node
```

**Problem**: Progressive tree indices are unexpected
```go
// Progressive trees have different structures than binary trees
// Use tree.Show() to see the actual index mappings
// Indices may be much larger than in binary trees
```

**Problem**: Proof verification fails
```go
// Ensure you're using the correct root hash
root := tree.Hash()
// Verify proof was generated from the same tree
isValid, err := treeproof.VerifyProof(root, proof)
```

See [Troubleshooting](troubleshooting.md) for more debugging techniques.

## Related Documentation

- **[Getting Started](getting-started.md)** - Basic Dynamic SSZ usage
- **[API Reference](api-reference.md)** - Complete API documentation
- **[SSZ Annotations](ssz-annotations.md)** - Progressive container syntax
- **[Performance Guide](performance.md)** - Optimization techniques
- **[Examples](../examples/)** - Comprehensive code examples