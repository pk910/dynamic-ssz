# SSZ Annotations

Dynamic SSZ uses struct tags to control serialization behavior. This guide provides comprehensive documentation for all available annotations.

## Tag Overview

| Tag | Purpose | Example |
|-----|---------|---------|
| `ssz-size` | Fixed size for bytes/strings | `ssz-size:"32"` |
| `ssz-max` | Maximum size for dynamic types | `ssz-max:"1024"` |
| `ssz-type` | Explicit type specification | `ssz-type:"uint256"` |
| `ssz-bitsize` | Fixed bit size for bitvectors | `ssz-bitsize:"12"` |
| `dynssz-size` | Dynamic size with expressions | `dynssz-size:"EPOCH_LENGTH*32"` |
| `dynssz-max` | Dynamic maximum with expressions | `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"` |
| `dynssz-bitsize` | Dynamic bit size for bitvectors | `dynssz-bitsize:"COMMITTEE_SIZE"` |
| `ssz-index` | Field index for progressive containers | `ssz-index:"0"` |

## Size Annotations

### Multi-dimensional Syntax

All size and maximum annotations support multi-dimensional syntax for nested arrays:

**Syntax**: Comma-separated values for each dimension
- Applied left-to-right: `"100,256"` means max 100 rows, max 256 columns per row
- **Available for**: `ssz-size`, `ssz-max`, `dynssz-size`, `dynssz-max`

**Examples**:
```go
type MultiDimensional struct {
    // Fixed 2D array: 8 rows, 16 bytes each
    Grid [][]byte `ssz-size:"8,16"`
    
    // Dynamic 2D array: max 100 rows, max 256 elements per row
    Matrix [][]uint32 `ssz-max:"100,256"`
    
    // Dynamic with expressions
    Data [][]byte `dynssz-size:"ROWS,COLS"`
    Shards [][]Transaction `dynssz-max:"SHARD_COUNT,MAX_TRANSACTIONS_PER_SHARD"`
}
```

### ssz-size

Specifies fixed size for variable-length types (strings, byte slices).

**Syntax**: `ssz-size:"<number>"`

**Applies to**:
- `string`
- `[]byte`

**Examples**:
```go
type Data struct {
    // Fixed 32-byte hash
    Hash []byte `ssz-size:"32"`
    
    // Fixed-length string (padded with zeros)
    Name string `ssz-size:"64"`
    
    // Multi-dimensional fixed size
    Grid [][]byte `ssz-size:"8,16"`  // 8 rows, 16 bytes each
}
```

**Important notes**:
- Strings are null-padded if shorter than specified size
- Byte slices must be exactly the specified size
- Cannot be used with `ssz-max` on same field

### dynssz-size

Dynamic size specification using runtime expressions.

**Syntax**: `dynssz-size:"<expression>"`

**Applies to**: Same as `ssz-size`

**Expression features**:
- Arithmetic operators: `+`, `-`, `*`, `/`
- Parentheses for grouping
- Spec value references
- Automatic rounding up for partial bytes

**Examples**:
```go
type Dynamic struct {
    // Size based on spec value
    Data []byte `dynssz-size:"CHUNK_SIZE"`
    
    // Expression with multiple values
    Buffer []byte `dynssz-size:"SLOT_SIZE*EPOCH_LENGTH"`
    
    // Complex expression
    Payload []byte `dynssz-size:"(BASE_SIZE+EXTENSION_SIZE)*8"`
    
    // Multi-dimensional
    Matrix [][]byte `dynssz-size:"ROWS,COLS"`
}
```

## Bit Size Annotations (Bitvectors)

The `ssz-bitsize` and `dynssz-bitsize` annotations specify the size in **bits** rather than bytes. These are specifically designed for bitvectors where the exact bit count matters.

**Why use bitsize?** When a bitvector's bit count is not a multiple of 8, the remaining bits in the last byte are padding bits. According to the SSZ specification, these padding bits must be zero. Using bitsize annotations enables validation of these padding bits during unmarshaling.

### ssz-bitsize

Specifies the exact bit size for bitvector types.

