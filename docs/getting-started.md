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

### Dynamic Specifications

Dynamic SSZ supports runtime configuration through specifications. These specifications allow you to define dynamic field sizes that can adapt to different Ethereum presets (mainnet, minimal, custom).

### Struct Tags

The library uses struct tags to control encoding/decoding behavior:

- `ssz-size`: Defines static default field sizes (compatible with fastssz)
- `dynssz-size`: Specifies dynamic sizes based on specification properties

Example:
```go
type BeaconState struct {
    BlockRoots []phase0.Root `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
    StateRoots []phase0.Root `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
}
```

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

## Common Pitfalls

1. **Pointer handling**: Always pass pointers to UnmarshalSSZ
2. **Specification values**: Ensure your specs map contains all required values
3. **Type compatibility**: Verify struct tags match your specifications
4. **Buffer reuse**: Consider reusing buffers for better performance

## Getting Help

- Check the [troubleshooting guide](troubleshooting.md)
- Look at the [examples](../examples/) directory
- Review the [API documentation](api-reference.md)