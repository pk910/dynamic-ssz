# Dynamic SSZ Documentation

## Getting Started

- **[Getting Started](getting-started.md)** - Installation, quick start, and core concepts
- **[API Reference](api-reference.md)** - Complete public interface documentation

## Core Guides

- **[Supported Types](supported-types.md)** - Type system reference (basic, collection, progressive, extended)
- **[SSZ Annotations](ssz-annotations.md)** - Struct tags, dynamic expressions, and tag combinations
- **[Code Generator](code-generator.md)** - CLI tool and programmatic code generation
- **[SSZ Views](views.md)** - Multiple SSZ schemas for fork handling
- **[Merkle Proofs](merkle-proofs.md)** - Tree construction and proof generation
- **[Streaming Support](streaming.md)** - Memory-efficient encoding/decoding via `io.Reader`/`io.Writer`

## Advanced Topics

- **[Extended Types](extended-types.md)** - Non-standard type extensions (signed ints, floats, big.Int, optionals)
- **[Type Wrapper](type-wrapper.md)** - Applying SSZ annotations to non-struct types
- **[Performance Guide](performance.md)** - Optimization techniques and benchmarks
- **[Ethereum Integration](go-eth2-client-integration.md)** - Working with go-eth2-client types
- **[Troubleshooting](troubleshooting.md)** - Common issues and debugging

## Examples

Comprehensive examples in the [examples/](../examples/) directory:

- **[Basic Usage](../examples/basic/)** - Simple encoding/decoding with go-eth2-client types
- **[Code Generation](../examples/codegen/)** - Programmatic code generation setup
- **[Custom Types](../examples/custom-types/)** - Dynamic expressions and spec values
- **[Progressive Merkleization](../examples/progressive-merkleization/)** - EIP-7916/7495 features
- **[Versioned Blocks](../examples/versioned-blocks/)** - Ethereum fork handling