**Syntax**: `ssz-bitsize:"<number>"`

**Applies to**:
- Bitvectors (byte arrays with `ssz-type:"bitvector"`)

**Examples**:
```go
type Attestation struct {
    // 12-bit bitvector: uses 2 bytes, but only 12 bits are valid
    // Padding bits (bits 12-15) are validated to be zero
    CommitteeBits [2]byte `ssz-type:"bitvector" ssz-bitsize:"12"`

    // 64-bit bitvector: exact byte alignment, no padding validation needed
    Flags [8]byte `ssz-type:"bitvector" ssz-bitsize:"64"`
}
```

**Padding validation**: When `ssz-bitsize` is specified and the bit count is not a multiple of 8, unmarshaling will verify that the unused bits in the last byte are zero. This ensures strict SSZ specification compliance.

### dynssz-bitsize

Dynamic bit size specification using runtime expressions.

**Syntax**: `dynssz-bitsize:"<expression>"`

**Applies to**: Same as `ssz-bitsize`

**Examples**:
```go
type DynamicAttestation struct {
    // Bit size based on spec value
    CommitteeBits []byte `ssz-type:"bitvector" ssz-bitsize:"12" dynssz-bitsize:"COMMITTEE_SIZE"`

    // Expression-based bit size
    SyncBits []byte `ssz-type:"bitvector" ssz-bitsize:"512" dynssz-bitsize:"SYNC_COMMITTEE_SIZE"`
}
```

**Combining with ssz-size**: You can use `ssz-bitsize` alongside `ssz-size` when you need both byte-level sizing for fastssz compatibility and bit-level validation:

```go
type ValidatorFlags struct {
    // ssz-size provides byte count for serialization
    // ssz-bitsize enables padding bit validation
    Flags []byte `ssz-type:"bitvector" ssz-size:"2" ssz-bitsize:"12"`
}
```

## Maximum Size Annotations

### ssz-max

Specifies maximum size for dynamic arrays. Highly recommended for hash tree root computation (security implications without it).

**Syntax**: `ssz-max:"<number>"`

**Applies to**:
- Slices (except byte slices with `ssz-size`)
- Bitlists (boolean slices)

**Note on bitlists**: For bitlist types, `ssz-max` specifies the maximum number of **bits**, not bytes. This is consistent with the SSZ specification where bitlist limits are defined in bits.

**Examples**:
```go
type Container struct {
    // Maximum 1024 validators
    Validators []Validator `ssz-max:"1024"`

    // Maximum 2048 bits (not bytes!)
    Participation []bool `ssz-max:"2048"`

    // Bitlist with go-bitfield: max 2048 bits
    AggregationBits bitfield.Bitlist `ssz-max:"2048"`

    // Multi-dimensional arrays
    Matrix [][]uint32 `ssz-max:"100,256"`
}
```


### dynssz-max

Dynamic maximum using runtime expressions.

**Syntax**: `dynssz-max:"<expression>"`

**Applies to**: Same as `ssz-max`

