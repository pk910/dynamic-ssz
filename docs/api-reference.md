# API Reference

This reference documents all public interfaces and methods in Dynamic SSZ.

## Core Types

### DynSsz

The main entry point for Dynamic SSZ operations.

```go
type DynSsz struct {
    // Internal fields
}
```

#### Constructor

```go
func NewDynSsz(specs map[string]any) *DynSsz
```

Creates a new DynSsz instance.

**Parameters**:
- `specs` - Map of specification values for dynamic expressions (can be nil)

**Example**:
```go
// Basic usage
ssz := dynssz.NewDynSsz(nil)

// With spec values
specs := map[string]any{
    "VALIDATOR_REGISTRY_LIMIT": 1099511627776,
    "MAX_ATTESTATIONS": 128,
}
ssz := dynssz.NewDynSsz(specs)
```

## Call Options

### CallOption

Per-call configuration for SSZ operations.

```go
type CallOption func(*callConfig)
```

### WithViewDescriptor

```go
func WithViewDescriptor(view any) CallOption
```

Specifies a view descriptor for fork-dependent SSZ schemas.

The view descriptor defines the SSZ layout (field order, tags, sizes) while the actual data is read from/written to the runtime object. This enables a single runtime type to support multiple SSZ representations for different forks.

**Parameters**:
- `view` - A struct or pointer to struct defining the SSZ schema. Its fields are mapped to the runtime type's fields by name. The view's field values are not used; only its type information matters.

**Example**:
```go
// Define view types for different forks
type Phase0BeaconBlockBodyView struct {
    RANDAOReveal [96]byte `ssz-size:"96"`
    // ... Phase0 fields only
}

type AltairBeaconBlockBodyView struct {
    RANDAOReveal  [96]byte `ssz-size:"96"`
    SyncAggregate *SyncAggregateView
    // ... Altair fields
}

// Use views in SSZ operations
data, err := ssz.MarshalSSZ(body, dynssz.WithViewDescriptor(&Phase0BeaconBlockBodyView{}))

err = ssz.UnmarshalSSZ(&body, data, dynssz.WithViewDescriptor(&AltairBeaconBlockBodyView{}))

root, err := ssz.HashTreeRoot(body, dynssz.WithViewDescriptor(&AltairBeaconBlockBodyView{}))

// Nil pointer works too (only type information is used)
data, err = ssz.MarshalSSZ(body, dynssz.WithViewDescriptor((*Phase0BeaconBlockBodyView)(nil)))
```

See [SSZ Views](views.md) for detailed view documentation.

## Serialization Methods

### MarshalSSZ

```go
func (d *DynSsz) MarshalSSZ(source any, opts ...CallOption) ([]byte, error)
```

Serializes an object to SSZ format.

**Parameters**:
- `source` - Object to serialize
- `opts` - Optional call options (e.g., `WithViewDescriptor`)

**Returns**:
- Serialized bytes
- Error if serialization fails

**Example**:
```go
data, err := ssz.MarshalSSZ(myStruct)
if err != nil {
    return err
}

// With view descriptor
data, err = ssz.MarshalSSZ(myStruct, dynssz.WithViewDescriptor(&MyView{}))
```

### MarshalSSZTo

```go
func (d *DynSsz) MarshalSSZTo(source any, buf []byte, opts ...CallOption) ([]byte, error)
```

Serializes to SSZ format using provided buffer.

**Parameters**:
- `source` - Object to serialize
- `buf` - Buffer to write to (can be nil)
- `opts` - Optional call options (e.g., `WithViewDescriptor`)

**Returns**:
- Serialized bytes (may be same as buf if large enough)
- Error if serialization fails

**Example**:
```go
buf := make([]byte, 0, 1024)
data, err := ssz.MarshalSSZTo(myStruct, buf)
```

### UnmarshalSSZ

```go
func (d *DynSsz) UnmarshalSSZ(target any, ssz []byte, opts ...CallOption) error
```

Deserializes from SSZ format.

**Parameters**:
- `target` - Pointer to object to deserialize into
- `ssz` - SSZ encoded bytes
- `opts` - Optional call options (e.g., `WithViewDescriptor`)

**Returns**:
- Error if deserialization fails

**Example**:
```go
var decoded MyStruct
err := ssz.UnmarshalSSZ(&decoded, data)

// With view descriptor
err = ssz.UnmarshalSSZ(&decoded, data, dynssz.WithViewDescriptor(&Phase0View{}))
```

## Streaming Methods

### MarshalSSZWriter

```go
func (d *DynSsz) MarshalSSZWriter(source any, w io.Writer, opts ...CallOption) error
```

