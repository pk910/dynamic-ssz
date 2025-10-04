# Getting Started

Dynamic SSZ is a flexible Go implementation of the Simple Serialize (SSZ) encoding standard used in Ethereum. This guide will help you get started with the library.

## Installation

```bash
go get github.com/pk910/dynamic-ssz
```

For code generation tool:
```bash
go install github.com/pk910/dynamic-ssz/dynssz-gen@latest
```

## Quick Start

### Basic Serialization and Deserialization

```go
package main

import (
    "fmt"
    dynssz "github.com/pk910/dynamic-ssz"
)

// Define a simple structure
type Person struct {
    Name    string
    Age     uint64
    Active  bool
}

func main() {
    // Create an instance
    person := &Person{
        Name:   "Alice",
        Age:    30,
        Active: true,
    }
    
    // Create DynSSZ instance
    ds := dynssz.NewDynSsz(nil)
    
    // Serialize
    encoded, err := ds.MarshalSSZ(person)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Encoded: %x\n", encoded)
    
    // Deserialize
    decoded := &Person{}
    err = ds.UnmarshalSSZ(decoded, encoded)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Decoded: %+v\n", decoded)
}
```

### Computing Hash Tree Root

```go
// Compute the hash tree root
root, err := ds.HashTreeRoot(person)
if err != nil {
    panic(err)
}
fmt.Printf("Root: %x\n", root)
```

### Using Specification Values

Dynamic SSZ supports runtime specification values for dynamic sizing:

```go
// Define structure with dynamic sizing
type Block struct {
    Slot        uint64
    Validators  []Validator `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}

// Create DynSSZ with specification values
specs := map[string]interface{}{
    "VALIDATOR_REGISTRY_LIMIT": 1099511627776, // 2^40
}
ds := dynssz.NewDynSsz(specs)

// Now you can marshal/unmarshal blocks with dynamic validator limits
```

## Basic Operations

### Size Calculation

Calculate the serialized size of an object:

```go
size, err := ds.SizeSSZ(person)
if err != nil {
    panic(err)
}
fmt.Printf("Serialized size: %d bytes\n", size)
```

### Type Validation

Validate that a type can be serialized:

```go
err := ds.ValidateType(reflect.TypeOf(Person{}))
if err != nil {
    fmt.Printf("Type validation failed: %v\n", err)
}
```

### Buffer Reuse

For performance-critical applications, reuse buffers:

```go
buf := make([]byte, 0, 1024)
encoded, err := ds.MarshalSSZTo(person, buf)
if err != nil {
    panic(err)
}
```

## Core Concepts

### SSZ Encoding

SSZ (Simple Serialize) is a serialization format designed for deterministic encoding of data structures. Key features:
- Fixed-size types are encoded in-place
- Variable-size types use offset tables
- All integers are little-endian
- Merkle tree hashing for efficient proofs

### Type System

Dynamic SSZ automatically detects and handles:
- Basic types: `bool`, `uint8`, `uint16`, `uint32`, `uint64`
- Large integers: `uint128`, `uint256` (via uint256.Int, [n]byte or [n]uint64)
- Collections: arrays, slices, bitvectors, bitlists
- Complex types: structs, pointers

### Struct Tags

Control serialization behavior with tags:
- `ssz-size`: Fixed size for strings/byte arrays
- `ssz-max`: Maximum size for dynamic arrays
- `ssz-type`: Explicit type specification

Example:
```go
type Example struct {
    FixedBytes  []byte    `ssz-size:"32"`
    DynamicList []uint64  `ssz-max:"1024"`
    CustomType  [4]uint64 `ssz-type:"uint256"`
}
```

## Working with Collections

### Fixed-Size Arrays

```go
type Data struct {
    Values [10]uint32  // Fixed array of 10 elements
}
```

### Dynamic Arrays (Lists)

```go
type Data struct {
    Items []uint64 `ssz-max:"100"`
}
```

### Bitvectors and Bitlists

```go
type Flags struct {
    FixedBits   [256]byte `ssz-type:"bitvector"` // Bitvector (fixed size)
    DynamicBits []byte    `ssz-type:"bitlist"`   // Bitlist (variable size)
}
```

## Common Patterns

### Nested Structures

```go
type Header struct {
    Version uint8
    Length  uint32
}

type Message struct {
    Header  Header
    Payload []byte `ssz-max:"1024"`
}
```

### Working with uint256

```go
import "github.com/holiman/uint256"

type Account struct {
    Balance *uint256.Int `ssz-type:"uint256"`
}
```

## Next Steps

- Explore [Supported Types](supported-types.md) for complete type reference
- Learn about [SSZ Annotations](ssz-annotations.md) for advanced field control
- Check the [API Reference](api-reference.md) for all available methods
- Generate [Merkle Proofs](merkle-proofs.md) for data verification
- Use the [Code Generator](code-generator.md) for optimal performance

## Example Projects

See the `examples/` directory for complete examples:
- `basic/` - Simple serialization examples
- `ethereum-types/` - Ethereum consensus types
- `progressive-merkleization/` - Advanced merkleization features