**Examples**:
```go
type State struct {
    // Dynamic validator limit
    Validators []Validator `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
    
    // Expression-based maximum
    Attestations []Attestation `dynssz-max:"MAX_ATTESTATIONS*SLOTS_PER_EPOCH"`
    
    // Multi-dimensional with expressions
    Data [][]byte `dynssz-max:"SHARD_COUNT,MAX_SHARD_BLOCK_SIZE"`
    
    // Multi-dimensional with complex expressions
    Matrix [][]uint32 `dynssz-max:"(ROWS*2),COLS+BUFFER"`
}
```

## Type Annotations

### ssz-type

Explicitly specifies the SSZ type for a field.

**Syntax**: `ssz-type:"<type>"`

**Available types**:

#### Basic Types
- `bool` - Boolean value
- `uint8`, `uint16`, `uint32`, `uint64` - Unsigned integers
- `uint128`, `uint256` - Large integers (requires byte arrays, uint64 arrays, or uint256.Int)

#### Container Types
- `container` - Struct type
- `list` - Dynamic array
- `vector` - Fixed array
- `bitlist` - Dynamic boolean array
- `bitvector` - Fixed boolean array

#### Progressive Types (EIP-7916 & EIP-7495)
- `progressive-list` - List with efficient merkleization
- `progressive-bitlist` - Bitlist with efficient merkleization
- `progressive-container` - Forward-compatible container
- `compatible-union` or `union` - Variant type

#### Special Types
- `custom` - Type implements custom SSZ interfaces
- `wrapper` or `type-wrapper` - Uses TypeWrapper pattern
- `?` or `auto` - Automatic type detection

**Examples**:
```go
type Advanced struct {
    // Force uint256 for byte array
    Balance [32]byte `ssz-type:"uint256"`
    
    // Progressive list for efficiency
    Validators []Validator `ssz-type:"progressive-list"`
    
    // Union type
    Operation CompatibleUnion[Operation] `ssz-type:"compatible-union"`
    
    // Let Dynamic SSZ detect type
    Auto MyOtherType `ssz-type:"?"`
}
```

## Progressive Container Annotations

### ssz-index

Specifies field index for progressive containers (EIP-7495).

**Syntax**: `ssz-index:"<number>"`

**Purpose**: Enables forward/backward compatibility by explicit field ordering

**Examples**:
```go
type BeaconState struct {
    // Core fields with low indices
    GenesisTime uint64 `ssz-index:"0"`
    Slot        uint64 `ssz-index:"1"`
    
    // Fields added in later forks
    NewField1   *uint64 `ssz-index:"5"`
    NewField2   *Data   `ssz-index:"6"`
}
```

**Best practices**:
- Start with index 0 for core fields
- Use pointer fields for indirection

## Expression Language

Dynamic annotations (`dynssz-size`, `dynssz-max`, `dynssz-bitsize`) support expressions with spec values.

### Syntax

**Operators** (precedence order):
1. Parentheses: `()`
2. Multiplication/Division: `*`, `/`
3. Addition/Subtraction: `+`, `-`

**Features**:
- Integer arithmetic only
- Spec value substitution
- Automatic rounding up for size calculations

### Examples

```go
// Simple reference
`dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`

// Basic arithmetic
`dynssz-size:"CHUNK_SIZE*4"`

// Complex expression
`dynssz-max:"(MAX_COMMITTEES_PER_SLOT*SLOTS_PER_EPOCH)+BUFFER"`