Serializes an object directly to an `io.Writer` for memory-efficient streaming.

**Parameters**:
- `source` - Object to serialize
- `w` - Destination writer (file, network connection, etc.)
- `opts` - Optional call options (e.g., `WithViewDescriptor`)

**Returns**:
- Error if serialization or writing fails

**Example**:
```go
file, _ := os.Create("beacon_state.ssz")
defer file.Close()

err := ssz.MarshalSSZWriter(state, file)
if err != nil {
    return err
}

// With view descriptor
err = ssz.MarshalSSZWriter(state, file, dynssz.WithViewDescriptor(&Phase0StateView{}))
```

### UnmarshalSSZReader

```go
func (d *DynSsz) UnmarshalSSZReader(target any, r io.Reader, size int, opts ...CallOption) error
```

Deserializes from an `io.Reader` for memory-efficient streaming.

**Parameters**:
- `target` - Pointer to object to deserialize into
- `r` - Source reader
- `size` - Expected total size of the SSZ data in bytes
- `opts` - Optional call options (e.g., `WithViewDescriptor`)

**Returns**:
- Error if deserialization fails

**Example**:
```go
file, _ := os.Open("beacon_state.ssz")
defer file.Close()

info, _ := file.Stat()
var state BeaconState
err := ssz.UnmarshalSSZReader(&state, file, int(info.Size()))

// With view descriptor
err = ssz.UnmarshalSSZReader(&state, file, int(info.Size()), dynssz.WithViewDescriptor(&AltairStateView{}))
```

See [Streaming Support](streaming.md) for detailed streaming documentation.

## Hash Tree Root

### HashTreeRoot

```go
func (d *DynSsz) HashTreeRoot(source any, opts ...CallOption) ([32]byte, error)
```

Computes the SSZ hash tree root.

**Parameters**:
- `source` - Object to hash
- `opts` - Optional call options (e.g., `WithViewDescriptor`)

**Returns**:
- 32-byte hash root
- Error if hashing fails

**Example**:
```go
root, err := ssz.HashTreeRoot(myStruct)
if err != nil {
    return err
}
fmt.Printf("Root: %x\n", root)

// With view descriptor for fork-specific hashing
root, err = ssz.HashTreeRoot(myStruct, dynssz.WithViewDescriptor(&AltairView{}))
```

### GetTree

```go
func (d *DynSsz) GetTree(source any, opts ...CallOption) (*treeproof.Node, error)
```

Builds and returns the complete Merkle tree for proof generation.

**Parameters**:
- `source` - Object to build tree for
- `opts` - Optional call options (e.g., `WithViewDescriptor`)

**Returns**:
- `*treeproof.Node` - Root node of the complete Merkle tree
- Error if tree construction fails

**Example**:
```go
tree, err := ssz.GetTree(myStruct)
if err != nil {
    return err
}

// Display tree structure
tree.Show(3)

// Generate proof for field at index 5
proof, err := tree.Prove(5)
if err != nil {
    return err
}

// Verify proof
isValid, err := treeproof.VerifyProof(tree.Hash(), proof)

// With view descriptor
tree, err = ssz.GetTree(myStruct, dynssz.WithViewDescriptor(&AltairView{}))
```

See [Merkle Proofs](merkle-proofs.md) for complete tree and proof generation documentation.

## Utility Methods

### SizeSSZ

```go
func (d *DynSsz) SizeSSZ(source any, opts ...CallOption) (int, error)
```

Calculates serialized size without serializing.

**Parameters**:
- `source` - Object to calculate size for
- `opts` - Optional call options (e.g., `WithViewDescriptor`)

**Returns**:
- Size in bytes
- Error if calculation fails

**Example**:
```go
size, err := ssz.SizeSSZ(myStruct)
fmt.Printf("Size: %d bytes\n", size)

// With view descriptor
size, err = ssz.SizeSSZ(myStruct, dynssz.WithViewDescriptor(&Phase0View{}))
```

### ValidateType

```go
func (d *DynSsz) ValidateType(t reflect.Type, opts ...CallOption) error
```

Validates that a type can be serialized. When a view descriptor is provided, also validates that the view type is compatible with the runtime type.

**Parameters**:
- `t` - Type to validate
- `opts` - Optional call options (e.g., `WithViewDescriptor` for view compatibility validation)

**Returns**:
- Error if type is invalid for SSZ or view is incompatible

