# Troubleshooting Guide

This guide helps you diagnose and resolve common issues when using the dynamic-ssz library.

## Common Issues

### 1. Unmarshal Errors

#### "did not consume full ssz range"

**Problem**: This error occurs when the SSZ data provided to `UnmarshalSSZ` is larger than expected.

**Causes:**
- SSZ data contains extra bytes
- Wrong data being unmarshaled
- Size calculation mismatch

**Solutions:**
```go
// Check the actual size first
expectedSize, err := ds.SizeSSZ(&targetStruct)
if err != nil {
    log.Printf("Size calculation failed: %v", err)
}
log.Printf("Expected: %d bytes, got: %d bytes", expectedSize, len(sszData))

// Verify the data is correct for the target type
var target MyStruct
err = ds.UnmarshalSSZ(&target, sszData)
```

#### "Failed to unmarshal: unexpected end of SSZ data"

**Problem**: The SSZ data is truncated or smaller than expected.

**Solutions:**
- Verify the SSZ data is complete
- Check if the data was marshaled with the same specifications
- Ensure proper data transmission/storage

### 2. Marshal Errors

#### "ssz length does not match expected length"

**Problem**: The calculated size doesn't match the actual marshaled size.

**Causes:**
- Dynamic size calculation error
- Inconsistent specifications
- Bug in size calculation for complex nested structures

**Solutions:**
```go
// Debug size calculation
size, err := ds.SizeSSZ(myStruct)
if err != nil {
    log.Printf("Size calculation error: %v", err)
}

// Enable verbose logging for debugging
ds.Verbose = true
data, err := ds.MarshalSSZ(myStruct)
```

### 3. Specification Issues

#### "specification value not found"

**Problem**: A `dynssz-size` tag references a specification that wasn't provided.

**Solutions:**
```go
// Ensure all required specifications are provided
specs := map[string]any{
    "REQUIRED_SPEC_VALUE": uint64(1000),
    // Add all referenced specifications
}
ds := dynssz.NewDynSsz(specs)

// Check what specifications your types need
cache := ds.GetTypeCache()
json, err := cache.DumpTypeDescriptor(reflect.TypeOf(myStruct))
fmt.Println(json) // Review size hints
```

### 4. Performance Issues

#### Slow encoding/decoding

**Problem**: Operations are slower than expected.

**Solutions:**
```go
// 1. Reuse DynSsz instances
var globalDS *dynssz.DynSsz
func init() {
    globalDS = dynssz.NewDynSsz(specs)
}

// 2. Use buffer reuse
buf := make([]byte, 0, 1024*1024)
for _, item := range items {
    buf = buf[:0] // Reset length, keep capacity
    data, err := ds.MarshalSSZTo(item, buf)
    // Process data...
}

// 3. Check if fastssz is being used
ds.NoFastSsz = false // Ensure fastssz is enabled
```

### 5. Memory Issues

#### High memory usage

**Problem**: Memory consumption is higher than expected.

**Solutions:**
```go
// 1. Monitor allocations
import "runtime"

func checkMemory() {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    fmt.Printf("Alloc = %d KB", m.Alloc/1024)
}

// 2. Use memory pooling for frequent operations
var bufferPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 0, 1024*1024)
    },
}

// 3. Limit concurrent operations
```

### 6. Type Compatibility Issues

#### "unhandled reflection kind"

**Problem**: Trying to encode a type that's not supported by SSZ.

**Supported types:**
- `bool`
- `uint8`, `uint16`, `uint32`, `uint64`
- Arrays and slices of supported types
- Structs with supported field types
- Pointers to supported types

**Solutions:**
```go
// Convert unsupported types
type MyStruct struct {
    // ❌ Not supported
    // FloatValue float64
    // StringMap  map[string]string
    
    // ✅ Supported alternatives
    FloatAsUint64 uint64  // Store as fixed-point or scaled integer
    StringList    [][]byte `ssz-size:"10,64"`
}
```

## Debugging Techniques

### 1. Enable Verbose Logging

```go
ds.Verbose = true
data, err := ds.MarshalSSZ(myStruct)
// Check console output for detailed processing information
```

### 2. Type Descriptor Analysis

```go
cache := ds.GetTypeCache()

// Dump specific type descriptor
json, err := cache.DumpTypeDescriptor(reflect.TypeOf(myStruct))
if err != nil {
    log.Fatal(err)
}
fmt.Println("Type descriptor:", json)

// Dump all cached types
allTypes, err := cache.DumpAllCachedTypes()
if err != nil {
    log.Fatal(err)
}
fmt.Println("All cached types:", allTypes)
```

