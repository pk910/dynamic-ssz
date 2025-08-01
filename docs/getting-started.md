# Getting Started with Dynamic SSZ

Dynamic SSZ (`dynssz`) is a Go library that provides flexible SSZ (Simple Serialize) encoding and decoding for any Go data structures. While commonly used with Ethereum data structures, it works with any SSZ-compatible types and supports dynamic field sizing based on runtime specifications. This guide will help you get started with the library.

## Installation

Add the library to your Go project:

```bash
go get github.com/pk910/dynamic-ssz
```

## Quick Start

Here's a simple example to get you started:

```go
package main

import (
    "fmt"
    "log"
    
    dynssz "github.com/pk910/dynamic-ssz"
    "github.com/attestantio/go-eth2-client/spec/phase0"
)

func main() {
    // Create a DynSsz instance with mainnet specifications
    specs := map[string]any{
        "SLOTS_PER_HISTORICAL_ROOT": uint64(8192),
        "SYNC_COMMITTEE_SIZE":       uint64(512),
    }
    ds := dynssz.NewDynSsz(specs)
    
    // Create a sample beacon block header
    header := &phase0.BeaconBlockHeader{
        Slot:          12345,
        ProposerIndex: 42,
        ParentRoot:    [32]byte{1, 2, 3},
        StateRoot:     [32]byte{4, 5, 6},
        BodyRoot:      [32]byte{7, 8, 9},
    }
    
    // Encode to SSZ
    encoded, err := ds.MarshalSSZ(header)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Encoded %d bytes\n", len(encoded))
    
    // Decode from SSZ
    var decoded phase0.BeaconBlockHeader
    err = ds.UnmarshalSSZ(&decoded, encoded)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Decoded slot: %d\n", decoded.Slot)
    
    // Calculate hash tree root
    root, err := ds.HashTreeRoot(header)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Hash tree root: %x\n", root)
}
```

## Core Concepts

### Supported Types

Before diving into the details, it's important to understand which types are supported by Dynamic SSZ:

**✅ Supported:**
- Unsigned integers: `uint8`, `uint16`, `uint32`, `uint64`
- Booleans: `bool`
- Byte arrays: `[N]byte` (fixed-size)
- Slices and arrays of supported types
- Structs containing only supported types
- Pointers to structs (optional fields)

**❌ Not Supported:**
- Signed integers (`int`, `int8`, etc.)
- Floating-point numbers (`float32`, `float64`)
- Strings (`string`) - use `[]byte` instead
- Maps, channels, functions, interfaces

### Dynamic Specifications

Dynamic SSZ supports runtime configuration through specifications. These specifications allow you to define dynamic field sizes that can adapt to different Ethereum presets (mainnet, minimal, custom).

### Struct Tags

The library uses struct tags to control encoding/decoding behavior:

- `ssz-size`: Defines field sizes (compatible with fastssz). Use `?` for dynamic dimensions
- `dynssz-size`: Specifies sizes based on specification properties with expression support
- `ssz-max`: **Required** for dynamic fields to define maximum elements for hash tree root
- `dynssz-max`: Dynamic maximum based on specifications

Example:
```go
type BeaconState struct {
    // Fixed-size fields (no ssz-max needed)
    BlockRoots []phase0.Root `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
    StateRoots []phase0.Root `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
    
    // Dynamic fields (ssz-max required)
    HistoricalRoots []phase0.Root `ssz-size:"?,32" ssz-max:"16777216" dynssz-max:"HISTORICAL_ROOTS_LIMIT"`
    Validators      []Validator   `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}
```

**Important**: Every dynamic length field must have either an `ssz-max` or `dynssz-max` tag for proper hash tree root calculation.

### Hybrid Approach

Dynamic SSZ automatically chooses between:
- **Static processing**: Uses fastssz for types without dynamic specifications (faster)
- **Dynamic processing**: Uses reflection for types with dynamic specifications (flexible)

## Basic Operations

### Creating a DynSsz Instance

```go
specs := map[string]any{
    "SLOTS_PER_HISTORICAL_ROOT": uint64(8192),
    "SYNC_COMMITTEE_SIZE":       uint64(512),
    // Add more specifications as needed
}
ds := dynssz.NewDynSsz(specs)
```

### Encoding (Marshaling)

```go
// Marshal to SSZ bytes
data, err := ds.MarshalSSZ(myStruct)

// Marshal to existing buffer
buf := make([]byte, 0, 1024)
data, err := ds.MarshalSSZTo(myStruct, buf)

// Get SSZ size without encoding
size, err := ds.SizeSSZ(myStruct)
```

### Decoding (Unmarshaling)

```go
var target MyStruct
err := ds.UnmarshalSSZ(&target, sszData)
```

### Hash Tree Root

```go
root, err := ds.HashTreeRoot(myStruct)
```

## Configuration Options

The DynSsz instance supports several configuration options:

```go
ds := dynssz.NewDynSsz(specs)
ds.NoFastSsz = true   // Disable fastssz optimization
ds.NoFastHash = true  // Disable fast hashing
ds.Verbose = true     // Enable verbose logging
```

## Next Steps

- Read the [API Reference](api-reference.md) for detailed function documentation
- Learn about [go-eth2-client integration](go-eth2-client-integration.md)
- Explore [performance optimization](performance.md) techniques
- Check out [examples](../examples/) for more use cases

## Working with Custom Types

Here's a comprehensive example using custom types with various tag combinations:

```go
type CustomData struct {
    // Fixed-size fields
    ID         uint64
    Hash       [32]byte                    // Fixed array, no tags needed
    FixedData  []byte    `ssz-size:"256"`  // Fixed 256-byte slice
    
    // Dynamic fields (require ssz-max)
    Counts     []uint64  `ssz-max:"100"`                            // Max 100 counts
    Data       []byte    `ssz-max:"4096" dynssz-max:"MAX_DATA"`    // Dynamic max from spec
    
    // Multi-dimensional arrays
    Matrix     [][]byte  `ssz-size:"?,32" ssz-max:"64"`            // Dynamic outer, fixed inner
    Dynamic2D  [][]uint8 `ssz-size:"?,?" ssz-max:"100,256"`        // Fully dynamic
}

// Usage
specs := map[string]any{
    "MAX_DATA": uint64(8192),
}
ds := dynssz.NewDynSsz(specs)

data := &CustomData{
    ID:        1,
    Hash:      [32]byte{1, 2, 3},
    FixedData: make([]byte, 256),
    Counts:    []uint64{100, 200, 300},
    Data:      []byte("dynamic data"),
    Matrix:    [][]byte{{1, 2}, {3, 4}},
    Dynamic2D: [][]uint8{{1}, {2, 3}, {4, 5, 6}},
}

encoded, _ := ds.MarshalSSZ(data)
root, _ := ds.HashTreeRoot(data)
```

## Common Pitfalls

1. **Missing ssz-max tags**: Dynamic fields without `ssz-max` will fail hash tree root calculation
2. **Mixing size and max**: Don't use `ssz-max` on fixed-size fields (those with numeric `ssz-size`)
3. **Pointer handling**: Always pass pointers to UnmarshalSSZ
4. **Specification values**: Ensure your specs map contains all required values for dynamic sizing

## Getting Help

- Check the [troubleshooting guide](troubleshooting.md)
- Look at the [examples](../examples/) directory
- Review the [API documentation](api-reference.md)