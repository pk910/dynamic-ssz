# Struct Tags & Annotations

Dynamic SSZ uses struct tags to control how fields are encoded and decoded. This guide covers all available tags and their usage.

## Overview

Dynamic SSZ supports several struct tags that provide fine-grained control over serialization:

- `ssz-size` - Define field sizes (compatible with fastssz)
- `dynssz-size` - Dynamic sizes based on specifications
- `ssz-max` - Maximum sizes for dynamic fields
- `dynssz-max` - Dynamic maximum sizes
- `ssz-type` - Explicit type specifications

## Size Tags

### `ssz-size`

Defines field sizes. This tag follows the same format supported by fastssz, allowing seamless integration.

```go
type Example struct {
    // Fixed-size array
    Hash [32]byte                    // No tag needed for arrays
    
    // Fixed-size slice
    FixedSlice []byte `ssz-size:"32"` // Exactly 32 bytes
    
    // Dynamic slice (? indicates dynamic)
    DynamicSlice []byte `ssz-size:"?"` // Dynamic size
    
    // Multi-dimensional
    Matrix [][]byte `ssz-size:"4,32"`  // 4 rows, 32 bytes each
}
```

**Key Points:**
- Use `?` to indicate dynamic length dimensions
- Numbers indicate fixed sizes
- Multi-dimensional sizes are comma-separated

### `dynssz-size`

Specifies sizes based on specification properties, supporting mathematical expressions.

```go
type Example struct {
    // Direct reference
    Roots []byte `dynssz-size:"SLOTS_PER_HISTORICAL_ROOT"`
    
    // Mathematical expression
    Buffer []byte `dynssz-size:"(MAX_SIZE*2)-5"`
    
    // Complex expression
    Data []byte `dynssz-size:"(SLOT_SIZE*EPOCH_LENGTH)+BUFFER"`
}
```

**Expression Support:**
- Basic arithmetic: `+`, `-`, `*`, `/`
- Parentheses for grouping
- Multiple spec values in one expression
- Evaluated at runtime based on provided specifications

## Maximum Size Tags

### `ssz-max`

**Required** for all dynamic length fields. Defines the maximum number of elements.

```go
type Example struct {
    // Dynamic slice with max
    Validators []Validator `ssz-max:"1000000"`
    
    // Multi-dimensional with max
    Data [][]byte `ssz-size:"?,32" ssz-max:"100"`
    
    // Both dimensions dynamic
    Matrix [][]uint64 `ssz-size:"?,?" ssz-max:"100,1000"`
}
```

**Important Rules:**
- Every dynamic field MUST have either `ssz-max` or `dynssz-max`
- Fixed-size fields should NOT have max tags
- Used for merkle tree depth calculation

### `dynssz-max`

Dynamic maximum sizes based on specifications, with expression support.

