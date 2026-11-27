# API Reference

This reference documents all public types, methods, and interfaces in Dynamic SSZ.

## Core Types

### DynSsz

The main entry point for all SSZ operations.

#### Constructor

```go
func NewDynSsz(specs map[string]any, options ...DynSszOption) *DynSsz
```

Creates a new DynSsz instance.

**Parameters**:
- `specs` - Map of specification values for dynamic expressions (can be `nil`)
- `options` - Optional functional options (see [Constructor Options](#constructor-options))

```go
// Basic usage
ds := dynssz.NewDynSsz(nil)

// With spec values
specs := map[string]any{
    "VALIDATOR_REGISTRY_LIMIT": uint64(1099511627776),
    "MAX_ATTESTATIONS":         uint64(128),
}
ds := dynssz.NewDynSsz(specs)

// With options
ds := dynssz.NewDynSsz(specs,
    dynssz.WithExtendedTypes(),
    dynssz.WithVerbose(),
)
```

### Constructor Options

```go
type DynSszOption func(*DynSszOptions)
```

| Option | Description |
|--------|-------------|
| `WithNoFastSsz()` | Disable fastssz fallback, force all operations through reflection |
| `WithNoFastHash()` | Disable the accelerated hashing libraries: a built-in hash algorithm is replaced by its pure Go counterpart |
| `WithDefaultHashAlgorithm(alg)` | Hash algorithm `HashTreeRoot` and `GetTree` merkleize with when a call names none (default `hasher.SHA256`); see [Hash algorithms](#hash-algorithms) |
| `WithExtendedTypes()` | Enable non-standard types (signed ints, floats, big.Int, optionals) |
| `WithVerbose()` | Enable verbose debug logging |
| `WithLogCb(fn)` | Set a custom logging callback (`func(format string, args ...any)`) |
| `WithStreamWriterBufferSize(n)` | Set stream encoder buffer size (default 2KB) |
| `WithStreamReaderBufferSize(n)` | Set stream decoder buffer size (default 2KB) |
| `WithNoDelegation()` | Disable delegation to generated Dynamic* methods (fastssz is governed by `WithNoFastSsz`) |
| `WithMaxStreamSize(n)` | Cap an unknown-length `UnmarshalSSZReader` decode (default 512MB); a declared size is its own bound; `WithStreamSizeLimit` overrides it per call |
| `WithMaxNestingDepth(n)` | Bound recursion-cycle nesting (default 1024); guards the stack against deeply nested payloads |
| `WithAsyncHashing(workers)` | Opt this instance's `HashTreeRoot` into background subtree reduction; `workers > 0` also sets the process-wide worker limit (see [Async hashing](performance.md#2-async-hashing)) |

### Global Instance

For legacy compatibility methods generated with `-legacy`, a global `DynSsz` instance is used:

```go
// Get (or create) the global instance
func GetGlobalDynSsz() *DynSsz

// Replace the global instance with new specs
func SetGlobalSpecs(specs map[string]any)
```

### Call Options

Per-call configuration for SSZ operations:

```go
type CallOption func(*callConfig)
```

#### WithViewDescriptor

```go
func WithViewDescriptor(view any) CallOption
```

Specifies a view descriptor for fork-dependent SSZ schemas. The view defines the SSZ layout while data is read from/written to the runtime object.

```go
data, err := ds.MarshalSSZ(body, dynssz.WithViewDescriptor(&Phase0BodyView{}))

// Nil pointer works too (only type information is used)
data, err = ds.MarshalSSZ(body, dynssz.WithViewDescriptor((*Phase0BodyView)(nil)))
```

See [SSZ Views](views.md) for detailed documentation.

#### WithStreamSizeLimit

```go
func WithStreamSizeLimit(size int) CallOption
```

Bounds one unknown-length `UnmarshalSSZReader` decode, overriding `WithMaxStreamSize` for that call. A non-positive value keeps the instance default; a declared size is its own bound and is not affected. Other operations ignore the option.

```go
err := ds.UnmarshalSSZReader(&block, body, -1, dynssz.WithStreamSizeLimit(4*1024*1024))
```

#### WithHashAlgorithm

```go
func WithHashAlgorithm(alg *hasher.Algorithm) CallOption
```

Selects the hash algorithm one `HashTreeRoot` or `GetTree` call merkleizes with, overriding `WithDefaultHashAlgorithm`. `HashTreeRootWith` hashes with the walker it is given; there the option only checks the walker and fails with `ErrHashAlgorithmMismatch` when the walker computes another hash. Other operations ignore the option.

```go
root, err := ds.HashTreeRoot(block, dynssz.WithHashAlgorithm(hasher.Keccak256))
```

See [Hash algorithms](#hash-algorithms).

## Serialization Methods

### MarshalSSZ

```go
func (d *DynSsz) MarshalSSZ(source any, opts ...CallOption) ([]byte, error)
```

Serializes an object to SSZ format. Automatically delegates to generated methods when available.

### MarshalSSZTo

```go
func (d *DynSsz) MarshalSSZTo(source any, buf []byte, opts ...CallOption) ([]byte, error)
```

Serializes to SSZ format using a provided buffer for reduced allocations.

```go
buf := make([]byte, 0, 1024)
data, err := ds.MarshalSSZTo(myStruct, buf)
```

### UnmarshalSSZ

```go
func (d *DynSsz) UnmarshalSSZ(target any, ssz []byte, opts ...CallOption) error
```

Deserializes from SSZ format. `target` must be a pointer.

```go
var decoded MyStruct
err := ds.UnmarshalSSZ(&decoded, data)
```

**Decoding reuses what the target already holds.** A slice long enough for the
result keeps its backing array, a non-nil pointer field is decoded into rather
than replaced, and a non-nil pointer element of a list of fixed-size elements
is decoded into on the buffer and known-size stream paths. This saves
allocations when a target is decoded into repeatedly. Elements are allocated
fresh, and an existing element and its excluded fields are dropped, in these
cases: elements of a list of variable-size elements in the reflection engine,
elements of any list on an unknown-size stream, and optional values in
generated code.

The consequence is that a decode writes through to objects the caller still
references:

```go
first := decoded.Items[0]      // caller keeps a reference
err = ds.UnmarshalSSZ(&decoded, other)
// first now holds values from `other`
```

Decode into a target you own. To keep an earlier result, either decode into a
fresh value or copy what you need out first. A successful decode always
produces exactly the value the input describes — reuse keeps allocations, never
data.

## Streaming Methods

### MarshalSSZWriter

```go
func (d *DynSsz) MarshalSSZWriter(source any, w io.Writer, opts ...CallOption) error
```

Serializes directly to an `io.Writer` for memory-efficient streaming.

### UnmarshalSSZReader

```go
func (d *DynSsz) UnmarshalSSZReader(target any, r io.Reader, size int, opts ...CallOption) error
```

Deserializes from an `io.Reader`. The `size` parameter specifies the expected total SSZ data size in bytes. A non-negative size is trusted input: allocations are sized from it before the bytes arrive, so it must come from a source you control (a local file's `stat()` result, a length your own protocol validated), never from an untrusted peer. Pass a negative size for input read off the wire and bound it with `WithMaxStreamSize` or `WithStreamSizeLimit`.

See [Streaming Support](streaming.md) for details.

## Hash Tree Root

### HashTreeRoot

```go
func (d *DynSsz) HashTreeRoot(source any, opts ...CallOption) ([32]byte, error)
```

Computes the SSZ hash tree root.

### GetTree

```go
func (d *DynSsz) GetTree(source any, opts ...CallOption) (*treeproof.Node, error)
```

Builds the complete Merkle tree for proof generation. The returned tree is
finalized (every branch hash cached), so it is read-only and immediately safe
for concurrent proof generation. On a finalization error the partially
finalized tree is returned alongside the error. The tree knows the hash
algorithm it was built for (`tree.Algorithm()`) and uses it for every proof.

```go
tree, err := ds.GetTree(myStruct)

// Display tree structure
tree.Show(3)

// Generate proof for field at generalized index
proof, err := tree.Prove(7)

// Verify proof
isValid, err := treeproof.VerifyProof(tree.Hash(), proof)
```

See [Merkle Proofs](merkle-proofs.md) for complete documentation.

### Hash algorithms

The tree shape of a value is the same under every hash; only the node hashes
differ. A `*hasher.Algorithm` is one such hash: a function that compresses two
32-byte chunks into one, together with the zero subtree roots and the hasher
pool derived from it.

| Algorithm | Hash | Backend |
|-----------|------|---------|
| `hasher.SHA256` | SHA-256, the hash of the consensus specification (default) | [hashtree](https://github.com/OffchainLabs/hashtree) |
| `hasher.SHA256Native` | SHA-256 | Go standard library |
| `hasher.Keccak256` | Ethereum Keccak-256 (padding `0x01`, not NIST SHA3-256) | [keccaktree](https://github.com/pk910/keccaktree) |
| `hasher.Keccak256Native` | Ethereum Keccak-256 | pure Go |

Any other hash is used through the algorithm of its function:

```go
// fn hashes consecutive 64-byte pairs of input into dst
alg, err := hasher.AlgorithmFor(fn)
```

`AlgorithmFor` returns one algorithm for functions that run the same code and
compute the same hash, and the built-in algorithm for a built-in function. The
function has to be deterministic, and an algorithm is kept for the life of the
process, so make it once per hash rather than per use. Finding it costs one
call of the function, and 64 for a hash no algorithm computes yet: its zero
subtree roots are computed then. Those of SHA-256 and Keccak-256 are computed
when they are first needed, and a function that computes one of these hashes
shares them. `hasher.NewHasherWithHashFn(fn)` and a `hasher.HasherPool` made
around a function derive their algorithm from that function in the same way,
per hasher or per pool.

Two algorithms are compatible when they compute the same hash, whatever
computes it: `alg.Compatible(other)`. `hasher.SHA256` and
`hasher.SHA256Native` are compatible, and so is the algorithm of any other
SHA-256 function.

The algorithm is a property of the value, not of the process: a chain that
changes its hash at a fork keeps hashing older values with the previous one.
A `DynSsz` instance has a default, and a call can name another:

```go
ds := dynssz.NewDynSsz(specs) // hashes with SHA-256

root, err := ds.HashTreeRoot(block)                                           // SHA-256
root, err = ds.HashTreeRoot(block, dynssz.WithHashAlgorithm(hasher.Keccak256)) // Keccak-256
tree, err := ds.GetTree(block, dynssz.WithHashAlgorithm(hasher.Keccak256))

// An instance for a network that hashes with Keccak-256 throughout
keccakDs := dynssz.NewDynSsz(specs, dynssz.WithDefaultHashAlgorithm(hasher.Keccak256))
```

Where a caller supplies the hash walker, the walker carries the algorithm:

```go
hh := hasher.Keccak256.Pool().Get()
defer hasher.Keccak256.Pool().Put(hh)

err := ds.HashTreeRootWith(block, hh)
root, err := hh.HashRoot()
```

The methods a type carries itself take no walker and no option: a generated
`HashTreeRoot()` or `HashTreeRootDyn(ds)` always hashes with SHA-256. The root
of a value under another algorithm is computed by `ds.HashTreeRoot` with the
option, which runs the same generated code on a walker of that algorithm.

Regenerate generated code before hashing with another algorithm. Code from a
generator that predates hash algorithms calls the argument-less
`HashTreeRoot()` of such a child without asking for the algorithm, so its
SHA-256 root ends up in a tree of another hash, and no error reports it.
Roots under SHA-256 are not affected.

A hand-written argument-less `HashTreeRoot()` likewise computes its root on
its own, with SHA-256. Under another algorithm both engines treat a child
that offers only this method as follows:

| Child | Under another algorithm |
|-------|-------------------------|
| A custom type | the method is called: a custom type answers for its 32-byte root, whatever it does to produce it |
| Its root takes no hashing: a basic value, or a vector of basic values or a bitvector of up to 32 bytes | the method is called |
| Any other type | its structure is walked instead |

A custom type that should follow the algorithm implements
`HashTreeRootWith(hh)` and hashes into the walker.

A child that implements `HashTreeRootWith(hh)` or
`HashTreeRootWithDyn(ds, hh)` hashes into the walker and works under every
algorithm.

The package-level `hasher.GetZeroHash`, `hasher.FastHasherPool`,
`hasher.DefaultHasherPool` and `hasher.WithDefaultHasher` are those of SHA-256.
`hasher.AlgorithmOf(hh)` reports the algorithm of a walker, which a cache of
roots needs as part of its key.

## Utility Methods

### SizeSSZ

```go
func (d *DynSsz) SizeSSZ(source any, opts ...CallOption) (int, error)
```

Calculates serialized size without encoding.

For a value that encodes, the size is exact — the length `MarshalSSZ` produces,
and both engines agree on it. For a value that does not encode, the number means
nothing and depends on which engine ran: a type with generated code sizes itself
through `DynamicSizer`, which returns a bare `int` and so cannot report the
problem, while the reflection engine returns an error. Use this to size a
buffer, not to decide whether a value is encodable — `MarshalSSZ` rejects such a
value either way.

### ValidateType

```go
func (d *DynSsz) ValidateType(t reflect.Type, opts ...CallOption) error
```

Validates that a type can be SSZ-serialized. When a view descriptor is provided, also validates view compatibility.

```go
err := ds.ValidateType(reflect.TypeOf(MyStruct{}))

// With view validation
err = ds.ValidateType(
    reflect.TypeOf(BeaconState{}),
    dynssz.WithViewDescriptor(&Phase0StateView{}),
)
```

### ResolveSpecValue

```go
func (d *DynSsz) ResolveSpecValue(name string) (bool, uint64, error)
```

Resolves a specification value by name or expression. Results are cached.

Expressions support integer arithmetic only: `+ - * / %` and parentheses over
unsigned integer literals and spec identifiers, evaluated with exact uint64
precision across the full value range. Division rounds up (ceil), since
partial bytes/bits cannot be serialized.

**Returns**: whether the value was found, the resolved value, and any parse error.

### GetTypeCache

```go
func (d *DynSsz) GetTypeCache() *ssztypes.TypeCache
```

Returns the internal type cache. Useful for debugging. Do not depend on the internal structure of TypeCache - it may change between versions.

## Custom Marshaling Interfaces

Types can implement these interfaces for custom SSZ behavior. The `DynSsz` runtime checks for these interfaces in order of specificity.

### Standard Interfaces (fastssz compatible)

Types generated by fastssz's `sszgen` are recognized through these interfaces
from the `sszutils` package. Each names one method and is looked for on its own,
so a type is delegated to for the operations it can serve and walked for the
rest: a type with only `SizeSSZ` sizes itself and is marshalled by the library,
and one with only `MarshalSSZ` marshals itself into a buffer the library
appends.

```go
type FastsszBufferMarshaler interface {
    MarshalSSZTo(dst []byte) ([]byte, error)
}

type FastsszValueMarshaler interface {
    MarshalSSZ() ([]byte, error)
}

type FastsszSizer interface {
    SizeSSZ() int
}

type FastsszUnmarshaler interface {
    UnmarshalSSZ(buf []byte) error
}

type FastsszHashRoot interface {
    HashTreeRoot() ([32]byte, error)
}
```

`FastsszMarshaler` is the three marshalling interfaces together, which is the
set `sszgen` emits. Marshalling prefers `MarshalSSZTo` over `MarshalSSZ`, since
appending to the caller's buffer costs neither an allocation nor a copy.

Whichever methods a type provides are used only where they can be right: a type
whose size or limit comes from a spec value is walked instead, because its own
methods have one preset baked in. Methods reached through an embedded field are
never used either -- they answer for the embedded value, not for the type that
promoted them.

A `HashTreeRootWith(hh) error` method is detected by name and used instead
of `HashTreeRoot()` when its parameter is an interface that the library's
`HashWalker` satisfies (such as fastssz's `ssz.HashWalker`). The
`HashTreeRootWith(*ssz.Hasher)` form that `sszgen` emits by default takes a
concrete foreign type, so it is not called; `HashTreeRoot()` is used instead.

Hash methods are called wherever the value sits. A list or vector of basic
values opens a packed walker scope (`StartTree` with `sszutils.TreeTypePacked`
set on the shape), in which the walker's `Put*` methods append the packed bytes
of a value instead of a padded chunk; the same method therefore packs its value
as a list element and leaves a chunk as a field.

A method owns what it leaves on the walker, and the engines check one thing
about it: a scope reduced with more chunks than its limit holds is refused with
`sszutils.ErrChunkLimitExceeded`. A method that merkleizes a leaf of its own
inside a packed scope contributes a whole chunk where the scope holds packed
bytes, so a list of such elements states a limit its own contents pass. Both
walkers report it, and both report it at every size. Anything else a method
leaves is unverified, and both walkers lay bytes out identically, so it
produces the same root through `HashTreeRoot` and through `GetTree`.

### Dynamic Interfaces (spec-aware)

These are the primary interfaces used by generated code. They receive the `DynSsz` instance as a `DynamicSpecs` for resolving spec values:

```go
type DynamicMarshaler interface {
    MarshalSSZDyn(ds DynamicSpecs, buf []byte) ([]byte, error)
}

type DynamicUnmarshaler interface {
    UnmarshalSSZDyn(ds DynamicSpecs, buf []byte) error
}

type DynamicSizer interface {
    SizeSSZDyn(ds DynamicSpecs) int
}

type DynamicHashRoot interface {
    HashTreeRootWithDyn(ds DynamicSpecs, hh HashWalker) error
}
```

### Streaming Interfaces

```go
type DynamicEncoder interface {
    MarshalSSZEncoder(specs DynamicSpecs, encoder Encoder) error
}

type DynamicDecoder interface {
    UnmarshalSSZDecoder(specs DynamicSpecs, decoder Decoder) error
}
```

### View Interfaces

Enable a single runtime type to support multiple SSZ schemas. Typically implemented via code generation. Methods return `nil` if the view type is not recognized, causing fallback to reflection.

```go
type DynamicViewMarshaler interface {
    MarshalSSZDynView(view any) func(ds DynamicSpecs, buf []byte) ([]byte, error)
}

type DynamicViewUnmarshaler interface {
    UnmarshalSSZDynView(view any) func(ds DynamicSpecs, buf []byte) error
}

type DynamicViewSizer interface {
    SizeSSZDynView(view any) func(ds DynamicSpecs) int
}

type DynamicViewHashRoot interface {
    HashTreeRootWithDynView(view any) func(ds DynamicSpecs, hh HashWalker) error
}

type DynamicViewEncoder interface {
    MarshalSSZEncoderView(view any) func(ds DynamicSpecs, encoder Encoder) error
}

type DynamicViewDecoder interface {
    UnmarshalSSZDecoderView(view any) func(ds DynamicSpecs, decoder Decoder) error
}
```

See [SSZ Views](views.md) for details.

### DynamicSpecs Interface

```go
type DynamicSpecs interface {
    ResolveSpecValue(name string) (bool, uint64, error)
}
```

Provides access to specification values during marshaling. The `DynSsz` type implements this interface.

### Encoder / Decoder Interfaces

These abstract over buffer-based and stream-based encoding:

```go
type Encoder interface {
    Seekable() bool
    GetPosition() int
    GetBuffer() []byte
    SetBuffer(buffer []byte)
    EncodeBool(v bool)
    EncodeUint8(v uint8)
    EncodeUint16(v uint16)
    EncodeUint32(v uint32)
    EncodeUint64(v uint64)
    EncodeBytes(v []byte)
    EncodeOffset(v uint32)
    EncodeOffsetAt(pos int, v uint32)  // not supported for streams
    EncodeZeroPadding(n int)
}

type Decoder interface {
    Seekable() bool
    GetPosition() int
    GetLength() int
    PushLimit(limit int)
    PopLimit() int
    DecodeBool() (bool, error)
    DecodeUint8() (uint8, error)
    DecodeUint16() (uint16, error)
    DecodeUint32() (uint32, error)
    DecodeUint64() (uint64, error)
    DecodeBytes(buf []byte) ([]byte, error)
    DecodeBytesBuf(len int) ([]byte, error)
    DecodeOffset() (uint32, error)
    DecodeOffsetAt(pos int) uint32     // not supported for streams
    SkipBytes(n int)                   // not supported for streams
}
```

See [Streaming Support](streaming.md) for details.

## Type Wrapper API

### TypeWrapper[D, T]

Generic wrapper for applying SSZ annotations to non-struct values.

```go
type TypeWrapper[D, T any] struct {
    Data T
}
```

**Type Parameters**:
- `D` - Descriptor struct with exactly one field carrying SSZ tags. The field type must match `T`.
- `T` - Wrapped value type.

**Methods**:
```go
func (w *TypeWrapper[D, T]) Get() T
func (w *TypeWrapper[D, T]) Set(value T)
func (w *TypeWrapper[D, T]) GetDescriptorType() reflect.Type
```

See [Type Wrapper](type-wrapper.md) for usage details.

## Union API

### Union[T]

Generic classic SSZ union type (`Union[type_0, type_1, ...]` from the SSZ specification).

```go
type Union[T any] struct {
    Variant uint8
    Data    interface{}
}
```

**Type Parameters**:
- `T` - Descriptor struct defining union variants as fields. Selectors are the 0-based field positions; `ssz-index` tags are not allowed and selectors above 127 are reserved.

**Constructor**:
```go
func NewUnion[T any](variantIndex uint8, data interface{}) (*Union[T], error)
```

**Usage**:
```go
type PayloadUnion = dynssz.Union[struct {
    Legacy ExecutionPayload // selector 0
    Full   FullPayload      // selector 1
}]

payload := PayloadUnion{
    Variant: 1,
    Data: FullPayload{...},
}
```

### None

Marker type declaring the empty option of a classic union. Declared as the first descriptor field, it makes selector 0 the None variant (`{Variant: 0, Data: nil}`, serialized as the single byte `0x00`):

```go
type None struct{}
```

```go
type MaybePayload = dynssz.Union[struct {
    None dynssz.None      // selector 0: no value
    Full ExecutionPayload // selector 1
}]
```

## Compatible Union API

### CompatibleUnion[T]

Generic union type for variant values (EIP-8016).

```go
type CompatibleUnion[T any] struct {
    Variant uint8
    Data    interface{}
}
```

**Type Parameters**:
- `T` - Descriptor struct defining union variants as fields. Selectors follow field order starting at 1 (EIP-8016 allows 1..127); `ssz-index` tags assign them explicitly.

**Constructor**:
```go
func NewCompatibleUnion[T any](variantIndex uint8, data interface{}) (*CompatibleUnion[T], error)
```

**Usage**:
```go
type PayloadUnion = dynssz.CompatibleUnion[struct {
    ExecutionPayload
    ExecutionPayloadWithBlobs
}]

payload := PayloadUnion{
    Variant: 1,
    Data: ExecutionPayload{...},
}
```

## Code Generator API

### CodeGenerator

```go
func NewCodeGenerator(typeCache *ssztypes.TypeCache) *CodeGenerator
// the cache is read-only: only its extended-types setting is inherited; nil for the default

func (cg *CodeGenerator) BuildFile(fileName string, opts ...CodeGeneratorOption)
func (cg *CodeGenerator) Generate() error
func (cg *CodeGenerator) GenerateToMap() (map[string]string, error)
```

### CodeGeneratorOption

```go
// Method generation control
func WithNoMarshalSSZ() CodeGeneratorOption
func WithNoUnmarshalSSZ() CodeGeneratorOption
func WithNoSizeSSZ() CodeGeneratorOption
func WithNoHashTreeRoot() CodeGeneratorOption
func WithCreateLegacyFn() CodeGeneratorOption
func WithoutDynamicExpressions() CodeGeneratorOption
func WithNoFastSsz() CodeGeneratorOption
func WithCreateEncoderFn() CodeGeneratorOption
func WithCreateDecoderFn() CodeGeneratorOption
func WithExtendedTypes() CodeGeneratorOption

// Type specification
func WithReflectType(t reflect.Type, typeOpts ...CodeGeneratorOption) CodeGeneratorOption
func WithGoTypesType(t types.Type, typeOpts ...CodeGeneratorOption) CodeGeneratorOption

// View support (nested within WithReflectType/WithGoTypesType)
func WithReflectViewTypes(views ...reflect.Type) CodeGeneratorOption
func WithGoTypesViewTypes(views ...types.Type) CodeGeneratorOption
func WithViewOnly() CodeGeneratorOption

// Hint options
func WithSizeHints(hints []dynssz.SszSizeHint) CodeGeneratorOption
func WithMaxSizeHints(hints []dynssz.SszMaxSizeHint) CodeGeneratorOption
func WithTypeHints(hints []dynssz.SszTypeHint) CodeGeneratorOption
```

See [Code Generator](code-generator.md) for detailed usage.

## Error Types

Every error the library returns wraps one of these sentinels, so `errors.Is`
tells a caller what happened. They do not wrap each other: a value refused for
its size does not match `ErrInvalidValueRange`.

```go
var (
    // The encoding does not say what the type does
    ErrUnexpectedEOF       = fmt.Errorf("unexpected end of SSZ")
    ErrOffset              = fmt.Errorf("incorrect offset")
    ErrListTooBig          = fmt.Errorf("list length is higher than max value")
    ErrVectorLength        = fmt.Errorf("incorrect vector length")
    ErrInvalidValueRange   = fmt.Errorf("invalid value range")
    ErrInvalidUnionVariant = ErrInvalidValueRange // alias: matches and prints as "invalid value range"
    ErrBitlistNotTerminated = ErrInvalidValueRange // alias

    // The value states a size nothing can encode, or this target cannot hold
    ErrSszSizeExceeded     = fmt.Errorf("ssz size exceeds the maximum encodable size")
    ErrPlatformOverflow    = fmt.Errorf("value exceeds platform integer range")

    // A hash walk was given something it cannot reduce
    ErrChunkLimitExceeded  = fmt.Errorf("merkleized more chunks than the limit holds")
    ErrScopeShapeMismatch  = fmt.Errorf("scope reduced in a shape it was not opened for")
    ErrHashAlgorithmMismatch = fmt.Errorf("hash walker compresses with another hash algorithm")

    // The input exceeds a bound the caller set
    ErrMaxDepthExceeded    = fmt.Errorf("maximum nesting depth exceeded")
    ErrStreamTooLarge      = fmt.Errorf("ssz stream exceeds maximum size")
    ErrPayloadTooLarge     = fmt.Errorf("%w", ErrStreamTooLarge)
    ErrNegativeRead        = fmt.Errorf("reader returned an invalid byte count")

    // The type cannot be described
    ErrUnsupportedType     = fmt.Errorf("unsupported type")
    ErrTypeMismatch        = fmt.Errorf("type mismatch")
    ErrInvalidTag          = fmt.Errorf("invalid tag")
    ErrInvalidConstraint   = fmt.Errorf("invalid constraint")
    ErrExtendedTypeDisabled = fmt.Errorf("extended type not enabled")
    ErrMissingInterface    = fmt.Errorf("missing interface")
    ErrNotImplemented      = fmt.Errorf("not implemented")
)
```

`ErrSszSizeExceeded` and `ErrChunkLimitExceeded` are new in v1.4.0. Sizes past
the SSZ size limit previously reported `ErrInvalidValueRange`; a caller matching
that for an over-size condition matches `ErrSszSizeExceeded` now.

## Related Documentation

- [Getting Started](getting-started.md) - Introduction and basics
- [Supported Types](supported-types.md) - Type system reference
- [SSZ Annotations](ssz-annotations.md) - Tag documentation
- [SSZ Views](views.md) - Fork handling with view descriptors
- [Code Generator](code-generator.md) - Code generation tools
- [Streaming Support](streaming.md) - Streaming encoding/decoding
