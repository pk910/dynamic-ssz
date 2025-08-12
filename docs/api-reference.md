# API Reference

This document provides comprehensive documentation for all public APIs in the dynamic-ssz library.

## Supported Types

Dynamic SSZ supports only SSZ-compatible types as defined in the SSZ specification:

### Base Types
- `uint8`/`byte`, `uint16`, `uint32`, `uint64` - Unsigned integers
- `bool` - Boolean values
- `string` - Strings (treated like a []byte)

### Composite Types
- **Arrays**: Fixed-size arrays of supported types
- **Slices**: Variable-size slices of supported types (require `ssz-size` or `ssz-max` tags)
- **Structs**: Structs containing only supported types
- **Pointers**: Pointers to structs (nil pointers will be filled with empty instances of the referred type)
- **TypeWrapper**: Generic wrapper for applying SSZ annotations to non-struct types (see [TypeWrapper Guide](type-wrapper.md))

### Not Supported
The following types are **not** part of the SSZ specification and therefore not supported:
- Signed integers (`int`, `int8`, `int16`, `int32`, `int64`)
- Floating-point numbers (`float32`, `float64`)
- Maps
- Channels
- Functions
- Complex numbers
- Interfaces (except when referring to concrete SSZ-compatible types)

### Handling Large Integers (uint128/uint256)

The SSZ specification defines `uint128` and `uint256` types, but Go doesn't have native support for these large integer types. This creates a gap between the SSZ specification and Go's type system.

#### Current Approach
- **For marshalling/unmarshalling**: Large integers are typically represented as byte arrays (`[16]byte` for uint128, `[32]byte` for uint256) or `uint64` arrays
- **For calculations**: Complex types that handle endianness and arithmetic operations are needed

#### Recommended Libraries
- **uint256**: Use `github.com/holiman/uint256` for proper uint256 handling
  ```go
  import "github.com/holiman/uint256"

  type MyStruct struct {
      // For SSZ marshalling/unmarshalling
      Balance1 [32]byte

      // For actual usage in calculations
      Balance2 uint256.Int
  }
  ```

**Note**: There is currently no widely adopted standard library for uint128 in Go. Consider using byte arrays or implementing custom handling based on your specific needs.

### Type Examples

```go
// Supported types
type ValidStruct struct {
    // Base types
    Count      uint64
    Flag       bool
    Hash       [32]byte
    Name       string

    // Composite types
    Values     []uint32      `ssz-max:"100"`
    Data       []byte        `ssz-max:"1024"`
    Labels     []string      `ssz-max:"10"`
    Matrix     [][]byte      `ssz-size:"?,32" ssz-max:"64"`
    SubStruct  *OtherStruct  // Pointer treated as empty instance for nil pointer
}

// TypeWrapper examples - for annotating non-struct types
type Hash32 = TypeWrapper[struct {
    Data []byte `ssz-type:"uint256"`
}, []byte]

type ValidatorList = TypeWrapper[struct {
    Data [][]byte `ssz-size:"?,48" ssz-max:"1000000"`
}, [][]byte]

// NOT supported
type InvalidStruct struct {
    Score   float64        // ❌ Not part of SSZ
    Count   int            // ❌ Use uint64 instead
    Mapping map[string]int // ❌ Maps not supported
}
```

## Core Types

### DynSsz

`DynSsz` is the main type for dynamic SSZ encoding/decoding operations.

```go
type DynSsz struct {
    NoFastSsz  bool // Disable fastssz optimization
    NoFastHash bool // Disable fast hashing using the optimized gohashtree hasher
    Verbose    bool // Enable verbose logging
}
```

#### Constructor

##### NewDynSsz

```go
func NewDynSsz(specs map[string]any) *DynSsz
```

Creates a new DynSsz instance with the provided specifications.

**Parameters:**
- `specs`: A map containing dynamic properties and configurations for SSZ serialization

**Returns:**
- A pointer to a new DynSsz instance

**Example:**
```go
specs := map[string]any{
    "SLOTS_PER_HISTORICAL_ROOT": uint64(8192),
    "SYNC_COMMITTEE_SIZE":       uint64(512),
}
ds := dynssz.NewDynSsz(specs)
```

## Encoding Methods

### MarshalSSZ

```go
func (d *DynSsz) MarshalSSZ(source any) ([]byte, error)
```

Serializes the given source into its SSZ representation.

**Parameters:**
- `source`: The Go value to be serialized

**Returns:**
- `[]byte`: The serialized data
- `error`: Error if serialization fails

**Example:**
```go
data, err := ds.MarshalSSZ(myStruct)
if err != nil {
    log.Fatal(err)
}
```

### MarshalSSZTo

```go
func (d *DynSsz) MarshalSSZTo(source any, buf []byte) ([]byte, error)
```

Serializes the given source into the provided buffer.

**Parameters:**
- `source`: The Go value to be serialized
- `buf`: Pre-allocated buffer for the serialized data

**Returns:**
- `[]byte`: The updated buffer containing serialized data
- `error`: Error if serialization fails