```go
type BeaconState struct {
    Validators []Validator `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
    
    // With expressions
    Buffer []byte `dynssz-max:"MAX_SIZE*2"`
}
```

## Type Specification Tags

### `ssz-type`

Explicitly specifies the SSZ type, overriding automatic detection.

```go
type Example struct {
    // Basic types
    Count uint64              `ssz-type:"uint64"`
    Flag  bool                `ssz-type:"bool"`
    
    // Bitfield types
    Bits     []byte          `ssz-type:"bitlist" ssz-max:"256"`
    FixedBits [8]byte        `ssz-type:"bitvector"`
    
    // Large integers
    Balance  [32]byte        `ssz-type:"uint256"`
    Amount   [16]byte        `ssz-type:"uint128"`
    
    // Container types
    Nested   NestedStruct    `ssz-type:"container"`
    
    // Special values
    Auto     []byte          `ssz-type:"auto"`    // Automatic detection
    Custom   CustomType      `ssz-type:"custom"`  // Has FastSSZ methods
}
```

**Supported Types:**
- Basic: `"bool"`, `"uint8"`, `"uint16"`, `"uint32"`, `"uint64"`, `"uint128"`, `"uint256"`
- Composite: `"container"`, `"list"`, `"vector"`, `"bitlist"`, `"bitvector"`
- Special: `"?"` or `"auto"` (automatic), `"custom"` (fastssz implementation)

## Multi-dimensional Arrays

Dynamic SSZ fully supports multi-dimensional slices with different constraints per dimension.

```go
type MultiDim struct {
    // Outer dynamic (max 100), inner fixed (32 bytes each)
    Data1 [][]byte `ssz-size:"?,32" ssz-max:"100"`
    
    // Both dimensions dynamic
    Data2 [][]uint64 `ssz-size:"?,?" ssz-max:"64,256"`
    
    // Three dimensions: dynamic, fixed, dynamic
    Data3 [][][]byte `ssz-size:"?,4,?" ssz-max:"10,,20"`
    
    // With dynamic specs
    Data4 [][]byte `ssz-size:"?,?" dynssz-max:"OUTER_LIMIT,INNER_LIMIT"`
}
```

**Rules:**
- Sizes and maxes are specified from outermost to innermost
- Use empty values in comma-separated lists for fixed dimensions
- Each dynamic dimension needs a corresponding maximum

## Real-World Examples

### Ethereum Beacon State

```go
type BeaconState struct {
    // Fixed fields
    GenesisTime           uint64
    GenesisValidatorsRoot phase0.Root `ssz-size:"32"`
    Slot                  phase0.Slot
    Fork                  *phase0.Fork  // Pointer = optional
    
    // Fixed with dynamic spec
    BlockRoots []phase0.Root `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
    StateRoots []phase0.Root `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
    
    // Dynamic with limits
    HistoricalRoots []phase0.Root `ssz-size:"?,32" ssz-max:"16777216" dynssz-max:"HISTORICAL_ROOTS_LIMIT"`
    
    // Large dynamic arrays
    Validators []Validator `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
    Balances   []uint64    `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
    
    // Bitlists
    PreviousEpochParticipation []byte `ssz-type:"bitlist" ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}
```

### Custom Application

```go
type GameState struct {
    // Basic types
    Round      uint32
    IsActive   bool
    
    // Fixed arrays
    BoardState [8][8]uint8              // 8x8 chess board
    PlayerIDs  [2][16]byte              // 2 players, 16-byte IDs
    
    // Dynamic with application-specific limits
    MoveHistory []Move    `ssz-max:"1000" dynssz-max:"MAX_MOVES"`
    ChatLog     []Message `ssz-max:"100"  dynssz-max:"MAX_MESSAGES"`
    
    // Bitfields
    Flags      [4]byte   `ssz-type:"bitvector"`     // 32 flags
    ActiveUnits []byte   `ssz-type:"bitlist" ssz-max:"256"` // Up to 256 units
    
    // Optional fields (nil = not present)
    PowerUp    *PowerUpData
    
    // Complex multi-dimensional
    Grid       [][]Cell  `ssz-size:"?,?" dynssz-size:"GRID_WIDTH,GRID_HEIGHT"`
}
```

## Best Practices

1. **Always specify max for dynamic fields** - Without max tags, hash tree root calculation will fail
2. **Use dynssz tags for configuration** - Allows runtime flexibility without recompilation
3. **Prefer fixed sizes when possible** - Better performance and simpler code
4. **Be explicit with types** - Use `ssz-type` for bitlists and large integers
5. **Order fields efficiently** - Group fixed-size fields before dynamic ones

## Common Pitfalls

1. **Missing max tags**
   ```go
   // WRONG - will fail at hash tree root
   type Bad struct {
       Data []byte  // Missing ssz-max!
   }
   
   // CORRECT
   type Good struct {
       Data []byte `ssz-max:"1024"`
   }
   ```

2. **Max tags on fixed fields**
   ```go
   // WRONG - fixed fields don't need max
   type Bad struct {
       Hash [32]byte `ssz-max:"32"`  // Don't do this!
   }
   
   // CORRECT
   type Good struct {
       Hash [32]byte  // No max tag needed
   }
   ```

3. **Incorrect multi-dimensional specs**
   ```go
   // WRONG - dimensions don't match
   type Bad struct {
       Data [][]byte `ssz-size:"?" ssz-max:"100"`  // Need max for both!
   }
   
   // CORRECT
   type Good struct {
       Data [][]byte `ssz-size:"?,32" ssz-max:"100"`     // Outer dynamic, inner fixed
       // OR
       Data [][]byte `ssz-size:"?,?" ssz-max:"100,1024"` // Both dynamic
   }
   ```

## Type Detection

Dynamic SSZ automatically detects certain well-known types:

- `github.com/holiman/uint256.Int` → detected as `uint256`
- `github.com/prysmaticlabs/go-bitfield` types → detected as bitlists/bitvectors
- Types implementing `ssz.Marshaler` → detected as custom types

You can override automatic detection using the `ssz-type` tag.