// Multi-dimensional (works with all size/max tags)
`ssz-size:"8,16"`
`ssz-max:"100,256"`
`dynssz-size:"ROWS,COLS"`
`dynssz-max:"SHARD_COUNT,MAX_SHARD_BLOCK_SIZE*2"`
```

### Spec Value Resolution

Spec values are resolved at runtime:

```go
specs := map[string]interface{}{
    "CHUNK_SIZE": 32,
    "SLOTS_PER_EPOCH": 32,
    "MAX_COMMITTEES_PER_SLOT": 64,
}
ssz := dynssz.NewDynSsz(specs)
```

## Tag Combinations

### Valid Combinations

```go
type Valid struct {
    // Size + type for byte arrays
    Hash []byte `ssz-size:"32" ssz-type:"vector"`

    // Max + type for lists
    Items []uint64 `ssz-max:"1024" ssz-type:"list"`

    // Progressive type with dynamic max
    Validators []Validator `ssz-type:"progressive-list" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`

    // Bitvector with bitsize for padding validation
    Bits [2]byte `ssz-type:"bitvector" ssz-bitsize:"12"`

    // Dynamic bitvector with both static and dynamic bitsize
    DynBits []byte `ssz-type:"bitvector" ssz-bitsize:"512" dynssz-bitsize:"COMMITTEE_SIZE"`
}
```

### Invalid Combinations

```go
type Invalid struct {
    // Cannot use both size and max
    Bad []byte `ssz-size:"32" ssz-max:"64"`  // Error!
    
    // Cannot use static and dynamic versions together
    Wrong []uint64 `ssz-max:"100" dynssz-max:"MAX_SIZE"`  // Error!
}
```

## Common Patterns

### Ethereum Types

```go
// Common Ethereum consensus types
type EthereumTypes struct {
    // 32-byte hash
    BlockRoot []byte `ssz-size:"32"`
    
    // BLS signature
    Signature []byte `ssz-size:"96"`
    
    // BLS public key
    PublicKey []byte `ssz-size:"48"`
    
    // Dynamic validator set
    Validators []Validator `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}
```

### Bitfields

```go
type Bitfields struct {
    // Fixed-size bitvector (byte-aligned, no padding validation needed)
    Flags [32]byte `ssz-type:"bitvector"`

    // Bitvector with bit-level size (enables padding bit validation)
    CommitteeBits [2]byte `ssz-type:"bitvector" ssz-bitsize:"12"`

    // Dynamic bitvector with spec-based bit size
    SyncBits []byte `ssz-type:"bitvector" ssz-bitsize:"512" dynssz-bitsize:"SYNC_COMMITTEE_SIZE"`

    // Variable-size bitlist
    Votes []byte `ssz-max:"2048"`

    // Using go-bitfield
    Attestation bitfield.Bitlist `ssz-max:"2048"`
}
```

### Pointer Fields

```go
type WithPointers struct {
    // Regular field
    Value uint64
    
    // Pointer field (initialized if nil)
    Pointer *uint64
    
    // Pointer with progressive index
    Feature *Feature `ssz-index:"100"`
}
```

### Nested Arrays

```go
type Nested struct {
    // 2D array with per-dimension limits
    Matrix [][]uint32 `ssz-max:"100,256"`
    
    // Array of fixed-size byte arrays
    Hashes [][32]byte `ssz-max:"1024"`
    
    // Dynamic with expressions
    Data [][]byte `dynssz-max:"SHARD_COUNT,MAX_SHARD_BLOCK_SIZE"`
}
```

## Migration Guide

### From fastssz

Dynamic SSZ is largely compatible with fastssz tags:

```go
// fastssz style (still works)
type FastSSZ struct {
    Data []byte `ssz-size:"32"`
    List []uint64 `ssz-max:"1024"`
}

// Dynamic SSZ additions
type DynamicSSZ struct {
    Data []byte `dynssz-size:"CHUNK_SIZE"`
    List []uint64 `dynssz-max:"MAX_LIST_SIZE"`
}
```

### Adding Progressive Types

Upgrade existing types for better performance:

```go
// Before
type State struct {
    Validators []Validator `ssz-max:"1099511627776"`
}

// After - with progressive merkleization
type State struct {
    Validators []Validator `ssz-type:"progressive-list" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}
```

## Best Practices

1. **Always specify `ssz-max`** for dynamic arrays
2. **Use `dynssz-*` tags** for runtime configuration
3. **Add `ssz-index`** for forward-compatible types
4. **Prefer progressive types** for large collections
5. **Document spec values** used in expressions

## Troubleshooting

### "Missing ssz-max" Warning

Dynamic arrays without `ssz-max` have security implications:
```go
// Discouraged: no maximum size limit
type Risky struct {
    Items []uint64  // Hash tree root calculation is vulnerable
}

// Recommended: specify maximum size
type Safe struct {
    Items []uint64 `ssz-max:"1000"`
}
```

**Security Note**: Without `ssz-max`, hash tree root calculations have security implications. Always specify maximum sizes for production code.

### "Invalid expression" Error

Check expression syntax:
```go
// Valid expressions
`dynssz-max:"VALUE"`
`dynssz-max:"VALUE*2"`
`dynssz-max:"(A+B)*C"`

// Invalid expressions
`dynssz-max:"VALUE^2"`      // No exponentiation
`dynssz-max:"VALUE/2.5"`    // No floats
`dynssz-max:"VALUE OR 100"` // No logical operators
```

### "Spec value not found" Error

Ensure all referenced values are provided:
```go
specs := map[string]interface{}{
    "MISSING_VALUE": 100,  // Add missing spec value
}
```

## Related Documentation

- [Supported Types](supported-types.md) - Complete type reference
- [API Reference](api-reference.md) - Runtime APIs
- [Code Generator](code-generator.md) - Generating efficient code