**Example:**
```go
buf := make([]byte, 0, 1024)
data, err := ds.MarshalSSZTo(myStruct, buf)
```

### SizeSSZ

```go
func (d *DynSsz) SizeSSZ(source any) (int, error)
```

Calculates the SSZ size of the given source without performing serialization.

**Parameters:**
- `source`: The Go value to calculate size for

**Returns:**
- `int`: The calculated size in bytes
- `error`: Error if size calculation fails

**Example:**
```go
size, err := ds.SizeSSZ(myStruct)
fmt.Printf("SSZ size: %d bytes\n", size)
```

## Streaming Methods

### MarshalSSZWriter

```go
func (d *DynSsz) MarshalSSZWriter(source any, w io.Writer) error
```

Serializes the given source into its SSZ representation and writes it directly to an io.Writer. This method provides memory-efficient streaming serialization for large data structures.

**Parameters:**
- `source`: The Go value to be serialized
- `w`: An io.Writer where the SSZ-encoded data will be written

**Returns:**
- `error`: Error if serialization or writing fails

**Example:**
```go
// Write to file
file, err := os.Create("beacon_state.ssz")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

err = ds.MarshalSSZWriter(state, file)
if err != nil {
    log.Fatal("Failed to write state:", err)
}

// Stream over network
conn, err := net.Dial("tcp", "localhost:8080")
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

err = ds.MarshalSSZWriter(block, conn)
```

### UnmarshalSSZReader

```go
func (d *DynSsz) UnmarshalSSZReader(target any, r io.Reader, size int64) error
```

Decodes SSZ-encoded data from an io.Reader directly into the target object. This method provides memory-efficient streaming deserialization, reading data incrementally from the source.

**Parameters:**
- `target`: Pointer to the Go value where the decoded data will be stored
- `r`: An io.Reader containing the SSZ-encoded data
- `size`: The expected size of the SSZ data (use -1 if unknown)

**Returns:**
- `error`: Error if decoding or reading fails

**Example:**
```go
// Read from file
file, err := os.Open("beacon_state.ssz")
if err != nil {
    log.Fatal(err)
}
defer file.Close()

// Get file size
info, _ := file.Stat()
var state phase0.BeaconState
err = ds.UnmarshalSSZReader(&state, file, info.Size())
if err != nil {
    log.Fatal("Failed to read state:", err)
}

// Read from network with unknown size
conn, _ := net.Dial("tcp", "localhost:8080")
var block phase0.BeaconBlock
err = ds.UnmarshalSSZReader(&block, conn, -1)
```

## Decoding Methods

### UnmarshalSSZ

```go
func (d *DynSsz) UnmarshalSSZ(target any, ssz []byte) error
```

Decodes SSZ-encoded data into the target object.

**Parameters:**
- `target`: Pointer to the Go value that will hold the decoded data
- `ssz`: The SSZ-encoded data to decode

**Returns:**
- `error`: Error if decoding fails

**Example:**
```go
var decoded MyStruct
err := ds.UnmarshalSSZ(&decoded, sszData)
if err != nil {
    log.Fatal(err)
}
```

## Hashing Methods

### HashTreeRoot

```go
func (d *DynSsz) HashTreeRoot(source any) ([32]byte, error)
```

Computes the hash tree root of the given source object.

**Parameters:**
- `source`: The Go value to compute hash tree root for

**Returns:**
- `[32]byte`: The computed hash tree root
- `error`: Error if computation fails

**Example:**
```go
root, err := ds.HashTreeRoot(myStruct)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Hash tree root: %x\n", root)
```

## Utility Methods

### GetTypeCache

```go
func (d *DynSsz) GetTypeCache() *TypeCache
```

Returns the type cache for the DynSsz instance. The type cache stores type descriptors for performance optimization.

**Returns:**
- `*TypeCache`: The type cache instance

**Example:**
```go
cache := ds.GetTypeCache()
descriptor, err := cache.GetTypeDescriptor(reflect.TypeOf(myStruct), nil, nil)
```

## Type Cache

### TypeCache

The `TypeCache` manages cached type descriptors for performance optimization.

#### GetTypeDescriptor

```go
func (tc *TypeCache) GetTypeDescriptor(t reflect.Type, sizeHints []SszSizeHint, maxSizeHints []SszMaxSizeHint) (*TypeDescriptor, error)
```

Returns a cached type descriptor, computing it if necessary.

**Parameters:**
- `t`: The reflection type to get descriptor for
- `sizeHints`: Size hints from struct tags
- `maxSizeHints`: Maximum size hints from struct tags

**Returns:**
- `*TypeDescriptor`: The type descriptor
- `error`: Error if descriptor creation fails

## Struct Tags

### ssz-size

Defines field sizes, compatible with fastssz. Use `?` to indicate dynamic length dimensions, or specify a number for fixed-size arrays/slices.

