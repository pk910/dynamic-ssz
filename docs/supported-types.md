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

### Timestamps

- Go type: `time.Time`
- SSZ type: `uint64`
- Encoding: Unix seconds, little-endian

`time.Time` is detected automatically and encoded as `uint64(t.Unix())`. It is
**lossy**: sub-second precision is dropped, the monotonic clock reading is not
carried, and the decoded value is normalized to UTC. A round-trip therefore does
not satisfy `orig.Equal(decoded)` unless the original was already a whole second
in UTC. Encode the raw nanoseconds yourself if you need them preserved.

```go
type Event struct {
    // 2023-11-14T22:13:20.123456789Z encodes as 2023-11-14T22:13:20Z
    Observed time.Time
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

#### Lists without a limit

The limit is part of the type in SSZ: `List[T, N]` and `Bitlist[N]` need `N` to
merkleize, so a list with no `ssz-max` (or `ssz-max:"0"`, the placeholder for a
limit that only a `dynssz-max` expression supplies) has **no hash tree root**.

Serialization never needs the limit, so such a list marshals and unmarshals
normally — only hashing is refused, and only when [extended
types](extended-types.md) are disabled:

```go
type Unbounded struct {
    Data []uint64  // no ssz-max
}

ds := dynssz.NewDynSsz(nil)
buf, _ := ds.MarshalSSZ(&Unbounded{Data: []uint64{1, 2, 3}})  // fine
_, err := ds.HashTreeRoot(&Unbounded{Data: []uint64{1, 2, 3}})
// err: list has no ssz-max, so it has no SSZ hash tree root
```

With extended types enabled, the list is hashed as an unbounded list:
merkleized to the chunks the value occupies with the length mixed in, so the
root identifies the value. That root is outside the spec and **no other SSZ
implementation will agree on it**. The code generator emits the same root and
prints a warning for every limit-less list it generates.

A view's data type is unaffected: it carries no tags because the layout lives
in the view schema, so hash it through its view.

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

#### Bitvector (fixed-size bit array)

A bitvector is a bit-packed fixed-size boolean array. Model it with a
byte-backed field annotated `ssz-type:"bitvector"`:

```go
type Permissions struct {
    Flags [32]byte `ssz-type:"bitvector"`  // 256-bit bitvector (32 bytes)
}
```

> **Note**: A Go `[N]bool` array is **not** a bitvector — it auto-detects as
> `Vector[boolean, N]`, one byte per element (`[256]bool` serializes to 256
> bytes, not 32). Use a byte-backed field with `ssz-type:"bitvector"` for
> bit packing.

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

#### Bitlist (variable-size bit array)

A bitlist is a bit-packed variable-size boolean array. Model it with a
byte-backed field annotated `ssz-type:"bitlist"` (or use `bitfield.Bitlist`,
below). For bitlists, `ssz-max` specifies the maximum number of **bits**, not
bytes, consistent with the SSZ specification.

```go
type Votes struct {
    Participants []byte `ssz-type:"bitlist" ssz-max:"2048"`  // Maximum 2048 bits
}
```

> **Canonical form**: the byte slice includes the terminating sentinel bit, so
> the canonical empty bitlist is `[]byte{0x01}`. An empty `[]byte{}` is
> accepted on encode as the empty bitlist, and decoding always yields the
> canonical form — a nil/empty slice does not survive a round trip unchanged.

> **Note**: A Go `[]bool` slice is **not** a bitlist — it auto-detects as
> `List[boolean, N]`, one byte per element, with `ssz-max` counted in elements
> rather than bits. Use a byte-backed field with `ssz-type:"bitlist"` for a
> bit-packed bitlist.

A bitlist without `ssz-max` has no hash tree root either, and follows the same
rule as [a list without a limit](#lists-without-a-limit).

#### Using go-bitfield
```go
import bitfield "github.com/OffchainLabs/go-bitfield"

