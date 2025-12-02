# Supported Types

Dynamic SSZ provides comprehensive type support through both automatic detection and explicit annotations. This guide covers all supported types, including the strict type system and progressive types.

## Type Detection System

Dynamic SSZ uses a multi-layered type detection system:

1. **Explicit Type Specification** - via `ssz-type` tag
2. **Interface Detection** - checks for SSZ marshaling interfaces
3. **Well-Known Types** - recognizes common types (uint256.Int, go-bitfield)
4. **Automatic Detection** - infers type from Go type system

## Basic Types

### Boolean
- Go type: `bool`
- SSZ type: `bool`
- Size: 1 byte (0x00 or 0x01)

```go
type Flags struct {
    IsActive bool
}
```

### Unsigned Integers
- Go types: `uint8`, `uint16`, `uint32`, `uint64`
- SSZ types: `uint8`, `uint16`, `uint32`, `uint64`
- Encoding: Little-endian

```go
type Numbers struct {
    Small  uint8
    Medium uint16
    Large  uint32
    XLarge uint64
}
```

### Large Integers (128-bit and 256-bit)

#### Using byte arrays
```go
type Account struct {
    Balance [32]byte `ssz-type:"uint256"` // 256-bit value
    Nonce   [16]byte `ssz-type:"uint128"` // 128-bit value
}
```

#### Using uint64 arrays
```go
type Account struct {
    Balance [4]uint64 `ssz-type:"uint256"` // 4 × 64-bit = 256-bit
    Nonce   [2]uint64 `ssz-type:"uint128"` // 2 × 64-bit = 128-bit
}
```

#### Using holiman/uint256
```go
import "github.com/holiman/uint256"

type Account struct {
    Balance *uint256.Int  // Automatically detected as uint256
}
```

## Collection Types

### Fixed Arrays
- Go type: `[N]T`
- SSZ type: `vector[T, N]`
- Elements must be of same type

```go
type Block struct {
    ParentHashes [16][32]byte  // Vector of 16 hash values
}
```

### Dynamic Arrays (Lists)
- Go type: `[]T`
- SSZ type: `list[T, N]`
- Needs `ssz-max` tag

```go
type Transaction struct {
    Data []byte `ssz-max:"1024"`
}
```

### Byte Arrays and Strings

#### Fixed-size byte arrays
```go
type Hash struct {
    Value [32]byte  // Fixed 32-byte array
}
```

#### Variable-size byte arrays
```go
type Data struct {
    Payload []byte `ssz-max:"2048"`
}
```

#### Strings (fixed-size)
```go
type Name struct {
    First string `ssz-size:"32"`
    Last  string `ssz-size:"32"`
}
```

### Bitvectors and Bitlists

#### Bitvector (fixed-size boolean array)
```go
type Permissions struct {
    Flags [256]bool  // Fixed-size bitvector
}
```

#### Bitvector with bit-level sizing
When a bitvector's bit count is not a multiple of 8, the remaining bits in the last byte are padding bits. Use `ssz-bitsize` to specify the exact bit count and enable padding validation:

```go
type CommitteeFlags struct {
    // 12-bit bitvector stored in 2 bytes
    // Bits 12-15 (padding) are validated to be zero during unmarshaling
    Bits [2]byte `ssz-type:"bitvector" ssz-bitsize:"12"`

    // Dynamic bit size based on spec value
    DynBits []byte `ssz-type:"bitvector" ssz-bitsize:"512" dynssz-bitsize:"SYNC_COMMITTEE_SIZE"`
}
```

**Padding bit validation**: According to the SSZ specification, unused bits in the last byte of a bitvector must be zero. When `ssz-bitsize` or `dynssz-bitsize` is specified, Dynamic SSZ validates these padding bits during unmarshaling and returns an error if any are non-zero.

#### Bitlist (variable-size boolean array)
```go
type Votes struct {
    Participants []bool `ssz-max:"2048"`  // Variable-size bitlist
}
```

#### Using go-bitfield
```go
import bitfield "github.com/prysmaticlabs/go-bitfield"

type Attestation struct {
    AggregationBits bitfield.Bitlist `ssz-max:"2048"`
}
```