**Example**:
```go
err := ssz.ValidateType(reflect.TypeOf(MyStruct{}))
if err != nil {
    fmt.Printf("Invalid type: %v\n", err)
}

// Validate view compatibility
err = ssz.ValidateType(
    reflect.TypeOf(BeaconState{}),
    dynssz.WithViewDescriptor(&Phase0BeaconStateView{}),
)
if err != nil {
    fmt.Printf("View incompatible: %v\n", err)
}
```

### GetTypeCache

```go
func (d *DynSsz) GetTypeCache() TypeCache
```

Returns the internal type cache for debugging purposes. Do not modify or build on the internal fields of the TypeCache. These might change in future versions of the library.

**Returns**:
- TypeCache instance

### ResolveSpecValue

```go
func (d *DynSsz) ResolveSpecValue(name string) (bool, uint64, error)
```

Resolves a specification value, including expressions.

**Parameters**:
- `name` - Spec name or expression (e.g., "SLOTS_PER_EPOCH*32")

**Returns**:
- Whether the value was found
- Resolved value
- Error if resolution fails

**Example**:
```go
found, value, err := ssz.ResolveSpecValue("MAX_VALIDATORS_PER_COMMITTEE")
```

## Custom Marshaling Interfaces

Types can implement these interfaces for custom behavior.

### Standard Interfaces (fastssz compatible)

```go
// Custom marshaling
type Marshaler interface {
    MarshalSSZ() ([]byte, error)
}

// Custom marshaling to buffer
type MarshalerTo interface {
    MarshalSSZTo(buf []byte) ([]byte, error)
}

// Custom unmarshaling
type Unmarshaler interface {
    UnmarshalSSZ(buf []byte) error
}

// Custom size calculation
type Sizer interface {
    SizeSSZ() int
}

// Custom hash tree root
type HashRoot interface {
    HashTreeRoot() ([32]byte, error)
}

// Custom hash tree root with hasher
type HashRootWith interface {
    HashTreeRootWith(hh HashWalker) error
}
```

### Dynamic Interfaces (spec-aware)

```go
// Dynamic marshaling with spec access
type DynamicMarshaler interface {
    MarshalSSZDyn(specs DynamicSpecs, buf []byte) ([]byte, error)
}

// Dynamic unmarshaling with spec access
type DynamicUnmarshaler interface {
    UnmarshalSSZDyn(specs DynamicSpecs, buf []byte) error
}

// Dynamic size calculation
type DynamicSizer interface {
    SizeSSZDyn(specs DynamicSpecs) int
}

// Dynamic hash tree root (common entrypoint)
type DynamicHashRoot interface {
    HashTreeRootDyn(specs DynamicSpecs) ([32]byte, error)
}

// Dynamic hash tree root with existing HashWalker
type DynamicHashRootWith interface {
    HashTreeRootWithDyn(specs DynamicSpecs, hh HashWalker) error
}
```

### Streaming Interfaces

```go
// Streaming encoder for memory-efficient marshaling
type DynamicEncoder interface {
    MarshalSSZEncoder(specs DynamicSpecs, encoder Encoder) error
}

// Streaming decoder for memory-efficient unmarshaling
type DynamicDecoder interface {
    UnmarshalSSZDecoder(specs DynamicSpecs, decoder Decoder) error
}
```

### View Interfaces

These interfaces enable a single runtime type to support multiple SSZ schemas (views). Typically implemented via code generation.

```go
// View-aware marshaling - returns nil if view not supported
type DynamicViewMarshaler interface {
    MarshalSSZDynView(view any) func(ds DynamicSpecs, buf []byte) ([]byte, error)
}

// View-aware unmarshaling - returns nil if view not supported
type DynamicViewUnmarshaler interface {
    UnmarshalSSZDynView(view any) func(ds DynamicSpecs, buf []byte) error
}

// View-aware size calculation - returns nil if view not supported
type DynamicViewSizer interface {
    SizeSSZDynView(view any) func(ds DynamicSpecs) int
}

// View-aware hash tree root - returns nil if view not supported
type DynamicViewHashRoot interface {
    HashTreeRootWithDynView(view any) func(ds DynamicSpecs, hh HashWalker) error
}

// View-aware streaming encoder - returns nil if view not supported
type DynamicViewEncoder interface {
    MarshalSSZEncoderView(view any) func(ds DynamicSpecs, encoder Encoder) error
}

// View-aware streaming decoder - returns nil if view not supported
type DynamicViewDecoder interface {
    UnmarshalSSZDecoderView(view any) func(ds DynamicSpecs, decoder Decoder) error
}
```

**Note**: These methods return `nil` if the view type is not recognized, causing Dynamic SSZ to fall back to reflection-based processing.

See [SSZ Views](views.md) for detailed view documentation.