### 3. Size Calculation Testing

```go
// Test size calculation
size, err := ds.SizeSSZ(myStruct)
if err != nil {
    log.Printf("Size calculation failed: %v", err)
    return
}

// Compare with actual marshal size
data, err := ds.MarshalSSZ(myStruct)
if err != nil {
    log.Printf("Marshal failed: %v", err)
    return
}

if size != uint32(len(data)) {
    log.Printf("Size mismatch: calculated %d, actual %d", size, len(data))
}
```

### 4. Round-trip Testing

```go
func testRoundTrip(ds *dynssz.DynSsz, original interface{}) error {
    // Marshal
    data, err := ds.MarshalSSZ(original)
    if err != nil {
        return fmt.Errorf("marshal failed: %w", err)
    }
    
    // Create new instance of same type
    targetType := reflect.TypeOf(original)
    if targetType.Kind() == reflect.Ptr {
        targetType = targetType.Elem()
    }
    target := reflect.New(targetType).Interface()
    
    // Unmarshal
    err = ds.UnmarshalSSZ(target, data)
    if err != nil {
        return fmt.Errorf("unmarshal failed: %w", err)
    }
    
    // Calculate hash tree roots to verify
    originalRoot, err := ds.HashTreeRoot(original)
    if err != nil {
        return fmt.Errorf("original hash failed: %w", err)
    }
    
    targetRoot, err := ds.HashTreeRoot(target)
    if err != nil {
        return fmt.Errorf("target hash failed: %w", err)
    }
    
    if originalRoot != targetRoot {
        return fmt.Errorf("hash mismatch: %x != %x", originalRoot, targetRoot)
    }
    
    return nil
}
```

## Performance Profiling

### CPU Profiling

```go
import _ "net/http/pprof"
import "net/http"

go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()

// Then run: go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
```

### Memory Profiling

```go
// Add to your code
import "runtime/pprof"

func profileMemory() {
    f, err := os.Create("mem.prof")
    if err != nil {
        log.Fatal(err)
    }
    defer f.Close()
    
    runtime.GC()
    if err := pprof.WriteHeapProfile(f); err != nil {
        log.Fatal(err)
    }
}

// Then run: go tool pprof mem.prof
```

## Common Gotchas

### 1. Pointer vs Value Semantics

```go
// ❌ Wrong: passing value to UnmarshalSSZ
var myStruct MyStruct
err := ds.UnmarshalSSZ(myStruct, data) // Error!

// ✅ Correct: passing pointer to UnmarshalSSZ
var myStruct MyStruct
err := ds.UnmarshalSSZ(&myStruct, data)
```

### 2. Specification Consistency

```go
// ❌ Wrong: different specs for marshal/unmarshal
marshalDS := dynssz.NewDynSsz(map[string]any{"SIZE": uint64(100)})
data, _ := marshalDS.MarshalSSZ(myStruct)

unmarshalDS := dynssz.NewDynSsz(map[string]any{"SIZE": uint64(200)})
err := unmarshalDS.UnmarshalSSZ(&myStruct, data) // May fail!

// ✅ Correct: same specs for both operations
specs := map[string]any{"SIZE": uint64(100)}
ds := dynssz.NewDynSsz(specs)
data, _ := ds.MarshalSSZ(myStruct)
err := ds.UnmarshalSSZ(&myStruct, data)
```

### 3. Buffer Size Assumptions

```go
// ❌ Wrong: buffer too small
buf := make([]byte, 0, 10) // Too small!
data, err := ds.MarshalSSZTo(largeStruct, buf)

// ✅ Correct: appropriate buffer size
size, _ := ds.SizeSSZ(largeStruct)
buf := make([]byte, 0, size)
data, err := ds.MarshalSSZTo(largeStruct, buf)
```

## Getting More Help

1. **Check Examples**: Review the [examples](../examples/) directory for working code
2. **Enable Debugging**: Use `ds.Verbose = true` for detailed operation logs
3. **Profile Performance**: Use Go's built-in profiling tools for performance issues
4. **Test Incrementally**: Start with simple types and gradually add complexity
5. **Verify Specifications**: Ensure all referenced specifications are provided and consistent

## Reporting Issues

When reporting issues, please include:

1. **Minimal reproducible example**
2. **Go version and dynamic-ssz version**
3. **Error messages and stack traces**
4. **Type definitions and struct tags**
5. **Specifications used**
6. **Expected vs actual behavior**

This information helps diagnose and resolve issues quickly.