## Container Types

### Structs
Structs are the primary container type:

```go
type BeaconBlock struct {
    Slot          uint64
    ProposerIndex uint64
    ParentRoot    [32]byte
    StateRoot     [32]byte
}
```

### Nested Structs
```go
type SignedBeaconBlock struct {
    Message   BeaconBlock
    Signature [96]byte
}
```

### Pointers
Pointers are treated as regular fields and are initialized if nil:

```go
type Block struct {
    Header     BlockHeader
    Body       *BlockBody  // Will be initialized if nil during unmarshaling
}
```

## Progressive Types (EIP-7916 & EIP-7495)

### Progressive Lists (M1)
Optimized merkleization for lists that grow over time:

```go
type State struct {
    Validators []Validator `ssz-type:"progressive-list"`
}
```

Benefits:
- Efficient merkle tree updates when appending
- Reduced computation for growing lists
- Maintains backward compatibility

### Progressive Bitlists (M2)
Optimized for participation tracking:

```go
type Participation struct {
    CurrentEpoch bitfield.Bitlist `ssz-type:"progressive-bitlist"`
}
```

### Progressive Containers (M3)
Forward-compatible containers using `ssz-index`:

```go
type BeaconState struct {
    // Core fields
    GenesisTime uint64 `ssz-index:"0"`
    Slot        uint64 `ssz-index:"1"`
    
    // Fields added in fork
    NewField    *uint64 `ssz-index:"5"`
}
```

Key features:
- Explicit field ordering via `ssz-index`
- Backward/forward compatibility
- Pointer fields for indirection

### Compatible Unions (M4)
Type-safe variant types using struct descriptor:

```go
import dynssz "github.com/pk910/dynamic-ssz"

// Define union using struct descriptor (field order = variant index)
type PayloadUnion = dynssz.CompatibleUnion[struct {
    ExecutionPayload            // Variant 0
    ExecutionPayloadWithBlobs   // Variant 1
}]

// Use in container
type BeaconBlock struct {
    Slot    uint64
    Payload PayloadUnion `ssz-type:"compatible-union"`
}

// Create union instance
block := BeaconBlock{
    Slot: 123,
    Payload: PayloadUnion{
        Variant: 0,  // Use ExecutionPayload
        Data: ExecutionPayload{...},
    },
}
```

## Type Wrapper

For applying SSZ annotations to non-struct types using struct descriptor:

```go
import dynssz "github.com/pk910/dynamic-ssz"

// Define descriptor struct with SSZ tags
type EpochDescriptor struct {
    Data uint64 `dynssz-max:"MAX_EPOCH"`
}

// Use wrapper
type State struct {
    CurrentEpoch dynssz.TypeWrapper[EpochDescriptor, uint64]
}

// Access wrapped value
state := State{
    CurrentEpoch: dynssz.TypeWrapper[EpochDescriptor, uint64]{
        Data: 12345,
    },
}
epoch := state.CurrentEpoch.Get()  // Returns uint64
state.CurrentEpoch.Set(67890)      // Sets new value
```

See [Type Wrapper Documentation](type-wrapper.md) for detailed usage.

## Custom Types

### Implementing SSZ Interfaces

Types can implement custom serialization:

```go
type CustomType struct {
    data []byte
}

func (c *CustomType) MarshalSSZ() ([]byte, error) {
    return c.data, nil
}

func (c *CustomType) UnmarshalSSZ(data []byte) error {
    c.data = data
    return nil
}

func (c *CustomType) SizeSSZ() int {
    return len(c.data)
}

func (c *CustomType) HashTreeRoot() ([32]byte, error) {
    // Custom merkleization
}
```

### Dynamic Interfaces

For spec-aware marshaling:

```go
import "github.com/pk910/dynamic-ssz/sszutils"

type DynamicType struct{}

func (d *DynamicType) MarshalSSZDyn(specs sszutils.DynamicSpecs, buf []byte) ([]byte, error) {
    found, size, err := specs.ResolveSpecValue("DYNAMIC_SIZE")
    if err != nil || !found {
        return nil, err
    }
    // Use spec value for marshaling
    return append(buf, make([]byte, size)...), nil
}

func (d *DynamicType) UnmarshalSSZDyn(specs sszutils.DynamicSpecs, data []byte) error {
    found, size, err := specs.ResolveSpecValue("DYNAMIC_SIZE")
    if err != nil || !found {
        return err
    }
    // Use spec value for unmarshaling
    return nil
}

func (d *DynamicType) SizeSSZDyn(specs sszutils.DynamicSpecs) int {
    found, size, _ := specs.ResolveSpecValue("DYNAMIC_SIZE")
    if !found {
        return 0
    }
    return int(size)
}
```

## Type Annotations

### Automatic Type Detection

Many types are automatically detected:

```go
import (
    "github.com/holiman/uint256"
    bitfield "github.com/prysmaticlabs/go-bitfield"
)

type AutoDetected struct {
    // Automatically detected as uint256
    Balance *uint256.Int
    
    // Automatically detected as bitlist
    Bits bitfield.Bitlist `ssz-max:"2048"`
}
```

### Explicit Type Specification

Use `ssz-type` for explicit control:

```go
type Explicit struct {
    // Force specific type
    Value uint64 `ssz-type:"uint64"`
    
    // Container type
    Data MyStruct `ssz-type:"container"`
}
```

### Special Type Values

- `?` or `auto` - Let Dynamic SSZ detect the type
- `custom` - Type implements custom interfaces
- `wrapper` or `type-wrapper` - Use TypeWrapper pattern

## Multi-dimensional Arrays

Dynamic SSZ supports complex nested structures:

```go
type Matrix struct {
    // 2D fixed array
    Values [10][20]uint32
    
    // Mixed dimensions
    Data [][32]byte `ssz-max:"100"`
    
    // Per-dimension sizing
    Grid [][]uint64 `ssz-max:"100,2048"`
}
```

## Type Validation

### Size Constraints

Dynamic arrays require maximum size:

```go
// Valid
type Good struct {
    Items []uint64 `ssz-max:"1000"`
}

// Invalid - will work, but hash tree root is insecure!
type Bad struct {
    Items []uint64  // Missing ssz-max
}
```

### Type Compatibility

Ensure types are SSZ-compatible:

```go
// Valid types
type Valid struct {
    Number uint64
    Flag   bool
    Data   []byte `ssz-max:"1024"`
}

// Invalid types
type Invalid struct {
    Channel chan int      // Channels not supported
    Func    func()        // Functions not supported
    Iface   interface{}   // Interfaces not supported
}
```

## Performance Considerations

### Type Selection

1. **Prefer fixed-size types** when possible
2. **Use progressive types** for large, growing collections
3. **Implement custom interfaces** for complex types
4. **Use TypeWrapper** for reusable type patterns

### Memory Efficiency

- Bitvectors are more efficient than bool arrays
- Progressive lists reduce merkleization cost
- Custom types can optimize for specific patterns

## Examples

### Ethereum Beacon State
```go
type BeaconState struct {
    // Fixed-size fields
    GenesisTime           uint64
    GenesisValidatorsRoot [32]byte
    Slot                  uint64
    
    // Progressive list for efficiency
    Validators []Validator `ssz-type:"progressive-list"`
    
    // Bitlist for participation
    JustificationBits bitfield.Bitvector4
    
    // Dynamic with expression
    Balances []uint64 `ssz-max:"500" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}
```

### Complex Nested Type
```go
type ComplexData struct {
    // Multi-dimensional with per-dimension limits
    Matrix [][]uint32 `ssz-max:"100,256"`
    
    // Pointer to nested structure
    Extra *struct {
        Data  []byte `ssz-max:"1024"`
        Index uint64
    }
    
    // Union type
    Operation dynssz.CompatibleUnion[struct {
        Deposit
        Withdrawal
    }] `ssz-type:"compatible-union"`
}
```

## Related Documentation

- [SSZ Annotations](ssz-annotations.md) - Detailed tag reference
- [Type Wrapper](type-wrapper.md) - Advanced type patterns
- [API Reference](api-reference.md) - Type-related APIs