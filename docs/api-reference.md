# API Reference

This document provides comprehensive documentation for all public APIs in the dynamic-ssz library.

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

Defines static default field sizes, compatible with fastssz.

```go
type MyStruct struct {
    Data []byte `ssz-size:"32"`
}
```

### dynssz-size

Specifies dynamic sizes based on specification properties.

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

## Thread Safety

- DynSsz instances are thread-safe for all operations
- Type cache uses read-write mutexes for concurrent access
- Multiple goroutines can safely use the same DynSsz instance
