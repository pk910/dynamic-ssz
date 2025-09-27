# Strict Type System

Dynamic SSZ (`dynssz`) provides a strict type system that allows explicit type specification through `ssz-type` annotations. This feature gives developers precise control over how their Go types are interpreted and encoded in SSZ format.

## Overview

The strict type system addresses scenarios where:
- Go's type system doesn't map directly to SSZ types
- You need to distinguish between similar data structures (e.g., bitlists vs byte slices)
- You want to use specific SSZ types regardless of the Go implementation
- You need to handle large integer types (uint128, uint256) that Go doesn't natively support

## The `ssz-type` Tag

The `ssz-type` struct tag allows you to explicitly specify the SSZ type for any field:

```go
type MyStruct struct {
    // Explicitly specify this byte slice as a bitlist
    Flags []byte `ssz-type:"bitlist" ssz-max:"256"`
    
    // Force interpretation as uint256
    Balance []byte `ssz-type:"uint256"`
    
    // Let the library auto-detect the type
    Data []byte `ssz-type:"auto" ssz-max:"1024"`
}
```

## Supported Type Annotations

### Basic Types

| Annotation | SSZ Type | Valid Go Types |
|------------|----------|----------------|
| `"bool"` | Bool | `bool` |
| `"uint8"` | Uint8 | `uint8` |
| `"uint16"` | Uint16 | `uint16` |
| `"uint32"` | Uint32 | `uint32` |
| `"uint64"` | Uint64 | `uint64` |
| `"uint128"` | Uint128 | `[16]byte`, `[2]uint64`, `[]byte`, `[]uint64` |
| `"uint256"` | Uint256 | `[32]byte`, `[4]uint64`, `[]byte`, `[]uint64` |

### Composite Types

| Annotation | SSZ Type | Description |
|------------|----------|-------------|
| `"container"` | Container | Struct types |
| `"list"` | List | Dynamic-length sequences |
| `"vector"` | Vector | Fixed-length sequences |
| `"bitlist"` | Bitlist | Dynamic-length bit sequences |
| `"bitvector"` | Bitvector | Fixed-length bit sequences |
| `"progressive-container"` | Progressive Container | Struct types with progressive merkleization and active field tracking (EIP-7495) |
| `"progressive-list"` | Progressive List | Dynamic-length sequences with progressive merkleization (EIP-7916) |
| `"progressive-bitlist"` | Progressive Bitlist | Dynamic-length bit sequences with progressive merkleization (EIP-7916) |
| `"compatible-union"` | Compatible Union | Union type with variant selection (EIP-7495) |


### Special Annotations

| Annotation | Behavior |
|------------|----------|
| `"?"` or `"auto"` | Use automatic type detection |
| `"custom"` | Type implements fastssz interfaces |

## Large Integer Support

Dynamic SSZ provides special handling for uint128 and uint256 types, which are commonly used in blockchain applications but not natively supported by Go.

### uint128 Support

```go
type Account struct {
    // As byte array (16 bytes)
    Balance128 [16]byte `ssz-type:"uint128"`
    
    // As uint64 array (2 elements)
    Amount [2]uint64 `ssz-type:"uint128"`
    
    // As slice with size hint
    Value []byte `ssz-type:"uint128" ssz-size:"16"`
}
```

### uint256 Support

```go
type Token struct {
    // As byte array (32 bytes)
    TotalSupply [32]byte `ssz-type:"uint256"`
    
    // As uint64 array (4 elements)
    MaxSupply [4]uint64 `ssz-type:"uint256"`
    
    // Automatically detected uint256.Int
    Balance uint256.Int  // No tag needed, auto-detected
}
```

## Automatic Type Detection

When no `ssz-type` tag is specified, dynamic SSZ automatically detects types based on:

1. **Well-Known Types**: Recognizes common implementations like `github.com/holiman/uint256.Int`
2. **Go Type Mapping**: Maps Go's built-in types to their SSZ equivalents

### Default Type Mappings

| Go Type | SSZ Type | Notes |
|---------|----------|-------|
| `bool` | Bool | |
| `uint8` | Uint8 | |
| `uint16` | Uint16 | |
| `uint32` | Uint32 | |
| `uint64` | Uint64 | |
| `[N]T` | Vector | Fixed-size array |
| `[]T` | List | Dynamic slice (requires `ssz-max`) |
| `string` | List of uint8 | Treated as byte slice |
| `struct` | Container | |
| `*struct` | Container | Pointer to struct |