type Attestation struct {
    AggregationBits bitfield.Bitlist `ssz-max:"2048"`  // Maximum 2048 bits
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

### Recursive Types

A type may refer to itself, as long as every cycle passes through a
variable-length field. The length of that field is what makes the encoding
finite — a list of zero elements terminates the recursion:

```go
type Node struct {
    Value    uint64
    Children []*Node `ssz-max:"4"`  // cycle closes through a list
}
```

A cycle that crosses only fixed-size fields has no finite encoding and is
rejected when the type is analyzed:

```go
type Bad struct {
    Value uint64
    Next  *Bad  // rejected: recursive type *Bad is not supported
}
```

#### Nesting depth is bounded

Encoding, decoding and hashing all walk the value recursively, so a deeply
nested value would otherwise exhaust the goroutine stack — and Go aborts the
process on stack exhaustion with `fatal error: stack overflow`, which
`recover()` cannot catch. Each level of a recursive type costs only a handful of
wire bytes, so a small piece of untrusted input can declare very deep nesting.

Both engines therefore bound the depth and return `sszutils.ErrMaxDepthExceeded`
instead. The default is 1024, far deeper than any practical schema (Ethereum
consensus types stay under 20).

Both bounds count by the same rule: one level per type descended into that
lies on a recursive cycle, whether its code is generated, inlined, or walked
by reflection; the outermost value itself costs nothing. A trip around a cycle
therefore costs as many levels as the cycle has structural members — `Node`
with a `[]*Node` field costs two (the container and the list; a type wrapper
costs none) — and the engines accept
and reject at identical nesting depths. Everything off a cycle bottoms out at
a depth fixed by its own structure and is never counted.

| | Configure with | Applies from |
|---|---|---|
| Reflection | `dynssz.WithMaxNestingDepth(n)` | immediately |
| Generated code | `codegen.WithRecursionDepth(n)` | after regeneration |

The generated value is baked into the emitted code, so changing it requires
regenerating. One caveat: a chain of *distinct* cycles spanning several
packages restarts the generated count at each package boundary (the
depth-carrying methods are unexported), while reflection counts such a chain
as one run. Each side stays bounded either way.

Non-recursive types are unaffected by either bound, and the code generated for
them is unchanged: only types on or above a cycle carry a depth.

> **Cyclic values are unencodable.** `ValidateType` accepts the type above
> because the type graph is legal, but a *value* whose pointers form a cycle
> (`n.Children = []*Node{n}`) has no finite encoding. `SizeSSZ`, `MarshalSSZ`
> and `HashTreeRoot` follow the cycle until the depth bound stops them, so do
> not build parent/child back-references in values you intend to serialize.

#### Recursive types with views

A recursive type can be generated with views, and the depth bound is carried
across the view boundary. It costs more stack than the plain case, though.

A view dispatcher hands back a closure rather than being called directly, so it
has nowhere to take a depth argument. For a type on a cycle the dispatcher
therefore returns a closure that supplies the depth itself — zero from the
public `MarshalSSZDynView`, and the caller's depth from the unexported twin the
generated code uses when it descends. That closure is a real function, so each
level of a recursive *view* costs two stack frames where the plain path costs
one:

| | Stack frames per level | Nesting reached at the default 1024 |
|---|---|---|
| Plain recursive type | 1 | 1024 |
| Recursive type through a view | 2 | 1024 |

The bound counts nesting levels either way, so both stop at the same depth — the
view path simply uses about twice the stack getting there. If you are sizing
`WithRecursionDepth` against a stack budget rather than against your schema,
halve it for types you decode through views.

### Optional Lists (canonical `List[T, 1]`)

Annotating a pointer field with `ssz-type:"optional-list"` encodes it as the canonical SSZ `List[T, 1]` — the encoding the Ethereum spec uses for canonical optional fields:

- `nil` pointer → empty list (no bytes)
- non-`nil` pointer → single-element list

This is **standard SSZ** and works without `WithExtendedTypes()`. It is distinct from the extended `ssz-type:"optional"` annotation, which uses a non-canonical presence-byte format.

```go
type Block struct {
    Slot uint64
    // nil   → empty list
    // &val  → List[T, 1] containing val
    Sidecar *Sidecar `ssz-type:"optional-list"`
}
```

Encoding:

| Case                     | Bytes                                                 |
|--------------------------|-------------------------------------------------------|
| `nil`                    | (empty)                                               |
| non-nil, fixed element   | `<element bytes>`                                     |
| non-nil, dynamic element | `0x04 0x00 0x00 0x00 || <element bytes>` (offset = 4) |

The hash tree root matches `List[T, 1]` exactly: the element's root merkleized
under a limit of one chunk, with the element count mixed in (0 for `nil`, 1
otherwise). The non-canonical [`ssz-type:"optional"`](extended-types.md#optional-types-pointers)
produces the same root, so the two differ only in their encoding.

## Union Types

Classic SSZ unions (`Union[type_0, type_1, ...]` from the SSZ specification) are expressed with the generic `dynssz.Union[T]` helper. The descriptor struct `T` lists the variants in order; selectors are the 0-based field positions:

```go
import dynssz "github.com/pk910/dynamic-ssz"

// Selector = descriptor field position (0-based).
type PayloadUnion = dynssz.Union[struct {
    Legacy ExecutionPayload    // selector 0
    Full   FullPayload         // selector 1
}]

type Block struct {
    Slot    uint64
    Payload PayloadUnion
}
```

Declaring `dynssz.None` as the **first** descriptor field makes selector 0 the empty option (`Union[None, type_1, ...]`):

```go
type MaybePayload = dynssz.Union[struct {
    None dynssz.None         // selector 0: no value
    Full ExecutionPayload    // selector 1
}]

// The zero value {Variant: 0, Data: nil} is the None option.
// It serializes as the single byte 0x00 and hashes as
// mix_in_selector(Bytes32(), 0).
```

Spec rules enforced at descriptor build:

- Selectors are positional; `ssz-index` tags are not allowed
- `None` is only legal as the first field, and a union declaring it must offer at least one further variant
- At most 128 variants (selectors above 127 are reserved)

Serialization is `selector byte || serialize(value)`; the hash tree root is `mix_in_selector(hash_tree_root(value), selector)`. Variant fields may carry the usual `ssz-size`/`ssz-max`/`ssz-type` tags.

For the EIP-8016 `CompatibleUnion` flavor (1-based selectors, no None, compatible merkleization), see [Compatible Unions](#compatible-unions-m4).

> **A union has no "unset" encoding.** In a vector of unions every element must
> be set before the value encodes, since a vector always has as many elements as
> its length says. A zero-valued `CompatibleUnion` names selector 0, which
> EIP-8016 does not have, and a zero-valued classic union has no data for the
> variant its selector names — both are refused, with the offending element in
> the error path. Only a classic union declaring `None` first has an encodable
> zero value. The same applies to a slice-backed vector shorter than its
> declared length: the padding elements are unset, and no selector is chosen on
> your behalf.

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

// Define union using a struct descriptor. Selectors follow field order
// starting at 1 (EIP-8016 allows 1..127); ssz-index tags assign them explicitly.
type PayloadUnion = dynssz.CompatibleUnion[struct {
    ExecutionPayload            // Variant 1
    ExecutionPayloadWithBlobs   // Variant 2
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
        Variant: 1,  // Use ExecutionPayload
        Data: ExecutionPayload{...},
    },
}
```

## Type Wrapper

For applying SSZ annotations to non-struct values using a descriptor struct with one tagged field:

```go
import dynssz "github.com/pk910/dynamic-ssz"

// Descriptor struct: exactly one field with SSZ tags, same type as T
type FixedBytesDescriptor struct {
    Data []byte `ssz-size:"32"`
}

// Use wrapper in a container
type MyContainer struct {
    Hash dynssz.TypeWrapper[FixedBytesDescriptor, []byte] `ssz-type:"wrapper"`
}

// Access wrapped value
container := MyContainer{}
container.Hash.Set([]byte{1, 2, 3})
value := container.Hash.Get()
```

See [Type Wrapper](type-wrapper.md) for detailed usage.

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

func (d *DynamicType) UnmarshalSSZDyn(specs sszutils.DynamicSpecs, buf []byte) error {
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
    bitfield "github.com/OffchainLabs/go-bitfield"
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

Dynamic arrays should specify a maximum size for secure hash tree root computation:

```go
// Recommended: maximum size specified
type Good struct {
    Items []uint64 `ssz-max:"1000"`
}

// Discouraged: no maximum size (hash tree root has security implications)
type Risky struct {
    Items []uint64
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
    Validators []Validator `ssz-type:"progressive-list" ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`

    // Bitlist for participation
    JustificationBits bitfield.Bitvector4

    // Dynamic with expression (ssz-max is fallback when spec value is unavailable)
    Balances []uint64 `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
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

## Extended Types (Non-Standard)

Dynamic SSZ also supports an extended set of types that are **not part of the SSZ specification**. These include signed integers (`int8`, `int16`, `int32`, `int64`), floating-point numbers (`float32`, `float64`), arbitrary-precision integers (`big.Int`), and optional types (pointer types annotated with `ssz-type:"optional"`).

Extended types must be explicitly enabled with `WithExtendedTypes()` and are **not compatible with other SSZ libraries**.

See [Extended Types](extended-types.md) for full documentation.

## Related Documentation

- [Extended Types](extended-types.md) - Non-standard type extensions
- [SSZ Annotations](ssz-annotations.md) - Detailed tag reference
- [Type Wrapper](type-wrapper.md) - Advanced type patterns
- [API Reference](api-reference.md) - Type-related APIs