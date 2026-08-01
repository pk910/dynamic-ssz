# SSZ Annotations

Dynamic SSZ uses struct tags to control SSZ encoding behavior. This guide covers all available annotations, how static and dynamic tags interact, and the expression language.

## Tag Overview

| Tag | Purpose | Example |
|-----|---------|---------|
| `ssz-size` | Fixed size for bytes/strings | `ssz-size:"32"` |
| `ssz-max` | Maximum size for dynamic types | `ssz-max:"1024"` |
| `ssz-type` | Explicit type specification | `ssz-type:"uint256"` |
| `ssz-bitsize` | Bit-level size for bitvectors | `ssz-bitsize:"12"` |
| `ssz-index` | Field index for progressive containers | `ssz-index:"0"` |
| `dynssz-size` | Dynamic size with expressions | `dynssz-size:"EPOCH_LENGTH*32"` |
| `dynssz-max` | Dynamic maximum with expressions | `dynssz-max:"VALIDATOR_REGISTRY_LIMIT"` |
| `dynssz-bitsize` | Dynamic bit size for bitvectors | `dynssz-bitsize:"COMMITTEE_SIZE"` |

## Static and Dynamic Tags

### How They Work Together

The `ssz-*` and `dynssz-*` tags are designed to be used in combination. The static tag provides a default value, and the dynamic tag overrides it when the referenced spec value can be resolved.

```go
type State struct {
    // ssz-max: static fallback (used when VALIDATOR_REGISTRY_LIMIT is not in specs)
    // dynssz-max: resolved at runtime from spec values (overrides ssz-max when available)
    Validators []Validator `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}