### Special Type Detection

Dynamic SSZ automatically recognizes certain third-party types:
- `github.com/holiman/uint256.Int` → Detected as `uint256`

## Bitlist and Bitvector Support

Bitlists and bitvectors require explicit type annotation since they cannot be distinguished from regular byte arrays through reflection alone:

```go
type Validators struct {
    // Dynamic bitlist with max 2048 bits
    ActiveFlags []byte `ssz-type:"bitlist" ssz-max:"2048"`
    
    // Fixed bitvector of 64 bits
    CommitteeFlags [8]byte `ssz-type:"bitvector"`
    
    // Regular byte slice (not a bitlist)
    Signatures []byte `ssz-max:"96000"`
}
```

## Type Validation

The library performs strict validation to ensure type annotations match the actual Go types:

- **Size Validation**: For fixed-size types, the Go type must match the expected size
- **Kind Validation**: The Go type must be compatible with the specified SSZ type
- **Limit Validation**: Dynamic types must have appropriate `ssz-max` tags

### Example Validation Errors

```go
// ERROR: uint256 requires 32 bytes, not 16
Field1 [16]byte `ssz-type:"uint256"`

// ERROR: bitlist must be byte slice or array
Field2 []uint32 `ssz-type:"bitlist"`

// ERROR: container type requires struct
Field3 []byte `ssz-type:"container"`
```

## Multi-dimensional Support

Type annotations work with multi-dimensional slices:

```go
type Matrix struct {
    // 2D byte array where inner arrays are uint256
    Values [][]byte `ssz-type:"list,uint256" ssz-size:"?,32" ssz-max:"100"`
    
    // 2D bitvector array
    Flags [][4]byte `ssz-type:"list,bitvector" ssz-max:"64"`
}
```

## Best Practices

1. **Use Type Annotations When Ambiguous**: Always specify `ssz-type` for bitlists/bitvectors
2. **Leverage Auto-Detection**: For standard types, let the library detect automatically
3. **Validate Early**: Test your type definitions with actual encoding/decoding
4. **Document Intent**: Use type annotations to make your SSZ schema explicit

## Examples

### Complete Example

```go
package main

import (
    "fmt"
    dynssz "github.com/pk910/dynamic-ssz"
    "github.com/holiman/uint256"
)

type Transaction struct {
    // Explicit uint256 as byte array
    Value [32]byte `ssz-type:"uint256"`
    
    // Auto-detected uint256.Int
    GasPrice uint256.Int
    
    // Bitlist for feature flags
    Features []byte `ssz-type:"bitlist" ssz-max:"256"`
    
    // Regular byte data
    Data []byte `ssz-max:"1000000"`
    
    // Container (struct)
    Receipt *Receipt `ssz-type:"container"`
}

type Receipt struct {
    Status    bool
    GasUsed   uint64
    LogsBloom [256]byte `ssz-type:"bitvector"`
}

func main() {
    ds := dynssz.NewDynSsz(nil)
    
    tx := &Transaction{
        Value:    [32]byte{1, 2, 3},
        GasPrice: *uint256.NewInt(1000000),
        Features: []byte{0xff, 0x01}, // bitlist with 8 bits
        Data:     []byte("transaction data"),
        Receipt: &Receipt{
            Status:  true,
            GasUsed: 50000,
        },
    }
    
    // Encode
    encoded, err := ds.MarshalSSZ(tx)
    if err != nil {
        panic(err)
    }
    
    // The type annotations ensure proper SSZ encoding
    fmt.Printf("Encoded transaction: %d bytes\n", len(encoded))
}
```

### Integration with Ethereum Types

```go
type BeaconState struct {
    // Ethereum uses uint256 for balances
    Balances []uint64 `ssz-type:"list" ssz-max:"1099511627776"`
    
    // Participation flags as bitlist
    PreviousEpochParticipation []byte `ssz-type:"bitlist" ssz-max:"1099511627776"`
    
    // Fixed-size roots
    BlockRoots [][32]byte `ssz-type:"vector" ssz-size:"8192"`
    
    // Explicitly marked container
    LatestExecutionPayloadHeader *ExecutionPayloadHeader `ssz-type:"container"`
}
```

## See Also

- [Getting Started](getting-started.md) - Basic usage of dynamic SSZ
- [API Reference](api-reference.md) - Complete API documentation
- [Performance](performance.md) - Performance considerations with strict types