```go
type MyStruct struct {
    FixedData   []byte   `ssz-size:"32"`     // Fixed 32-byte slice
    DynamicData []byte   `ssz-size:"?"`      // Dynamic slice (requires ssz-max)
    Matrix      [][]byte `ssz-size:"4,32"`   // Fixed 4x32 matrix
    Dynamic2D   [][]byte `ssz-size:"?,32"`   // Dynamic outer, fixed inner
}
```

**Note**: Fixed-size fields (those with numeric values) do not use `ssz-max` tags.

### dynssz-size

Specifies sizes based on specification properties with expression support.

```go
type MyStruct struct {
    Roots []Root `ssz-size:"8192,32" dynssz-size:"SLOTS_PER_HISTORICAL_ROOT,32"`
}
```

#### Expression Support

The `dynssz-size` tag supports mathematical expressions:

- Direct reference: `dynssz-size:"SPEC_VALUE"`
- Mathematical expression: `dynssz-size:"(SPEC_VALUE*2)-5"`
- Multiple values: `dynssz-size:"(SPEC_VALUE1*SPEC_VALUE2)+SPEC_VALUE3"`

### ssz-max

**Required** for all dynamic length fields to properly calculate hash tree root. Defines the maximum number of elements.

```go
type MyStruct struct {
    DynamicData []byte    `ssz-max:"1024"`              // Max 1024 bytes
    Items       []Item    `ssz-max:"100"`               // Max 100 items
    Matrix      [][]byte  `ssz-size:"?,32" ssz-max:"64"` // Max 64 rows of 32 bytes each
}
```

### dynssz-max

Similar to `dynssz-size`, allows specification-based maximum sizes with expression support.

```go
type MyStruct struct {
    Validators []Validator `ssz-max:"1099511627776" dynssz-max:"VALIDATOR_REGISTRY_LIMIT"`
    Data       []byte      `dynssz-max:"MAX_DATA_SIZE"`
    Items      [][]byte    `ssz-size:"?,?" ssz-max:"100,256" dynssz-max:"MAX_ITEMS,MAX_ITEM_SIZE"`
}
```

### Multi-dimensional Arrays

For multi-dimensional arrays/slices, specify sizes and maximums for each dimension using comma-separated values:

```go
type MyStruct struct {
    // Fixed dimensions
    Fixed2D     [][]byte  `ssz-size:"4,32"`                // 4x32 fixed matrix

    // Mixed dimensions
    Mixed       [][]byte  `ssz-size:"?,32" ssz-max:"100"`  // Dynamic outer (max 100), fixed inner (32)

    // Fully dynamic
    Dynamic2D   [][]byte  `ssz-size:"?,?" ssz-max:"64,256"` // Max 64x256 matrix

    // With spec values
    SpecBased   [][]uint64 `ssz-size:"?,?" ssz-max:"100,100" dynssz-max:"MAX_COMMITTEES,MAX_VALIDATORS"`
}
```

Multi-dimensional slices are fully supported for all operations including hash tree root calculations, encoding, and decoding. This includes complex types like `[][]CustomType` and higher dimensional slices such as `[][][]byte`.

## Error Handling

All methods return errors that provide context about what went wrong:

- **Type errors**: When unsupported types are encountered
- **Size errors**: When calculated sizes don't match actual data
- **Parsing errors**: When SSZ data cannot be parsed
- **Specification errors**: When required specifications are missing

## Performance Considerations

1. **Instance Reuse**: Reuse DynSsz instances to benefit from type caching
2. **Buffer Reuse**: Use `MarshalSSZTo` with pre-allocated buffers
3. **Static Optimization**: The library automatically uses fastssz for static types
4. **Specification Caching**: Specification values are cached for performance

## TypeWrapper API

### Type Definition

```go
type TypeWrapper[D, T any] struct {
    Data T
}
```

Generic wrapper for applying SSZ annotations to non-struct types. See the [comprehensive TypeWrapper guide](type-wrapper.md) for detailed documentation and examples.

**Type Parameters:**
- `D`: Descriptor struct with exactly one field containing SSZ annotations
- `T`: The actual value type being wrapped

### Constructor

```go
func NewTypeWrapper[D, T any](data T) (*TypeWrapper[D, T], error)
```

Creates a new TypeWrapper instance.

### Methods

```go
func (w *TypeWrapper[D, T]) Get() T
func (w *TypeWrapper[D, T]) Set(value T)
func (w *TypeWrapper[D, T]) GetDescriptorType() reflect.Type // Internal use
```

### Usage Example

```go
type ByteArray32 = TypeWrapper[struct {
    Data []byte `ssz-size:"32"`
}, []byte]

// Usage
wrapper := ByteArray32{}
wrapper.Set([]byte{1, 2, 3})
data := wrapper.Get() // Type-safe access

// All SSZ operations work transparently
marshaled, _ := dynssz.MarshalSSZ(&wrapper)
hash, _ := dynssz.HashTreeRoot(&wrapper)
```

## Thread Safety

- DynSsz instances are thread-safe for all operations
- Type cache uses read-write mutexes for concurrent access
- Multiple goroutines can safely use the same DynSsz instance
