# Dynamic SSZ Documentation

Welcome to the comprehensive documentation for the dynamic-ssz library. This documentation provides everything you need to get started with and master the dynamic SSZ encoder/decoder.

## Quick Navigation

### Getting Started
- **[Getting Started Guide](getting-started.md)** - Start here if you're new to dynamic-ssz
- **[API Reference](api-reference.md)** - Complete API documentation with examples
- **[Troubleshooting](troubleshooting.md)** - Common issues and solutions

### Advanced Usage
- **[Performance Guide](performance.md)** - Optimization techniques and best practices
- **[go-eth2-client Integration](go-eth2-client-integration.md)** - Ethereum-specific integration patterns

### Examples
- **[Basic Usage](../examples/basic/)** - Simple encoding/decoding examples
- **[Custom Types](../examples/custom-types/)** - Non-Ethereum data structures
- **[Versioned Blocks](../examples/versioned-blocks/)** - Ethereum fork handling patterns

### Testing
- **[Spec Tests](../spectests/)** - Ethereum consensus specification compliance tests

## Overview

Dynamic SSZ is a Go library that provides flexible SSZ (Simple Serialize) encoding and decoding for any Go data structures. Key features include:

### Core Features
- **Universal Compatibility**: Works with any SSZ-compatible Go types, not just Ethereum structures
- **Dynamic Sizing**: Runtime field size configuration through specifications
- **Hybrid Performance**: Automatically uses fastssz for static types, reflection for dynamic types
- **Type Caching**: Optimizes repeated operations through intelligent caching
- **Thread Safety**: Safe for concurrent use across multiple goroutines

### Use Cases
- **Ethereum Development**: Beacon chain data structures, custom presets
- **General SSZ**: Any application requiring SSZ serialization
- **Performance Critical**: Applications needing optimized encoding/decoding
- **Multi-Environment**: Different configurations for dev/test/prod environments

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Your Types    │──▶│   Dynamic SSZ    │──▶│  SSZ Encoding   │
│                 │    │                  │    │                 │
│ • Structs       │    │ • Type Cache     │    │ • Bytes         │
│ • Arrays        │    │ • Spec Values    │    │ • Hash Roots    │
│ • Slices        │    │ • Reflection     │    │ • Size Calc     │
│ • Basic Types   │    │ • FastSSZ        │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

## Quick Start

```go
package main

import (
    "fmt"
    "log"
    dynssz "github.com/pk910/dynamic-ssz"
)

func main() {
    // Define specifications
    specs := map[string]any{
        "MAX_ITEMS": uint64(1000),
        "BUFFER_SIZE": uint64(4096),
    }
    
    // Create encoder/decoder
    ds := dynssz.NewDynSsz(specs)
    
    // Your data structure
    type MyData struct {
        ID    uint64
        Items []byte `ssz-max:"2048" dynssz-max:"MAX_ITEMS"`
    }
    
    data := &MyData{
        ID:    12345,
        Items: []byte("Hello, SSZ!"),
    }
    
    // Encode
    encoded, err := ds.MarshalSSZ(data)
    if err != nil {
        log.Fatal(err)
    }
    
    // Decode
    var decoded MyData
    err = ds.UnmarshalSSZ(&decoded, encoded)
    if err != nil {
        log.Fatal(err)
    }
    
    // Hash tree root
    root, err := ds.HashTreeRoot(data)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Encoded %d bytes, root: %x\n", len(encoded), root)
}
```

## Key Concepts

### Supported Types
Dynamic SSZ only supports SSZ-compatible types:
- **Base types**: `uint8`, `uint16`, `uint32`, `uint64`, `bool`, fixed byte arrays
- **Composite types**: Arrays, slices, structs, pointers (optional fields)
- **Not supported**: Signed integers, floats, strings, maps, channels, interfaces

### Struct Tags
- **`ssz-size`**: Size hints for fields (fastssz compatible). Use `?` for dynamic dimensions
- **`dynssz-size`**: Dynamic size based on specifications with expression support
- **`ssz-max`**: Maximum elements for dynamic fields (**required** for hash tree root)
- **`dynssz-max`**: Dynamic maximum based on specifications

### Specifications
Runtime configuration that controls dynamic field sizes:
```go
specs := map[string]any{
    "MAX_ITEMS":     uint64(1000),
    "BUFFER_SIZE":   uint64(4096),
    "CUSTOM_LENGTH": uint64(256),
}
```

### Hybrid Processing
- **Static types**: Automatically uses fastssz (fastest)
- **Dynamic types**: Uses reflection (flexible)
- **Automatic selection**: Based on presence of dynamic specifications

## Best Practices

1. **Reuse instances**: Create one DynSsz instance and reuse it
2. **Pre-allocate buffers**: Use `MarshalSSZTo` with reused buffers
3. **Consistent specifications**: Use same specs for marshal/unmarshal
4. **Monitor performance**: Profile your application for optimization opportunities
5. **Handle errors**: Always check and handle encoding/decoding errors

## Support and Community

- **Examples**: Comprehensive examples in [examples/](../examples/) directory
- **Issues**: Report issues with minimal reproducible examples
- **Performance**: Follow the [performance guide](performance.md) for optimization
- **Integration**: See [integration guides](go-eth2-client-integration.md) for common patterns

## License

This library is licensed under the Apache-2.0 License. See the LICENSE file for more details.