### Encoder Interface

The `Encoder` interface abstracts buffer-based and stream-based encoding:

```go
type Encoder interface {
    Seekable() bool                    // Returns false for stream encoder
    GetPosition() int                 // Current write position
    GetBuffer() []byte                // Get output buffer (temp buffer for streams)
    SetBuffer(buffer []byte)          // Set/write buffer
    EncodeBool(v bool)
    EncodeUint8(v uint8)
    EncodeUint16(v uint16)
    EncodeUint32(v uint32)
    EncodeUint64(v uint64)
    EncodeBytes(v []byte)
    EncodeOffset(v uint32)
    EncodeOffsetAt(pos int, v uint32) // Not supported for streams
    EncodeZeroPadding(n int)
}
```

### Decoder Interface

The `Decoder` interface abstracts buffer-based and stream-based decoding:

```go
type Decoder interface {
    Seekable() bool                        // Returns false for stream decoder
    GetPosition() int                     // Current read position
    GetLength() int                       // Remaining length
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
    DecodeOffsetAt(pos int) uint32        // Not supported for streams
    SkipBytes(n int)                      // Not supported for streams
}
```

See [Streaming Support](streaming.md) for detailed streaming documentation.

### DynamicSpecs Interface

```go
type DynamicSpecs interface {
    ResolveSpecValue(name string) (bool, uint64, error)
}
```

Provides access to specification values during marshaling.

## Type Wrapper API

### TypeWrapper[D, T]

Generic wrapper for applying SSZ annotations.

```go
type TypeWrapper[D, T any] struct {
    Data T
}
```

**Type Parameters**:
- `D` - Descriptor struct type containing SSZ annotations as struct tags
- `T` - Wrapped value type

**Methods**:
```go
func (w *TypeWrapper[D, T]) Get() T
func (w *TypeWrapper[D, T]) Set(value T)
func (w *TypeWrapper[D, T]) GetDescriptorType() reflect.Type
```

See [Type Wrapper](type-wrapper.md) for detailed usage.

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
- `T` - Descriptor struct type defining union variants as fields

**Constructor**:
```go
func NewCompatibleUnion[T any](variantIndex uint8, data interface{}) (*CompatibleUnion[T], error)
```

**Methods**:
```go
func (u *CompatibleUnion[T]) GetDescriptorType() reflect.Type
```

**Usage**:
```go
// Define union descriptor
type PayloadUnion = CompatibleUnion[struct {
    ExecutionPayload
    ExecutionPayloadWithBlobs
}]

// Create union with variant 0
payload := PayloadUnion{
    Variant: 0,
    Data: ExecutionPayload{...},
}
```

## Error Types

Common errors returned by Dynamic SSZ:

```go
// From sszutils package
var (
    ErrListTooBig          = fmt.Errorf("list length is higher than max value")
    ErrUnexpectedEOF       = fmt.Errorf("unexpected end of SSZ")
    ErrOffset              = fmt.Errorf("incorrect offset")
    ErrInvalidUnionVariant = fmt.Errorf("invalid union variant")
    ErrVectorLength        = fmt.Errorf("incorrect vector length")
    ErrNotImplemented      = fmt.Errorf("not implemented")
)
```

## Code Generator API

### CodeGenerator

Programmatic code generation.

```go
type CodeGenerator struct {
    // Internal fields
}
```

#### NewCodeGenerator

```go
func NewCodeGenerator(dynSsz *dynssz.DynSsz) *CodeGenerator
```

#### BuildFile

```go
func (cg *CodeGenerator) BuildFile(fileName string, opts ...CodeGeneratorOption)
```

#### Generate

```go
func (cg *CodeGenerator) Generate() error
func (cg *CodeGenerator) GenerateToMap() (map[string]string, error)
```

### CodeGeneratorOption

```go
type CodeGeneratorOption func(*CodeGeneratorOptions)
```

Available options:

```go
// Method generation control
func WithNoMarshalSSZ() CodeGeneratorOption
func WithNoUnmarshalSSZ() CodeGeneratorOption
func WithNoSizeSSZ() CodeGeneratorOption
func WithNoHashTreeRoot() CodeGeneratorOption
func WithCreateLegacyFn() CodeGeneratorOption
func WithoutDynamicExpressions() CodeGeneratorOption
func WithNoFastSsz() CodeGeneratorOption
func WithCreateEncoderFn() CodeGeneratorOption  // Generate streaming encoder
func WithCreateDecoderFn() CodeGeneratorOption  // Generate streaming decoder

// Hint options
func WithSizeHints(hints []dynssz.SszSizeHint) CodeGeneratorOption
func WithMaxSizeHints(hints []dynssz.SszMaxSizeHint) CodeGeneratorOption
func WithTypeHints(hints []dynssz.SszTypeHint) CodeGeneratorOption

// Type specification
func WithReflectType(t reflect.Type, typeOpts ...CodeGeneratorOption) CodeGeneratorOption
func WithGoTypesType(t types.Type, typeOpts ...CodeGeneratorOption) CodeGeneratorOption

// View support (used as nested options within WithReflectType/WithGoTypesType)
func WithReflectViewTypes(views ...reflect.Type) CodeGeneratorOption  // Add view types using reflection
func WithGoTypesViewTypes(views ...types.Type) CodeGeneratorOption    // Add view types using go/types
func WithViewOnly() CodeGeneratorOption                               // Generate only view methods
```

See [Code Generator](code-generator.md) for detailed usage.

## Performance Utilities

### Buffer Pooling

Dynamic SSZ uses internal buffer pooling for performance:

```go
// Reuse buffers for marshaling
buf := make([]byte, 0, 1024)
data, err := ssz.MarshalSSZTo(obj, buf)

// Buffers are pooled internally for hashing
```

### Type Caching

Type descriptors are cached automatically:

```go
// First call analyzes type
ssz.MarshalSSZ(obj1)

// Subsequent calls use cache
ssz.MarshalSSZ(obj2)  // Faster
```

## Examples

### Basic Usage

```go
package main

import (
    dynssz "github.com/pk910/dynamic-ssz"
)

type Block struct {
    Slot      uint64
    StateRoot [32]byte
    Body      BlockBody
}

type BlockBody struct {
    Transactions []Transaction `ssz-max:"1048576"`
}

func main() {
    // Create instance
    ssz := dynssz.NewDynSsz(nil)
    
    // Create block
    block := &Block{
        Slot: 12345,
        // ... fill fields
    }
    
    // Serialize
    data, err := ssz.MarshalSSZ(block)
    if err != nil {
        panic(err)
    }
    
    // Compute root
    root, err := ssz.HashTreeRoot(block)
    if err != nil {
        panic(err)
    }
    
    // Deserialize
    var decoded Block
    err = ssz.UnmarshalSSZ(&decoded, data)
    if err != nil {
        panic(err)
    }
}
```

### With Spec Values

```go
// Define specs
specs := map[string]interface{}{
    "MAX_PROPOSER_SLASHINGS":     16,
    "MAX_ATTESTER_SLASHINGS":     2,
    "MAX_ATTESTATIONS":           128,
    "MAX_DEPOSITS":               16,
    "MAX_VOLUNTARY_EXITS":        16,
}

// Create instance
ssz := dynssz.NewDynSsz(specs)

// Use in types
type BeaconBlockBody struct {
    ProposerSlashings []ProposerSlashing `dynssz-max:"MAX_PROPOSER_SLASHINGS"`
    AttesterSlashings []AttesterSlashing `dynssz-max:"MAX_ATTESTER_SLASHINGS"`
    Attestations      []Attestation      `dynssz-max:"MAX_ATTESTATIONS"`
    Deposits          []Deposit          `dynssz-max:"MAX_DEPOSITS"`
    VoluntaryExits    []VoluntaryExit    `dynssz-max:"MAX_VOLUNTARY_EXITS"`
}
```

### Custom Marshaling

```go
type CustomType struct {
    data []byte
}

// Implement standard interface
func (c *CustomType) MarshalSSZ() ([]byte, error) {
    return c.data, nil
}

func (c *CustomType) UnmarshalSSZ(buf []byte) error {
    c.data = make([]byte, len(buf))
    copy(c.data, buf)
    return nil
}

// Implement dynamic interface
func (c *CustomType) MarshalSSZDyn(specs DynamicSpecs, buf []byte) ([]byte, error) {
    size := specs.GetValue("CUSTOM_SIZE")
    if len(c.data) != int(size) {
        return nil, fmt.Errorf("invalid size")
    }
    if cap(buf) >= len(c.data) {
        buf = buf[:len(c.data)]
        copy(buf, c.data)
        return buf, nil
    }
    return append(buf, c.data...), nil
}
```

## Related Documentation

- [Getting Started](getting-started.md) - Introduction and basics
- [Supported Types](supported-types.md) - Type system reference
- [SSZ Annotations](ssz-annotations.md) - Tag documentation
- [SSZ Views](views.md) - Fork handling with view descriptors
- [Code Generator](code-generator.md) - Code generation tools
- [Streaming Support](streaming.md) - Streaming encoding/decoding