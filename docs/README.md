# Dynamic SSZ Documentation

Welcome to the comprehensive documentation for Dynamic SSZ - a flexible Go implementation of Simple Serialize (SSZ) encoding.

## Core Documentation

### Essential Guides
- **[Getting Started](getting-started.md)** - Installation, quick start, and basic operations
- **[Supported Types](supported-types.md)** - Complete type system reference including progressive types
- **[SSZ Annotations](ssz-annotations.md)** - All struct tags and their usage
- **[API Reference](api-reference.md)** - Complete public interface documentation
- **[Merkle Proofs](merkle-proofs.md)** - Tree construction and proof generation
- **[Code Generator](code-generator.md)** - CLI tool and programmatic code generation

### Advanced Topics
- **[Type Wrapper](type-wrapper.md)** - Applying SSZ annotations to non-struct types
- **[Ethereum Integration](go-eth2-client-integration.md)** - Working with Ethereum types
- **[Performance Guide](performance.md)** - Optimization techniques and benchmarks
- **[Troubleshooting](troubleshooting.md)** - Common issues and debugging

## Quick Navigation

### By Use Case
- **New Users**: Start with [Getting Started](getting-started.md)
- **Ethereum Developers**: See [Ethereum Integration](go-eth2-client-integration.md)
- **Performance Optimization**: Read [Performance Guide](performance.md)
- **Type Issues**: Check [Troubleshooting](troubleshooting.md)
- **Advanced Types**: Explore [Type Wrapper](type-wrapper.md)

### By Topic
- **Types & Annotations**: [Supported Types](supported-types.md) → [SSZ Annotations](ssz-annotations.md)
- **API Usage**: [Getting Started](getting-started.md) → [API Reference](api-reference.md)
- **Proofs & Trees**: [Merkle Proofs](merkle-proofs.md)
- **Code Generation**: [Code Generator](code-generator.md)
- **Integration**: [Ethereum Integration](go-eth2-client-integration.md)

## Overview

Dynamic SSZ provides flexible SSZ encoding/decoding for Go applications with these key features:

### Core Capabilities
- **Universal Compatibility**: Works with any SSZ-compatible Go types
- **Dynamic Sizing**: Runtime field size configuration through specifications
- **Progressive Types**: EIP-7916/7495 progressive merkleization and containers
- **Merkle Proofs**: Complete tree construction and proof generation
- **Hybrid Performance**: Automatically optimizes with fastssz when possible
- **Type Safety**: Comprehensive validation and error handling

### Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Your Types    │──▶│   Dynamic SSZ    │──▶│  SSZ Encoding   │
│                 │    │                  │    │                 │
│ • Structs       │    │ • Type Cache     │    │ • Serialization │
│ • Annotations   │    │ • Spec Values    │    │ • Hash Roots    │
│ • Collections   │    │ • Type System    │    │ • Validation    │
│ • Custom Types  │    │ • Code Gen       │    │ • Performance   │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

## Quick Example

```go
package main

import (
    "fmt"
    dynssz "github.com/pk910/dynamic-ssz"
)

type Block struct {
    Slot        uint64
    StateRoot   [32]byte
    Validators  []Validator `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}

func main() {
    // Setup with runtime specifications
    specs := map[string]interface{}{
        "VALIDATOR_REGISTRY_LIMIT": 1099511627776, // 2^40
    }
    ssz := dynssz.NewDynSsz(specs)
    
    block := &Block{Slot: 12345, /* ... */}
    
    // Serialize
    data, err := ssz.MarshalSSZ(block)
    if err != nil {
        panic(err)
    }
    
    // Hash tree root
    root, err := ssz.HashTreeRoot(block)
    if err != nil {
        panic(err)
    }
    
    // Generate Merkle tree and proofs
    tree, err := ssz.GetTree(block)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Encoded: %d bytes, Root: %x\n", len(data), root)
}
```

## Examples

Comprehensive examples in the [examples/](../examples/) directory:

- **[Basic Usage](../examples/basic/)** - Simple encoding/decoding
- **[Code Generation](../examples/codegen/)** - Performance optimization
- **[Custom Types](../examples/custom-types/)** - Advanced type patterns
- **[Progressive Merkleization](../examples/progressive-merkleization/)** - EIP-7916/7495 features
- **[Versioned Blocks](../examples/versioned-blocks/)** - Ethereum fork handling

## Key Features Detail

### Type System
- **Basic Types**: `bool`, `uint8-64`, fixed arrays, slices
- **Advanced Types**: `uint128/256`, bitfields, progressive containers
- **Custom Types**: Implement marshaling interfaces for any type
- **Type Wrapper**: Apply annotations to non-struct types

### Annotations
- **Size Control**: `ssz-size`, `dynssz-size` for fixed/dynamic sizing
- **Maximum Limits**: `ssz-max`, `dynssz-max` for dynamic arrays
- **Type Specification**: `ssz-type` for explicit type control
- **Progressive Fields**: `ssz-index` for forward-compatible containers

### Performance Features
- **Hybrid Processing**: FastSSZ integration + reflection flexibility
- **Type Caching**: Automatic optimization for repeated operations
- **Buffer Reuse**: Efficient memory management patterns
- **Code Generation**: Static generation for maximum performance

### Progressive Types (EIP-7916 & EIP-7495)
- **Progressive Lists**: Efficient merkleization for growing collections
- **Progressive Bitlists**: Optimized participation tracking
- **Progressive Containers**: Forward-compatible struct evolution
- **Compatible Unions**: Type-safe variant types

## Best Practices

1. **Reuse Instances**: Create one DynSsz instance per specification set
2. **Use Code Generation**: Generate static methods for critical paths
3. **Progressive Types**: Use for large, growing data structures
4. **Buffer Management**: Reuse buffers with `MarshalSSZTo`
5. **Specification Consistency**: Keep specs consistent across operations

## Testing & Compliance

- **[Spec Tests](../spectests/)** - Ethereum consensus specification compliance
- **Round-trip Testing**: Comprehensive marshal/unmarshal validation
- **Performance Benchmarks**: Compare with other SSZ implementations
- **Fork Compatibility**: Tested across all Ethereum fork versions

## Community & Support

- **GitHub Issues**: Report bugs with reproducible examples
- **Performance Questions**: Use the [Performance Guide](performance.md)
- **Integration Help**: Check the relevant integration guides
- **Examples First**: Review [examples/](../examples/) for patterns

This documentation covers everything from basic usage to advanced optimization techniques. Start with [Getting Started](getting-started.md) and explore the topics most relevant to your use case.