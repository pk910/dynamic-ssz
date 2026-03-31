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
| `WithNoFastHash()` | Disable accelerated hashtree hashing library |
| `WithExtendedTypes()` | Enable non-standard types (signed ints, floats, big.Int, optionals) |
| `WithVerbose()` | Enable verbose debug logging |
| `WithLogCb(fn)` | Set a custom logging callback (`func(format string, args ...any)`) |
| `WithStreamWriterBufferSize(n)` | Set stream encoder buffer size (default 2KB) |
| `WithStreamReaderBufferSize(n)` | Set stream decoder buffer size (default 2KB) |

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

Deserializes from an `io.Reader`. The `size` parameter specifies the expected total SSZ data size in bytes.

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

Builds the complete Merkle tree for proof generation.

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

## Utility Methods

### SizeSSZ

```go
func (d *DynSsz) SizeSSZ(source any, opts ...CallOption) (int, error)
```

Calculates serialized size without encoding.

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

**Returns**: whether the value was found, the resolved value, and any parse error.

### GetTypeCache

```go
func (d *DynSsz) GetTypeCache() *ssztypes.TypeCache
```

Returns the internal type cache. Useful for debugging. Do not depend on the internal structure of TypeCache - it may change between versions.

## Custom Marshaling Interfaces

Types can implement these interfaces for custom SSZ behavior. The `DynSsz` runtime checks for these interfaces in order of specificity.

### Standard Interfaces (fastssz compatible)

```go
type Marshaler interface {
    MarshalSSZ() ([]byte, error)
}

type MarshalerTo interface {
    MarshalSSZTo(buf []byte) ([]byte, error)
}

type Unmarshaler interface {
    UnmarshalSSZ(buf []byte) error
}

type Sizer interface {
    SizeSSZ() int
}

type HashRoot interface {
    HashTreeRoot() ([32]byte, error)
}

type HashRootWith interface {
    HashTreeRootWith(hh HashWalker) error
}
```

### Dynamic Interfaces (spec-aware)

These are the primary interfaces used by generated code. They receive the `DynSsz` instance as a `DynamicSpecs` for resolving spec values:

```go
type DynamicMarshaler interface {
    MarshalSSZDyn(specs DynamicSpecs, buf []byte) ([]byte, error)
}

type DynamicUnmarshaler interface {
    UnmarshalSSZDyn(specs DynamicSpecs, buf []byte) error
}

type DynamicSizer interface {
    SizeSSZDyn(specs DynamicSpecs) int
}

type DynamicHashRoot interface {
    HashTreeRootDyn(specs DynamicSpecs) ([32]byte, error)
}

type DynamicHashRootWith interface {
    HashTreeRootWithDyn(specs DynamicSpecs, hh HashWalker) error
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

## Compatible Union API

### CompatibleUnion[T]

Generic union type for variant values (EIP-7495).

```go
type CompatibleUnion[T any] struct {
    Variant uint8
    Data    interface{}
}
```

**Type Parameters**:
- `T` - Descriptor struct defining union variants as fields (field order = variant index)

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
    Variant: 0,
    Data: ExecutionPayload{...},
}
```

## Code Generator API

### CodeGenerator

```go
func NewCodeGenerator(dynSsz *dynssz.DynSsz) *CodeGenerator

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

Common errors from the `sszutils` package:

```go
var (
    ErrListTooBig          = fmt.Errorf("list length is higher than max value")
    ErrUnexpectedEOF       = fmt.Errorf("unexpected end of SSZ")
    ErrOffset              = fmt.Errorf("incorrect offset")
    ErrInvalidUnionVariant = fmt.Errorf("invalid union variant")
    ErrVectorLength        = fmt.Errorf("incorrect vector length")
    ErrNotImplemented      = fmt.Errorf("not implemented")
)
```

## Related Documentation

- [Getting Started](getting-started.md) - Introduction and basics
- [Supported Types](supported-types.md) - Type system reference
- [SSZ Annotations](ssz-annotations.md) - Tag documentation
- [SSZ Views](views.md) - Fork handling with view descriptors
- [Code Generator](code-generator.md) - Code generation tools
- [Streaming Support](streaming.md) - Streaming encoding/decoding