```

**Resolution order:**
1. Parse the `ssz-*` tag to get the static default
2. Parse the `dynssz-*` tag and attempt to resolve the expression
3. If the expression resolves successfully, override the static value
4. If the expression cannot be resolved (spec value not provided), keep the static default

This design allows the same type definitions to work across different configurations. For example, Ethereum types can use mainnet defaults in `ssz-max` while allowing `dynssz-max` to adapt to minimal or custom presets.

**Recommended usage**: Always provide both tags for fields that need dynamic sizing:

```go
type BeaconBlockBody struct {
    Attestations []Attestation `ssz-max:"128" dynssz-max:"MAX_ATTESTATIONS"`
    Deposits     []Deposit     `ssz-max:"16"  dynssz-max:"MAX_DEPOSITS"`
}
```

If you only use `dynssz-max` without `ssz-max`, the field has no fallback when the spec value is unavailable.

### Annotation Locations

#### Struct Field Tags (Standard)

The standard way to annotate SSZ fields is through Go struct tags:

```go
type BeaconState struct {
    Validators []Validator `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}
```

#### Type Annotations with `sszutils.Annotate[T]()` (Non-Struct Types)

For top-level non-struct type definitions (e.g., named slices, arrays), Go does not allow struct tags. Use `sszutils.Annotate[T]()` to register SSZ annotations:

```go
import "github.com/pk910/dynamic-ssz/sszutils"

type Blobs []*Blob
var _ = sszutils.Annotate[Blobs](`ssz-max:"4096"`)

type BlobKZGs [][48]byte
var _ = sszutils.Annotate[BlobKZGs](`ssz-max:"4096" dynssz-max:"MAX_BLOB_COMMITMENTS_PER_BLOCK"`)
```

The tag string uses the same `key:"value"` syntax as Go struct field tags. All SSZ tags are supported.

**How it works:**
- `Annotate[T]()` registers the tag string in a global registry at package init time
- Both the code generator and the runtime reflection path read from this registry
- The code generator discovers `Annotate` calls by scanning the source AST

**Important:**
- Call `Annotate[T]()` at package level (in a `var` block or `init()` function) so the annotation is registered before any SSZ operation
- The tag string must use the exact struct tag format with quoted values: `ssz-max:"4096"`, not `ssz-max:4096`
- When a struct field uses an annotated type but also has its own field tags, the field tags take precedence

## Size Annotations

### Multi-dimensional Syntax

All size and maximum annotations support comma-separated values for nested arrays:

```go
type MultiDimensional struct {
    Grid   [][]byte   `ssz-size:"8,16"`    // 8 rows, 16 bytes each
    Matrix [][]uint32 `ssz-max:"100,256"`  // max 100 rows, max 256 per row
    Data   [][]byte   `dynssz-size:"ROWS,COLS"`
}
```

### The `?` placeholder

A dimension can be written as `?` instead of a value. It does not mean "leave
this one alone" — it is a declaration in its own right:

| Tag | `?` means |
|-----|-----------|
| `ssz-size` / `dynssz-size` | the dimension is **dynamic** — a list, not a vector |
| `ssz-max` / `dynssz-max` | the dimension is **unbounded** — no limit |

Two rules follow from that, and both are enforced when the type is analyzed:

**A dimension must be declared the same way by a tag and its dynamic
counterpart.** `?` in one and a value in the other describes two different types
— a list and a vector, or a bounded and an unbounded list — and the field would
encode differently depending on which tag was consulted:

```go
// Rejected: dimension 0 is a 2-vector to ssz-size and dynamic to dynssz-size.
Bad  [2][]byte `ssz-size:"2,6" dynssz-size:"?,COLS"`

// Correct: repeat the static length; only dimension 1 is spec-driven.
Good [2][]byte `ssz-size:"2,6" dynssz-size:"2,COLS"`

// Rejected: dimension 0 has a limit statically but none dynamically.
Bad2 [][]uint64 `ssz-max:"16,8" dynssz-max:"?,ROW_LIMIT"`

// Correct: the placeholders line up, so dimension 0 is unbounded either way.
Good2 [][]uint64 `ssz-max:"?,8" dynssz-max:"?,ROW_LIMIT"`
```

**A dimension cannot be `?` to both families.** `ssz-size:"?"` with
`ssz-max:"?"` gives it neither a length nor a limit, which names no SSZ type —
`List[T, N]` needs its `N` to merkleize (see
[lists without a limit](supported-types.md#lists-without-a-limit)):

```go
// Rejected: dimension 0 is dynamic and unbounded at once.
Bad [][]byte `ssz-size:"?,6" ssz-max:"?,8"`
```

Omitting a tag is not the same as writing `?`. A field with no `ssz-max` at all
is described by its Go type, and an unbounded list stays expressible that way
under [extended types](extended-types.md#unbounded-lists-and-bitlists).

### ssz-size

Specifies a fixed size for variable-length types, turning them into SSZ vectors.

**Applies to**: `[]byte`, `string`, and nested slices

```go
type Data struct {
    Hash []byte `ssz-size:"32"`    // fixed 32-byte vector
    Name string `ssz-size:"64"`    // fixed 64-byte string (null-padded)
    Grid [][]byte `ssz-size:"8,16"` // 8 rows, 16 bytes each
}
```

Strings are null-padded if shorter than the specified size. Cannot be combined with `ssz-max` on the same field (a field is either fixed-size or variable-size).

### dynssz-size

Dynamic size using runtime expressions. Used alongside `ssz-size`:

```go
type Dynamic struct {
    Data   []byte `ssz-size:"32" dynssz-size:"CHUNK_SIZE"`
    Buffer []byte `ssz-size:"256" dynssz-size:"SLOT_SIZE*EPOCH_LENGTH"`
    Matrix [][]byte `ssz-size:"8,16" dynssz-size:"ROWS,COLS"`
}
```

## Bit Size Annotations (Bitvectors)

The `ssz-bitsize` and `dynssz-bitsize` annotations specify size in **bits**. These are for bitvectors where the exact bit count matters for padding validation.

When a bitvector's bit count is not a multiple of 8, the remaining bits in the last byte are padding. Per the SSZ spec, these padding bits must be zero. Using bitsize annotations enables this validation during unmarshaling.

### ssz-bitsize

```go
type Attestation struct {
    // 12-bit bitvector in 2 bytes; bits 12-15 validated to be zero
    CommitteeBits [2]byte `ssz-type:"bitvector" ssz-bitsize:"12"`

    // 64-bit bitvector: byte-aligned, no padding validation needed
    Flags [8]byte `ssz-type:"bitvector" ssz-bitsize:"64"`
}
```

### dynssz-bitsize

Dynamic bit size using runtime expressions:

```go
type DynamicAttestation struct {
    CommitteeBits []byte `ssz-type:"bitvector" ssz-bitsize:"12" dynssz-bitsize:"COMMITTEE_SIZE"`
    SyncBits      []byte `ssz-type:"bitvector" ssz-bitsize:"512" dynssz-bitsize:"SYNC_COMMITTEE_SIZE"`
}
```

## Maximum Size Annotations

### ssz-max

Specifies the maximum element count for dynamic lists. Required for secure hash tree root computation.

**Note on bitlists**: For bitlist types, `ssz-max` specifies the maximum number of **bits**, not bytes. This is consistent with the SSZ specification.

```go
type Container struct {
    Validators    []Validator `ssz-max:"1024"`   // max 1024 validators
    Participation []byte      `ssz-type:"bitlist" ssz-max:"2048"` // bitlist, max 2048 bits
    Matrix        [][]uint32  `ssz-max:"100,256"` // max 100 rows, 256 per row
}
```

### dynssz-max

Dynamic maximum using runtime expressions. Used alongside `ssz-max`:

```go
type State struct {
    Validators   []Validator   `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
    Attestations []Attestation `ssz-max:"128" dynssz-max:"MAX_ATTESTATIONS*SLOTS_PER_EPOCH"`
    Data         [][]byte      `ssz-max:"64,1073741824" dynssz-max:"SHARD_COUNT,MAX_SHARD_BLOCK_SIZE"`
}
```

## Type Annotations

### ssz-type

Explicitly specifies the SSZ type for a field. When omitted, the type is auto-detected from the Go type.

**Basic types**: `bool`, `uint8`, `uint16`, `uint32`, `uint64`, `uint128`, `uint256`

**Collection types**: `container`, `list`, `vector`, `bitlist`, `bitvector`

**Union types**: `union` (classic spec union via `dynssz.Union[T]`, 0-based positional selectors)

**Progressive types** (EIP-7916/7495): `progressive-list`, `progressive-bitlist`, `progressive-container`, `compatible-union` (EIP-8016 via `dynssz.CompatibleUnion[T]`)

**Pointer encoding**: `optional-list` — encodes a pointer as canonical `List[T, 1]`

**Extended types** (non-standard, requires `WithExtendedTypes()`): `int8`, `int16`, `int32`, `int64`, `float32`, `float64`, `bigint`, `optional`

**Special values**: `custom` (type implements SSZ interfaces), `wrapper`/`type-wrapper` (TypeWrapper pattern), `?`/`auto` (auto-detect), `-` (exclude the field)

```go
type Advanced struct {
    Balance    [32]byte    `ssz-type:"uint256"`
    Validators []Validator `ssz-type:"progressive-list" ssz-max:"1099511627776"`
    Operation  CompatibleUnion[Op] `ssz-type:"compatible-union"`
}
```

#### Excluding a field

`ssz-type:"-"` removes a field from the SSZ layout entirely: it is not
encoded, decoded, sized or hashed, and its Go type does not need to be
SSZ-compatible. This is useful for caches, computed values, or metadata kept
alongside the SSZ data. On decode the field is left **unchanged** — it is
skipped, not reset, so when decoding into a reused object it keeps its previous
value. Clear or reinitialize such fields yourself if you need them zeroed.

```go
type Block struct {
    Slot uint64
    Body BlockBody
    root [32]byte        `ssz-type:"-"` // cached root, not serialized
    seen map[string]bool `ssz-type:"-"` // non-SSZ type, ignored
}
```

The struct above encodes identically to one containing only `Slot` and `Body`.
Both the reflection and code-generation engines honor the exclusion. (This is
the dynamic-ssz spelling of fastssz's `ssz:"-"`; dynamic-ssz does not read the
plain `ssz` struct tag.)

### ssz-index

Specifies field index for progressive containers (EIP-7495), enabling forward/backward compatibility:

```go
type BeaconState struct {
    GenesisTime uint64  `ssz-index:"0"`
    Slot        uint64  `ssz-index:"1"`
    NewField    *uint64 `ssz-index:"5"` // added in a later fork
}
```

## Expression Language

Dynamic annotations (`dynssz-size`, `dynssz-max`, `dynssz-bitsize`) support expressions with spec values.

**Operators** (standard precedence):
1. Parentheses: `()`
2. Multiplication/Division: `*`, `/`
3. Addition/Subtraction: `+`, `-`

**Features**: Integer arithmetic only. Spec value substitution. Automatic rounding up for partial bytes.

```go
// Simple reference
`dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`

// Arithmetic
`dynssz-size:"CHUNK_SIZE*4"`

// Complex expression
`dynssz-max:"(MAX_COMMITTEES_PER_SLOT*SLOTS_PER_EPOCH)+BUFFER"`

// Multi-dimensional
`dynssz-max:"SHARD_COUNT,MAX_SHARD_BLOCK_SIZE*2"`
```

Spec values are provided when creating the `DynSsz` instance:

```go
specs := map[string]any{
    "CHUNK_SIZE":                 uint64(32),
    "SLOTS_PER_EPOCH":           uint64(32),
    "MAX_COMMITTEES_PER_SLOT":   uint64(64),
}
ds := dynssz.NewDynSsz(specs)
```

## Tag Combinations

### Valid Combinations

```go
type Valid struct {
    // Static size + dynamic override
    Data []byte `ssz-size:"32" dynssz-size:"CHUNK_SIZE"`

    // Static max + dynamic override (recommended pattern)
    Items []uint64 `ssz-max:"1024" dynssz-max:"MAX_LIST_SIZE"`

    // Bitvector with bitsize for padding validation
    Bits [2]byte `ssz-type:"bitvector" ssz-bitsize:"12"`

    // Dynamic bitvector with static fallback
    DynBits []byte `ssz-type:"bitvector" ssz-bitsize:"512" dynssz-bitsize:"COMMITTEE_SIZE"`

    // Progressive type with dynamic max
    Validators []Validator `ssz-type:"progressive-list" ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}
```

### Invalid Combinations

```go
type Invalid struct {
    // Cannot use both ssz-size and ssz-max (a field is either fixed or variable)
    Bad []byte `ssz-size:"32" ssz-max:"64"`

    // A dimension declared two ways: a 2-vector statically, dynamic dynamically
    Bad2 [2][]byte `ssz-size:"2,6" dynssz-size:"?,COLS"`

    // Same for limits: bounded statically, unbounded dynamically
    Bad3 [][]uint64 `ssz-max:"16,8" dynssz-max:"?,ROW_LIMIT"`

    // A dimension with neither a length nor a limit
    Bad4 [][]byte `ssz-size:"?,6" ssz-max:"?,8"`
}
```

See [the `?` placeholder](#the--placeholder) for the rules behind the last
three.

## Common Patterns

### Ethereum Types

```go
type EthereumTypes struct {
    BlockRoot  []byte      `ssz-size:"32"`     // 32-byte hash
    Signature  []byte      `ssz-size:"96"`     // BLS signature
    PublicKey  []byte      `ssz-size:"48"`     // BLS public key
    Validators []Validator `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
}
```

### Bitfields

```go
type Bitfields struct {
    Flags         [32]byte         `ssz-type:"bitvector"`
    CommitteeBits [2]byte          `ssz-type:"bitvector" ssz-bitsize:"12"`
    SyncBits      []byte           `ssz-type:"bitvector" ssz-bitsize:"512" dynssz-bitsize:"SYNC_COMMITTEE_SIZE"`
    Votes         []byte           `ssz-type:"bitlist" ssz-max:"2048"`
    Attestation   bitfield.Bitlist `ssz-max:"2048"`
}
```

### Pointer Fields

```go
type WithPointers struct {
    Value   uint64
    Pointer *uint64           // treated as regular field; initialized if nil on unmarshal
    Feature *Feature `ssz-index:"100"` // progressive container field
}
```

### Nested Arrays

```go
type Nested struct {
    Matrix [][]uint32 `ssz-max:"100,256"`
    Hashes [][32]byte `ssz-max:"1024"`
    Data   [][]byte   `ssz-max:"64,1073741824" dynssz-max:"SHARD_COUNT,MAX_SHARD_BLOCK_SIZE"`
}
```

## Troubleshooting

### "Spec value not found" / expression not resolved

The `dynssz-*` expression references a spec value that was not provided. If an `ssz-*` fallback exists, it will be used. If not, the operation may fail.

```go
// Ensure all referenced values are provided
specs := map[string]any{
    "MISSING_VALUE": uint64(100),
}
ds := dynssz.NewDynSsz(specs)
```

### "Invalid expression" Error

Check expression syntax:
```go
// Valid
`dynssz-max:"VALUE"`
`dynssz-max:"VALUE*2"`
`dynssz-max:"(A+B)*C"`

// Invalid
`dynssz-max:"VALUE^2"`      // no exponentiation
`dynssz-max:"VALUE/2.5"`    // no floats
`dynssz-max:"VALUE OR 100"` // no logical operators
```

### Missing ssz-max for dynamic arrays

Dynamic arrays without `ssz-max` have security implications for hash tree root computation. Always specify a maximum size for production code.

## Related Documentation

- [Supported Types](supported-types.md) - Complete type reference
- [Getting Started](getting-started.md) - Basic usage
- [Code Generator](code-generator.md) - Code generation
- [API Reference](api-reference.md) - Runtime APIs
