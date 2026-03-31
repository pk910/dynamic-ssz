# Getting Started

Dynamic SSZ is a Go library for [SSZ](https://github.com/ethereum/consensus-specs/blob/master/ssz/simple-serialize.md) (Simple Serialize) encoding, decoding, and hash tree root computation. It supports runtime-configurable field sizes, making it suitable for Ethereum consensus types that vary across network presets.

## Installation

```bash
go get github.com/pk910/dynamic-ssz
```

For the code generation CLI tool:
```bash
go install github.com/pk910/dynamic-ssz/dynssz-gen@latest
```

## Quick Start

```go
package main

import (
    "fmt"
    dynssz "github.com/pk910/dynamic-ssz"
)

// Define a structure with SSZ annotations
type BeaconBlockHeader struct {
    Slot          uint64
    ProposerIndex uint64
    ParentRoot    [32]byte
    StateRoot     [32]byte
    BodyRoot      [32]byte
}

func main() {
    // Create a DynSsz instance (the central entry point)
    ds := dynssz.NewDynSsz(nil)

    header := &BeaconBlockHeader{
        Slot:          12345,
        ProposerIndex: 42,
    }

    // Serialize to SSZ
    data, err := ds.MarshalSSZ(header)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Encoded: %d bytes\n", len(data))

    // Deserialize from SSZ
    var decoded BeaconBlockHeader
    err = ds.UnmarshalSSZ(&decoded, data)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Decoded slot: %d\n", decoded.Slot)

    // Compute hash tree root
    root, err := ds.HashTreeRoot(header)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Root: %x\n", root)
}
```

## Core Concepts

### The DynSsz Instance

All SSZ operations go through a `DynSsz` instance. Create one with `NewDynSsz()`, passing optional specification values and options:

```go
// Basic instance (no dynamic specs)
ds := dynssz.NewDynSsz(nil)

// With spec values for dynamic field sizes
specs := map[string]any{
    "VALIDATOR_REGISTRY_LIMIT": uint64(1099511627776),
    "MAX_ATTESTATIONS":         uint64(128),
}
ds := dynssz.NewDynSsz(specs)

// With options
ds := dynssz.NewDynSsz(specs,
    dynssz.WithExtendedTypes(),  // enable non-standard types
    dynssz.WithVerbose(),        // enable debug logging
)
```

Reuse the same `DynSsz` instance across your application - it caches type information for performance.

### Struct Tags

SSZ encoding is controlled through Go struct tags:

- `ssz-size:"N"` - Fixed size for byte slices and strings (makes them SSZ vectors)
- `ssz-max:"N"` - Maximum size for dynamic lists (required for hash tree root security)
- `ssz-type:"T"` - Explicit SSZ type (e.g., `"bitvector"`, `"uint256"`, `"progressive-list"`)
- `ssz-bitsize:"N"` - Bit-level size for bitvectors (enables padding validation)

```go
type Example struct {
    Hash   []byte    `ssz-size:"32"`       // fixed 32-byte vector
    Items  []uint64  `ssz-max:"1024"`      // list with max 1024 elements
    Bits   [8]byte   `ssz-type:"bitvector"` // bitvector
}
```

### Dynamic Expressions

The `dynssz-*` tags reference spec values that are resolved at runtime. They work alongside the static `ssz-*` tags:

```go
type State struct {
    // ssz-max provides the static fallback, dynssz-max overrides it at runtime
    Validators []Validator `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}
```

When a `dynssz-*` expression resolves successfully, it overrides the corresponding `ssz-*` value. If the expression cannot be resolved (e.g., the spec value was not provided), the `ssz-*` value is used as a fallback. This lets the same type definitions work across different Ethereum presets.

Expressions support arithmetic operators: `dynssz-max:"MAX_COMMITTEES_PER_SLOT*SLOTS_PER_EPOCH"`

See [SSZ Annotations](ssz-annotations.md) for the complete tag reference.

## Basic Operations

### Size Calculation

```go
size, err := ds.SizeSSZ(header)
if err != nil {
    panic(err)
}
fmt.Printf("Serialized size: %d bytes\n", size)
```

### Buffer Reuse

For performance-critical paths, reuse a buffer across multiple operations:

```go
buf := make([]byte, 0, 1024)

for _, block := range blocks {
    buf, err = ds.MarshalSSZTo(block, buf[:0])
    if err != nil {
        panic(err)
    }
    // process buf...
}
```

### Type Validation

```go
err := ds.ValidateType(reflect.TypeOf(MyStruct{}))
if err != nil {
    fmt.Printf("Type validation failed: %v\n", err)
}
```

## Working with Collections

### Fixed Arrays (Vectors)

```go
type Data struct {
    Values [10]uint32  // vector of 10 elements
}
```

### Dynamic Lists

Lists require `ssz-max` (or `dynssz-max`) for hash tree root security:

```go
type Data struct {
    Items []uint64 `ssz-max:"100"`
}
```

### Byte Slices and Strings

Byte slices and strings are variable-length by default and need `ssz-size` or `ssz-max`:

```go
type Data struct {
    Hash    []byte `ssz-size:"32"`   // fixed 32 bytes (vector)
    Payload []byte `ssz-max:"2048"`  // variable up to 2048 bytes (list)
    Name    string `ssz-size:"64"`   // fixed 64 bytes, null-padded
}
```

### Bitvectors and Bitlists

```go
type Flags struct {
    FixedBits   [32]byte `ssz-type:"bitvector"`              // 256-bit bitvector
    DynamicBits []byte   `ssz-type:"bitlist" ssz-max:"2048"` // bitlist, max 2048 bits
}
```

For bitlists, `ssz-max` specifies the maximum number of **bits**, not bytes. This matches the SSZ specification.

## Common Patterns

### Nested Structures

```go
type Header struct {
    Slot      uint64
    StateRoot [32]byte
}

type Block struct {
    Header  Header
    Body    *BlockBody  // pointers are followed; nil pointers are initialized on unmarshal
}
```

### Using uint256

```go
import "github.com/holiman/uint256"

type Account struct {
    Balance *uint256.Int  // automatically detected as uint256
}
```

## Next Steps

- [Supported Types](supported-types.md) - Complete type reference
- [SSZ Annotations](ssz-annotations.md) - All struct tags and dynamic expressions
- [Code Generation](code-generator.md) - Performance optimization
- [API Reference](api-reference.md) - Full public interface

## Examples

See the [examples/](../examples/) directory:
- `basic/` - Encoding/decoding with go-eth2-client types
- `codegen/` - Code generation setup
- `custom-types/` - Dynamic expressions and spec values
- `versioned-blocks/` - Ethereum fork handling
- `progressive-merkleization/` - EIP-7916/7495 features
